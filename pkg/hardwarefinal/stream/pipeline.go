package stream

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// TTSPipelineConfig 配置
type TTSPipelineConfig struct {
	TTSService          TTSService
	SendCallback        func(data []byte) error // 发送音频数据的回调
	GetPendingCountFunc func() int              // 获取待发送包数量的回调
	TTSWorkers          int                     // TTS 并发数，默认 2
	TargetSampleRate    int                     // 目标采样率，默认 16000
	FrameDuration       time.Duration           // 帧时长，默认 60ms
	Logger              *zap.Logger
}

// TTSPipeline TTS 流式处理管道
type TTSPipeline struct {
	segmenter           *TextSegmenter
	ttsWorkers          *TTSWorkerPool
	audioSender         *AudioSender
	segmentCh           chan TextSegment
	audioCh             chan AudioFrame
	currentPlayID       string
	mu                  sync.RWMutex
	ctx                 context.Context
	cancel              context.CancelFunc
	wg                  sync.WaitGroup
	logger              *zap.Logger
	onCompleteFunc      func() // 播放完成回调
	completeMu          sync.Mutex
	completionCancel    context.CancelFunc // 用于取消上一个 waitForCompletion
	completionCancelMu  sync.Mutex
	getPendingCountFunc func() int // 获取待发送包数量的回调
}

// NewTTSPipeline 创建 TTS 管道
func NewTTSPipeline(config *TTSPipelineConfig) (*TTSPipeline, error) {
	if config.TTSService == nil {
		return nil, fmt.Errorf("TTSService is required")
	}
	if config.SendCallback == nil {
		return nil, fmt.Errorf("SendCallback is required")
	}
	if config.TTSWorkers == 0 {
		config.TTSWorkers = 2
	}
	if config.TargetSampleRate == 0 {
		config.TargetSampleRate = 16000
	}
	if config.FrameDuration == 0 {
		config.FrameDuration = 60 * time.Millisecond
	}
	if config.Logger == nil {
		config.Logger, _ = zap.NewDevelopment()
	}

	segmentCh := make(chan TextSegment, 10)
	audioCh := make(chan AudioFrame, 20)
	segmenter := NewTextSegmenter(segmentCh, config.Logger)
	ttsWorkers := NewTTSWorkerPool(
		segmentCh,
		audioCh,
		config.TTSService,
		config.TTSWorkers,
		config.Logger,
	)
	audioSender, err := NewAudioSender(
		audioCh,
		config.TargetSampleRate,
		config.FrameDuration,
		config.SendCallback,
		config.GetPendingCountFunc,
		config.Logger,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create audio sender: %w", err)
	}

	return &TTSPipeline{
		segmenter:           segmenter,
		ttsWorkers:          ttsWorkers,
		audioSender:         audioSender,
		segmentCh:           segmentCh,
		audioCh:             audioCh,
		currentPlayID:       uuid.New().String(),
		logger:              config.Logger,
		getPendingCountFunc: config.GetPendingCountFunc,
	}, nil
}

// Start 启动管道
func (p *TTSPipeline) Start(ctx context.Context) error {
	p.ctx, p.cancel = context.WithCancel(ctx)

	// 设置 Segmenter 的 PlayID
	p.segmenter.SetPlayID(p.currentPlayID)

	// 启动所有组件
	if err := p.segmenter.Start(p.ctx); err != nil {
		return fmt.Errorf("failed to start segmenter: %w", err)
	}

	if err := p.ttsWorkers.Start(p.ctx); err != nil {
		return fmt.Errorf("failed to start tts workers: %w", err)
	}

	if err := p.audioSender.Start(p.ctx); err != nil {
		return fmt.Errorf("failed to start audio sender: %w", err)
	}

	p.logger.Info("TTSPipeline started",
		zap.String("play_id", p.currentPlayID))

	return nil
}

// Stop 停止管道
func (p *TTSPipeline) Stop() error {
	// 取消所有协程
	p.cancel()

	// 取消 waitForCompletion 协程
	p.completionCancelMu.Lock()
	if p.completionCancel != nil {
		p.completionCancel()
	}
	p.completionCancelMu.Unlock()

	// 停止所有组件
	p.segmenter.Stop()
	p.ttsWorkers.Stop()
	p.audioSender.Stop()

	// 关闭 channel
	close(p.segmentCh)
	close(p.audioCh)

	p.logger.Info("TTSPipeline stopped")
	return nil
}

// OnLLMToken 处理 LLM 输出的 token
func (p *TTSPipeline) OnLLMToken(token string) {
	p.segmenter.OnToken(token)
}

// OnLLMComplete LLM 完成输出
func (p *TTSPipeline) OnLLMComplete() {
	p.logger.Info("[TTS Pipeline] ========== OnLLMComplete 被调用 ==========")

	p.segmenter.OnComplete()

	// 取消上一个 waitForCompletion 协程（如果存在）
	p.completionCancelMu.Lock()
	if p.completionCancel != nil {
		p.logger.Info("[TTS Pipeline] 取消上一个 waitForCompletion 协程")
		p.completionCancel()
	}

	// 创建新的 context 用于本次 waitForCompletion
	completionCtx, completionCancel := context.WithCancel(p.ctx)
	p.completionCancel = completionCancel
	p.completionCancelMu.Unlock()

	p.logger.Info("[TTS Pipeline] LLM 完成，启动 waitForCompletion 协程")

	// 启动监控协程，等待所有音频发送完成
	go p.waitForCompletion(completionCtx)

	p.logger.Info("[TTS Pipeline] waitForCompletion 协程已启动")
}

