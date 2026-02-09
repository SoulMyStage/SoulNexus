package sessions

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"
)

// VADComponent VAD 管道组件（用于 ASR Pipeline）
type VADComponent struct {
	enabled                 bool
	threshold               float64 // RMS 阈值
	consecutiveFramesNeeded int     // 需要连续超过阈值的帧数
	frameCounter            int     // 当前连续帧计数
	logger                  *zap.Logger
	mu                      sync.RWMutex
	lastLogTime             time.Time // 上次日志时间（用于限流）

	// Barge-in 相关
	bargeInCallback func()      // 检测到 barge-in 时的回调
	getTTSPlaying   func() bool // 获取 TTS 播放状态的回调
}

// NewVADComponent 创建 VAD 管道组件
func NewVADComponent(enabled bool, threshold float64, consecutiveFrames int, logger *zap.Logger) *VADComponent {
	if logger == nil {
		logger = zap.NewNop()
	}

	// 设置默认值
	if threshold <= 0 {
		threshold = 500.0 // 默认阈值
	}
	if consecutiveFrames <= 0 {
		consecutiveFrames = 1 // 默认1帧
	}

	return &VADComponent{
		enabled:                 enabled,
		threshold:               threshold,
		consecutiveFramesNeeded: consecutiveFrames,
		frameCounter:            0,
		logger:                  logger,
		lastLogTime:             time.Now(),
	}
}

// SetBargeInCallback 设置 barge-in 回调
func (v *VADComponent) SetBargeInCallback(callback func()) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.bargeInCallback = callback
}

// SetTTSPlayingCallback 设置获取 TTS 播放状态的回调
func (v *VADComponent) SetTTSPlayingCallback(callback func() bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.getTTSPlaying = callback
}

// Name 返回组件名称
func (v *VADComponent) Name() string {
	return "VAD"
}

// Process 处理音频数据
// 返回：处理后的数据，是否继续处理，错误
func (v *VADComponent) Process(ctx context.Context, data interface{}) (interface{}, bool, error) {
	pcmData, ok := data.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("invalid data type for VAD")
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// 如果 VAD 未启用，直接通过
	if !v.enabled {
		return pcmData, true, nil
	}

	// 检查是否在 TTS 播放期间
	isTTSPlaying := false
	if v.getTTSPlaying != nil {
		isTTSPlaying = v.getTTSPlaying()
	}

	// ========== 关键逻辑：只在 TTS 播放期间才进行 VAD 检测 ==========
	// 平时不做 VAD 检测，所有音频直接通过
	if !isTTSPlaying {
		// 重置计数器
		v.frameCounter = 0
		// 直接通过，不做 VAD 检测
		return pcmData, true, nil
	}

	// ========== TTS 播放期间：检测 Barge-in ==========
	// 计算音频能量 (RMS)
	rms := calculateRMS(pcmData)

	// 限流日志：每3秒最多记录一次
	now := time.Now()
	shouldLog := now.Sub(v.lastLogTime) >= 3*time.Second

	// 检查能量是否超过阈值
	if rms > v.threshold {
		v.frameCounter++
		if shouldLog {
			v.lastLogTime = now
			v.logger.Debug("[VAD] TTS 播放期间检测到音频能量",
				zap.Float64("rms", rms),
				zap.Float64("threshold", v.threshold),
				zap.Int("frameCounter", v.frameCounter),
				zap.Int("framesNeeded", v.consecutiveFramesNeeded),
			)
		}

		// 达到连续帧数要求，认为是有效语音，触发 barge-in
		if v.frameCounter >= v.consecutiveFramesNeeded {
			if v.bargeInCallback != nil {
				v.logger.Info("[VAD] TTS 播放期间检测到用户语音，触发 Barge-in",
					zap.Float64("rms", rms),
					zap.Int("frames", v.frameCounter))
				v.bargeInCallback()
				// 触发后重置计数器，避免重复触发
				v.frameCounter = 0
			}
		}
	} else {
		// 能量低于阈值，重置计数器
		if v.frameCounter > 0 && shouldLog {
			v.lastLogTime = now
			v.logger.Debug("[VAD] 音频能量低于阈值，重置计数器",
				zap.Float64("rms", rms),
				zap.Float64("threshold", v.threshold),
				zap.Int("previousFrameCounter", v.frameCounter),
			)
		}
		v.frameCounter = 0
	}

	// TTS 播放期间，所有音频都通过（发送静音帧给 ASR）
	return pcmData, true, nil
}

// SetEnabled 设置 VAD 是否启用
func (v *VADComponent) SetEnabled(enabled bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.enabled = enabled
	if !enabled {
		v.frameCounter = 0
	}
	v.logger.Info("[VAD] 状态已更新", zap.Bool("enabled", enabled))
}

// SetThreshold 设置 RMS 阈值
func (v *VADComponent) SetThreshold(threshold float64) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.threshold = threshold
	v.logger.Info("[VAD] 阈值已更新", zap.Float64("threshold", threshold))
}

// SetConsecutiveFrames 设置需要连续超过阈值的帧数
func (v *VADComponent) SetConsecutiveFrames(frames int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.consecutiveFramesNeeded = frames
	v.logger.Info("[VAD] 连续帧数已更新", zap.Int("frames", frames))
}

// Reset 重置 VAD 状态
func (v *VADComponent) Reset() {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.frameCounter = 0
	v.logger.Debug("[VAD] 状态已重置")
}

// calculateRMS 计算 16-bit PCM 音频数据的 RMS (Root Mean Square)
// 返回值范围：0 到 32768（对于16-bit PCM）
// 正常语音的RMS通常在 500-5000 之间，静音通常在 0-100 之间
func calculateRMS(pcmData []byte) float64 {
	if len(pcmData) < 2 {
		return 0
	}

	var sumSquares float64
	sampleCount := len(pcmData) / 2

	if sampleCount == 0 {
		return 0
	}

	for i := 0; i < len(pcmData)-1; i += 2 {
		// 转换为 int16 (little-endian)
		sample := int16(pcmData[i]) | int16(pcmData[i+1])<<8
		// 使用绝对值
		absSample := math.Abs(float64(sample))
		sumSquares += absSample * absSample
	}

	return math.Sqrt(sumSquares / float64(sampleCount))
}
