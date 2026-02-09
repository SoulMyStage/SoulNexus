package sessions

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/code-100-precent/LingEcho/pkg/media"
	"github.com/code-100-precent/LingEcho/pkg/media/encoder"
	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"go.uber.org/zap"
)

// pipeline architecture for asr handler
// handle data by: audio input -> VAD -> echo filter -> ASR -> sensitive filter -> output text

// Pipeline pipeline abstract interface
type Pipeline interface {
	Process(ctx context.Context, data interface{}) (interface{}, error)
}

// PipelineComponent pipeline component abstract interface
type PipelineComponent interface {
	Name() string
	Process(ctx context.Context, data interface{}) (interface{}, bool, error)
}

// PipelineMetrics ASR Pipeline 性能指标
type PipelineMetrics struct {
	mu              sync.RWMutex
	FirstPacketTime time.Time     // 第一个音频包时间
	LastPacketTime  time.Time     // 最后一个音频包时间
	ASRFirstResult  time.Time     // 第一个 ASR 结果时间
	ASRLatency      time.Duration // ASR 延迟（从最后一个包到第一个结果）
	TotalAudioBytes int           // 总音频字节数（PCM）
	AudioDuration   time.Duration // 音频总时长
	RTF             float64       // Real-Time Factor (处理时间/音频时长)
}

type ASRPipelineOption struct {
	Asr           recognizer.TranscribeService
	SampleRate    int
	Channels      int
	FrameDuration string
}

func (ao *ASRPipelineOption) String() string {
	return fmt.Sprintf("ASRPipelineOption{Asr: %s, SampleRate: %d, Channels: %d, FrameDuration: %s}", ao.Asr, ao.SampleRate, ao.Channels, ao.FrameDuration)
}

// ASRPipeline Global ASR pipeline
type ASRPipeline struct {
	inputStages  []PipelineComponent
	Asr          recognizer.TranscribeService // 导出以便外部访问
	outputStages []PipelineComponent
	onOutput     func(text string, isFinal bool)
	logger       *zap.Logger
	metrics      *PipelineMetrics
}

// NewASRPipeline Create a new ASR pipeline
func NewASRPipeline(option *ASRPipelineOption, logger *zap.Logger) (*ASRPipeline, error) {
	if option == nil || option.Asr == nil {
		return nil, fmt.Errorf("invalid option or invalid asr service")
	}
	pipeline := &ASRPipeline{
		Asr:     option.Asr,
		logger:  logger,
		metrics: &PipelineMetrics{},
	}
	decode, err := encoder.CreateDecode(
		media.CodecConfig{
			Codec:         "opus",
			SampleRate:    option.SampleRate,
			Channels:      option.Channels,
			BitDepth:      16,
			FrameDuration: option.FrameDuration,
		},
		media.CodecConfig{
			Codec:         "pcm",
			SampleRate:    option.SampleRate,
			Channels:      option.Channels,
			BitDepth:      16,
			FrameDuration: option.FrameDuration,
		},
	)
	if err != nil {
		return nil, err
	}
	pipeline.inputStages = []PipelineComponent{
		&OpusDecodeComponent{logger: logger, decoder: decode},
		//&VADComponent{logger: logger},
		//&EchoFilterComponent{logger: logger},
		&ASRInputComponent{asr: pipeline.Asr, logger: logger, metrics: pipeline.metrics},
	}
	// 输出阶段：ASR 结果是文本，不需要编码
	pipeline.outputStages = []PipelineComponent{
		//&FilterWordComponent{logger: logger},
		// 注意：ASR 输出是文本，不需要 OpusEncode
	}
	pipeline.Asr.Init(
		pipeline.onASRResult,
		pipeline.onASRError,
	)
	return pipeline, nil
}

// onASRResult ASR 结果回调
func (p *ASRPipeline) onASRResult(text string, isFinal bool, duration time.Duration, uuid string) {
	p.logger.Info("[Pipeline] onASRResult 被调用",
		zap.String("text", text),
		zap.Bool("isFinal", isFinal),
		zap.Duration("duration", duration),
		zap.String("uuid", uuid))
	if text == "" {
		p.logger.Debug("[Pipeline] 跳过空文本")
		return
	}
	p.metrics.mu.Lock()
	if p.metrics.ASRFirstResult.IsZero() {
		p.metrics.ASRFirstResult = time.Now()
		if !p.metrics.LastPacketTime.IsZero() {
			p.metrics.ASRLatency = p.metrics.ASRFirstResult.Sub(p.metrics.LastPacketTime)
		}
	}
	p.metrics.mu.Unlock()
	if p.onOutput != nil {
		p.logger.Debug("[Pipeline] 调用输出回调",
			zap.String("text", text),
			zap.Bool("isFinal", isFinal))
		p.onOutput(text, isFinal)
	} else {
		p.logger.Warn("[Pipeline] 输出回调未设置！")
	}
}

