package hardware

import "time"

// EchoSuppressionConfig 回音抑制配置
type EchoSuppressionConfig struct {
	// 是否启用回音抑制
	Enabled bool `json:"enabled" yaml:"enabled"`

	// 基础音频能量阈值
	BaseEnergyThreshold int64 `json:"base_energy_threshold" yaml:"base_energy_threshold"`

	// TTS播放期间的能量阈值倍数
	TTSEnergyMultiplier float64 `json:"tts_energy_multiplier" yaml:"tts_energy_multiplier"`

	// 非常高能量的阈值倍数（用于TTS期间的严格检测）
	VeryHighEnergyMultiplier float64 `json:"very_high_energy_multiplier" yaml:"very_high_energy_multiplier"`

	// 回音抑制窗口时间（毫秒）
	SuppressionWindowMs int `json:"suppression_window_ms" yaml:"suppression_window_ms"`

	// TTS结束后的宽限期（毫秒）
	GracePeriodMs int `json:"grace_period_ms" yaml:"grace_period_ms"`

	// 回音检测的相似度阈值（0.0-1.0）
	SimilarityThreshold float64 `json:"similarity_threshold" yaml:"similarity_threshold"`

	// 回音检测的时间窗口（毫秒）
	EchoTimeWindowMs int `json:"echo_time_window_ms" yaml:"echo_time_window_ms"`

	// VAD检测配置
	VAD VADConfig `json:"vad" yaml:"vad"`
}

// VADConfig VAD检测配置
type VADConfig struct {
	// 基础RMS阈值
	BaseThreshold float64 `json:"base_threshold" yaml:"base_threshold"`

	// TTS播放期间的阈值倍数
	TTSMultiplier float64 `json:"tts_multiplier" yaml:"tts_multiplier"`

	// 需要连续超过阈值的帧数
	RequiredFrames int `json:"required_frames" yaml:"required_frames"`
}

// DefaultEchoSuppressionConfig 默认回音抑制配置
func DefaultEchoSuppressionConfig() *EchoSuppressionConfig {
	return &EchoSuppressionConfig{
		Enabled:                  true,
		BaseEnergyThreshold:      2000,
		TTSEnergyMultiplier:      4.0,
		VeryHighEnergyMultiplier: 6.0,
		SuppressionWindowMs:      5000,
		GracePeriodMs:            1000,
		SimilarityThreshold:      0.7,
		EchoTimeWindowMs:         2000,
		VAD: VADConfig{
			BaseThreshold:  2000.0,
			TTSMultiplier:  6.0,
			RequiredFrames: 20,
		},
	}
}

// ApplyConfig 应用配置到音频管理器和VAD检测器
func (config *EchoSuppressionConfig) ApplyConfig(audioManager *AudioManager, vadDetector *VADDetector) {
	if audioManager != nil {
		audioManager.SetEchoSuppression(config.Enabled)
		// 这里可以添加更多配置应用逻辑
	}

	if vadDetector != nil {
		vadDetector.SetEnabled(config.Enabled)
		// 这里可以添加更多VAD配置应用逻辑
	}
}

// GetEffectiveTTSThreshold 获取TTS播放期间的有效阈值
func (config *EchoSuppressionConfig) GetEffectiveTTSThreshold() int64 {
	return int64(float64(config.BaseEnergyThreshold) * config.TTSEnergyMultiplier)
}

// GetVeryHighThreshold 获取非常高能量的阈值
func (config *EchoSuppressionConfig) GetVeryHighThreshold() int64 {
	return int64(float64(config.BaseEnergyThreshold) * config.VeryHighEnergyMultiplier)
}

// GetGracePeriod 获取宽限期时间
func (config *EchoSuppressionConfig) GetGracePeriod() time.Duration {
	return time.Duration(config.GracePeriodMs) * time.Millisecond
}

// GetSuppressionWindow 获取抑制窗口时间
func (config *EchoSuppressionConfig) GetSuppressionWindow() time.Duration {
	return time.Duration(config.SuppressionWindowMs) * time.Millisecond
}

// GetEchoTimeWindow 获取回音检测时间窗口
func (config *EchoSuppressionConfig) GetEchoTimeWindow() time.Duration {
	return time.Duration(config.EchoTimeWindowMs) * time.Millisecond
}