// waitForCompletion 等待所有音频发送完成
func (p *TTSPipeline) waitForCompletion(ctx context.Context) {
	p.logger.Info("[TTS Pipeline] ========== waitForCompletion 协程开始执行 ==========")

	// 先等待一段时间，给 TTS 合成留出时间
	time.Sleep(500 * time.Millisecond)
	p.logger.Info("[TTS Pipeline] 初始等待完成，开始检查队列状态")

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	timeout := time.After(30 * time.Second) // 30秒超时
	emptyCount := 0                         // 连续空闲次数

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("[TTS Pipeline] waitForCompletion 被取消（可能是新的对话开始）")
			return
		case <-timeout:
			p.logger.Warn("[TTS Pipeline] 等待完成超时（30秒）")
			p.triggerComplete()
			return
		case <-ticker.C:
			bufferLevel := p.audioSender.GetBufferLevel()
			segmentChLen := len(p.segmentCh)
			audioChLen := len(p.audioCh)
			pendingPackets := 0
			if p.getPendingCountFunc != nil {
				pendingPackets = p.getPendingCountFunc()
			}
			p.logger.Info("[TTS Pipeline] 检查完成状态",
				zap.Int("segmentCh", segmentChLen),
				zap.Int("audioCh", audioChLen),
				zap.Int("bufferLevel", bufferLevel),
				zap.Int("pendingPackets", pendingPackets),
				zap.Int("emptyCount", emptyCount))

			// 检查所有 channel 和缓冲区是否为空
			if segmentChLen == 0 && audioChLen == 0 && bufferLevel == 0 && pendingPackets == 0 {
				emptyCount++
				p.logger.Info("[TTS Pipeline] 所有队列为空",
					zap.Int("emptyCount", emptyCount))
				// 连续 3 次检查都为空（300ms），认为播放完成
				if emptyCount >= 3 {
					p.logger.Info("[TTS Pipeline] ========== 所有音频已发送完成，触发回调 ==========")
					p.triggerComplete()
					return
				}
			} else {
				emptyCount = 0 // 重置计数
			}
		}
	}
}

// triggerComplete 触发完成回调
func (p *TTSPipeline) triggerComplete() {
	p.logger.Info("[TTS Pipeline] ========== triggerComplete 被调用 ==========")

	p.completeMu.Lock()
	defer p.completeMu.Unlock()

	if p.onCompleteFunc != nil {
		p.logger.Info("[TTS Pipeline] 执行完成回调函数")
		p.onCompleteFunc()
		p.logger.Info("[TTS Pipeline] 完成回调函数执行完毕")
	} else {
		p.logger.Warn("[TTS Pipeline] 完成回调函数为 nil，无法执行")
	}
}

// SetOnComplete 设置完成回调
func (p *TTSPipeline) SetOnComplete(callback func()) {
	p.completeMu.Lock()
	defer p.completeMu.Unlock()
	p.onCompleteFunc = callback
}

// Interrupt 打断当前播放
func (p *TTSPipeline) Interrupt() {
	p.mu.Lock()
	defer p.mu.Unlock()

	// 生成新的 PlayID
	oldPlayID := p.currentPlayID
	p.currentPlayID = uuid.New().String()

	p.logger.Info("Pipeline interrupted",
		zap.String("old_play_id", oldPlayID),
		zap.String("new_play_id", p.currentPlayID))

	// 重置所有组件
	p.segmenter.Reset()
	p.segmenter.SetPlayID(p.currentPlayID)
	p.audioSender.Reset()

	// 清空 channel
	p.drainChannel(p.segmentCh)
	p.drainChannel(p.audioCh)
}

// drainChannel 清空 channel
func (p *TTSPipeline) drainChannel(ch interface{}) {
	switch c := ch.(type) {
	case chan TextSegment:
		for len(c) > 0 {
			<-c
		}
	case chan AudioFrame:
		for len(c) > 0 {
			<-c
		}
	}
}

// GetCurrentPlayID 获取当前 PlayID
func (p *TTSPipeline) GetCurrentPlayID() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentPlayID
}

// GetBufferLevel 获取当前缓冲区水位
func (p *TTSPipeline) GetBufferLevel() int {
	return p.audioSender.GetBufferLevel()
}

// PrintMetrics 打印性能指标（简化版）
func (p *TTSPipeline) PrintMetrics() {
	bufferLevel := p.audioSender.GetBufferLevel()

	p.logger.Info("=== TTS Pipeline 状态 ===",
		zap.Int("buffer_level", bufferLevel),
		zap.Int("segment_ch", len(p.segmentCh)),
		zap.Int("audio_ch", len(p.audioCh)))
}

// UpdateTTSService 动态更新 TTS 服务（用于发音人切换）
// 这个方法不会重建整个 Pipeline，只更新 TTS 服务，速度更快
func (p *TTSPipeline) UpdateTTSService(newService TTSService) {
	p.logger.Info("Pipeline 开始更新 TTS 服务")
	p.ttsWorkers.UpdateTTSService(newService)
	p.logger.Info("Pipeline TTS 服务更新完成")
}