// onASRError ASR 错误回调
func (p *ASRPipeline) onASRError(err error, isFatal bool) {
	p.logger.Error("[Pipeline] ASR 错误", zap.Error(err), zap.Bool("fatal", isFatal))
}

// ProcessInput 处理输入（音频）
func (p *ASRPipeline) ProcessInput(ctx context.Context, audioData []byte) error {
	p.metrics.mu.Lock()
	if p.metrics.FirstPacketTime.IsZero() {
		p.metrics.FirstPacketTime = time.Now()
	}
	p.metrics.LastPacketTime = time.Now()
	p.metrics.mu.Unlock()
	current := interface{}(audioData)
	for _, stage := range p.inputStages {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		result, shouldContinue, err := stage.Process(ctx, current)
		if err != nil {
			p.logger.Error("[Pipeline] 输入阶段失败",
				zap.String("stage", stage.Name()),
				zap.Error(err),
			)
			return err
		}

		if !shouldContinue {
			p.logger.Debug("[Pipeline] 输入被过滤",
				zap.String("stage", stage.Name()),
			)
			return nil
		}

		current = result
	}

	return nil
}

// SetOutputCallback 设置输出回调
func (p *ASRPipeline) SetOutputCallback(callback func(text string, isFinal bool)) {
	p.onOutput = callback
}

// ResetState 重置状态（用于新的对话轮次）
func (p *ASRPipeline) ResetState() {
	p.metrics.mu.Lock()
	p.metrics.FirstPacketTime = time.Time{}
	p.metrics.LastPacketTime = time.Time{}
	p.metrics.ASRFirstResult = time.Time{}
	p.metrics.ASRLatency = 0
	p.metrics.TotalAudioBytes = 0
	p.metrics.AudioDuration = 0
	p.metrics.RTF = 0
	p.metrics.mu.Unlock()

	p.logger.Debug("[Pipeline] 状态已重置")
}

// ASRInputComponent ASR 输入阶段（发送到 ASR）
type ASRInputComponent struct {
	asr     recognizer.TranscribeService
	logger  *zap.Logger
	metrics *PipelineMetrics
}

func (s *ASRInputComponent) Name() string {
	return "ASRInput"
}

func (s *ASRInputComponent) Process(ctx context.Context, data interface{}) (interface{}, bool, error) {
	pcmData, ok := data.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("invalid data type")
	}

	// 记录 PCM 数据大小（用于计算音频时长）
	if s.metrics != nil {
		s.metrics.mu.Lock()
		s.metrics.TotalAudioBytes += len(pcmData)
		s.metrics.mu.Unlock()
	}

	err := s.asr.SendAudioBytes(pcmData)
	if err != nil {
		s.logger.Error(fmt.Sprintf("[ASRInput] 发送失败, %v", err))
		return nil, false, err
	}
	return nil, true, nil
}

// GetMetrics 获取性能指标（线程安全）
func (p *ASRPipeline) GetMetrics() PipelineMetrics {
	p.metrics.mu.RLock()
	defer p.metrics.mu.RUnlock()

	// 计算音频时长（假设 16kHz, 单声道, 16-bit）
	if p.metrics.TotalAudioBytes > 0 {
		p.metrics.AudioDuration = time.Duration(p.metrics.TotalAudioBytes/2/16000) * time.Second
	}

	// 计算 RTF (Real-Time Factor)
	// RTF = ASR延迟 / 音频时长
	if p.metrics.AudioDuration > 0 && p.metrics.ASRLatency > 0 {
		p.metrics.RTF = float64(p.metrics.ASRLatency) / float64(p.metrics.AudioDuration)
	}

	return *p.metrics
}

// PrintMetrics 打印性能指标
func (p *ASRPipeline) PrintMetrics() {
	metrics := p.GetMetrics()

	p.logger.Info("=== ASR Pipeline 性能指标 ===")
	p.logger.Info("ASR 延迟:",
		zap.Duration("latency", metrics.ASRLatency),
		zap.String("说明", "从最后一个音频包到第一个识别结果"),
	)
	p.logger.Info("实时性能:",
		zap.Float64("rtf", metrics.RTF),
		zap.Duration("audio_duration", metrics.AudioDuration),
		zap.String("performance", getRTFDescription(metrics.RTF)),
	)
}

// getRTFDescription 获取 RTF 性能描述
func getRTFDescription(rtf float64) string {
	if rtf < 0.1 {
		return "优秀 (Excellent) - 实时性极佳"
	} else if rtf < 0.3 {
		return "良好 (Good) - 实时性好"
	} else if rtf < 0.5 {
		return "可接受 (Acceptable) - 接近实时"
	} else {
		return "需要优化 (Needs Optimization) - 延迟较高"
	}
}
