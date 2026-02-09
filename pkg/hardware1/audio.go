package hardware

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	// TTSEchoSuppressionWindow TTS回音抑制窗口（毫秒）- 延长到5秒以确保完全覆盖
	ttsEchoSuppressionWindow = 5000
	// AudioEnergyThreshold 音频能量阈值（用于检测有效音频）- 提高阈值
	audioEnergyThreshold = 2000
	// TTSPlayingGracePeriod TTS播放状态的宽限期（毫秒）- 在TTS结束后继续抑制一段时间
	ttsPlayingGracePeriod = 1000
)

// AudioManager 音频管理器 - 解决TTS冲突问题
// 通过智能过滤TTS回音，实现真正的双向流
type AudioManager struct {
	mu              sync.RWMutex
	logger          *zap.Logger
	ttsOutputBuffer []TTSFrame // TTS输出音频缓冲区（用于回声消除）
	ttsOutputIndex  int        // 当前TTS输出索引
	maxTTSSamples   int        // 最大TTS样本数
	sampleRate      int        // 采样率
	channels        int        // 声道数
	echoSuppression bool       // 是否启用回音抑制
	lastTTSEndTime  time.Time  // 最后一次TTS结束时间（用于宽限期）
}

// TTSFrame TTS音频帧
type TTSFrame struct {
	Data      []byte
	Timestamp time.Time
	Energy    int64 // 音频能量（用于快速匹配）
}

// NewAudioManager 创建音频管理器
func NewAudioManager(sampleRate, channels int, logger *zap.Logger) *AudioManager {
	// 计算最大TTS样本数（基于回音抑制窗口）
	// 假设16-bit PCM，每样本2字节
	samplesPerMs := sampleRate * channels / 1000
	maxSamples := samplesPerMs * ttsEchoSuppressionWindow / 2 // 除以2因为16-bit

	return &AudioManager{
		logger:          logger,
		ttsOutputBuffer: make([]TTSFrame, 0, 100),
		sampleRate:      sampleRate,
		channels:        channels,
		maxTTSSamples:   maxSamples,
		echoSuppression: true, // 默认启用回音抑制
	}
}

// ProcessInputAudio 处理输入音频（智能过滤TTS回音）
// 返回 (处理后的音频数据, 是否应该处理)
func (m *AudioManager) ProcessInputAudio(data []byte, ttsPlaying bool) ([]byte, bool) {
	if len(data) == 0 {
		return nil, false
	}

	m.mu.RLock()
	echoSuppression := m.echoSuppression
	lastTTSEndTime := m.lastTTSEndTime
	m.mu.RUnlock()

	// 检查是否在TTS宽限期内（TTS刚结束，仍可能有回音）
	inGracePeriod := !lastTTSEndTime.IsZero() && time.Since(lastTTSEndTime) < ttsPlayingGracePeriod*time.Millisecond
	effectiveTTSPlaying := ttsPlaying || inGracePeriod

	// 如果TTS不在播放且不在宽限期内，直接处理
	if !effectiveTTSPlaying || !echoSuppression {
		return data, true
	}

	// 计算输入音频的能量
	inputEnergy := m.calculateEnergy(data)

	// TTS播放期间或宽限期内，大幅提高能量阈值，只有非常强的信号才能通过
	ttsAudioEnergyThreshold := int64(audioEnergyThreshold * 4) // 提高到8000

	// 如果能量太低，可能是静音或无效音频
	if inputEnergy < ttsAudioEnergyThreshold {
		m.logger.Debug("TTS播放期间/宽限期：输入音频能量过低，忽略",
			zap.Int64("energy", inputEnergy),
			zap.Int64("threshold", ttsAudioEnergyThreshold),
			zap.Bool("ttsPlaying", ttsPlaying),
			zap.Bool("inGracePeriod", inGracePeriod),
		)
		return nil, false
	}

	// 检查是否是TTS回音（更严格的检测）
	if m.isTTSEcho(data, inputEnergy) {
		m.logger.Debug("检测到TTS回音，过滤",
			zap.Int64("energy", inputEnergy),
		)
		return nil, false
	}

	// 额外的安全检查：即使不是明显的回音，在TTS播放期间也要更谨慎
	// 只有能量非常高的音频才被认为是真正的用户语音
	veryHighEnergyThreshold := int64(audioEnergyThreshold * 6) // 12000
	if inputEnergy < veryHighEnergyThreshold {
		m.logger.Debug("TTS播放期间/宽限期：音频能量不够高，可能是回音或噪音，过滤",
			zap.Int64("energy", inputEnergy),
			zap.Int64("veryHighThreshold", veryHighEnergyThreshold),
			zap.Bool("ttsPlaying", ttsPlaying),
			zap.Bool("inGracePeriod", inGracePeriod),
		)
		return nil, false
	}

	// 通过所有检查，可能是真正的用户语音
	m.logger.Info("TTS播放期间/宽限期：检测到高能量音频，可能是用户语音",
		zap.Int64("energy", inputEnergy),
		zap.Int64("threshold", veryHighEnergyThreshold),
		zap.Bool("ttsPlaying", ttsPlaying),
		zap.Bool("inGracePeriod", inGracePeriod),
	)
	return data, true
}

// RecordTTSOutput 记录TTS输出音频（用于回声消除）
func (m *AudioManager) RecordTTSOutput(data []byte) {
	if len(data) == 0 {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// 计算能量
	energy := m.calculateEnergy(data)

	// 添加到缓冲区
	frame := TTSFrame{
		Data:      make([]byte, len(data)),
		Timestamp: time.Now(),
		Energy:    energy,
	}
	copy(frame.Data, data)

	m.ttsOutputBuffer = append(m.ttsOutputBuffer, frame)

	// 限制缓冲区大小（保留最近的回音抑制窗口内的数据）
	windowSize := m.sampleRate * m.channels * ttsEchoSuppressionWindow / 1000
	totalSamples := 0
	for i := len(m.ttsOutputBuffer) - 1; i >= 0; i-- {
		totalSamples += len(m.ttsOutputBuffer[i].Data) / 2 // 16-bit = 2 bytes per sample
		if totalSamples > windowSize {
			// 删除超出窗口的数据
			m.ttsOutputBuffer = m.ttsOutputBuffer[i+1:]
			break
		}
	}

	// 限制最大帧数
	if len(m.ttsOutputBuffer) > 100 {
		m.ttsOutputBuffer = m.ttsOutputBuffer[len(m.ttsOutputBuffer)-100:]
	}
}

// isTTSEcho 检查输入音频是否是TTS回音
func (m *AudioManager) isTTSEcho(inputData []byte, inputEnergy int64) bool {
	if len(m.ttsOutputBuffer) == 0 {
		return false
	}

	// 检查更多帧以提高准确性
	checkFrames := 20 // 增加检查帧数
	if len(m.ttsOutputBuffer) < checkFrames {
		checkFrames = len(m.ttsOutputBuffer)
	}

	startIdx := len(m.ttsOutputBuffer) - checkFrames

	// 更严格的能量和相似度匹配
	for i := startIdx; i < len(m.ttsOutputBuffer); i++ {
		frame := m.ttsOutputBuffer[i]

		// 检查时间窗口（回音通常在TTS输出后50-2000ms内）
		timeDiff := time.Since(frame.Timestamp)
		if timeDiff < 50*time.Millisecond || timeDiff > 2*time.Second {
			continue
		}

		// 更严格的能量匹配 - 能量差异不能超过20%
		energyDiff := abs64(inputEnergy - frame.Energy)
		energyThreshold := frame.Energy * 2 / 10 // 20%
		if energyDiff > energyThreshold {
			continue
		}

		// 如果数据长度相似，进行更详细的比较
		lengthDiff := abs(len(inputData) - len(frame.Data))
		if lengthDiff < len(frame.Data)/4 { // 长度差异不超过25%
			// 计算相似度（提高阈值）
			similarity := m.calculateSimilarity(inputData, frame.Data)
			if similarity > 0.7 { // 提高相似度阈值到70%
				m.logger.Debug("检测到回音匹配",
					zap.Float64("similarity", similarity),
					zap.Int64("inputEnergy", inputEnergy),
					zap.Int64("frameEnergy", frame.Energy),
					zap.Duration("timeDiff", timeDiff),
				)
				return true
			}
		}

		// 额外检查：如果能量非常接近且时间窗口合适，也认为是回音
		if energyDiff < frame.Energy/10 && timeDiff < 500*time.Millisecond { // 能量差异小于10%且时间在500ms内
			m.logger.Debug("通过能量和时间检测到回音",
				zap.Int64("inputEnergy", inputEnergy),
				zap.Int64("frameEnergy", frame.Energy),
				zap.Int64("energyDiff", energyDiff),
				zap.Duration("timeDiff", timeDiff),
			)
			return true
		}
	}

	return false
}

// calculateEnergy 计算音频能量
func (m *AudioManager) calculateEnergy(data []byte) int64 {
	if len(data) < 2 {
		return 0
	}

	var sumSquares int64
	sampleCount := len(data) / 2

	for i := 0; i < sampleCount; i++ {
		sample := int16(data[i*2]) | (int16(data[i*2+1]) << 8)
		sumSquares += int64(sample) * int64(sample)
	}

	if sampleCount == 0 {
		return 0
	}

	return sumSquares / int64(sampleCount)
}

// calculateSimilarity 计算两个音频数据的相似度
func (m *AudioManager) calculateSimilarity(data1, data2 []byte) float64 {
	if len(data1) == 0 || len(data2) == 0 {
		return 0.0
	}

	// 取较小的长度进行比较
	minLen := len(data1)
	if len(data2) < minLen {
		minLen = len(data2)
	}

	if minLen < 2 {
		return 0.0
	}

	// 比较前N个样本（简化版）
	compareSamples := minLen / 2
	if compareSamples > 100 {
		compareSamples = 100 // 最多比较100个样本
	}

	var diffSum int64
	for i := 0; i < compareSamples; i++ {
		sample1 := int16(data1[i*2]) | (int16(data1[i*2+1]) << 8)
		sample2 := int16(data2[i*2]) | (int16(data2[i*2+1]) << 8)
		diff := int64(sample1) - int64(sample2)
		if diff < 0 {
			diff = -diff
		}
		diffSum += diff
	}

	// 归一化相似度（0-1）
	maxDiff := int64(65536) * int64(compareSamples) // 16-bit最大值
	if maxDiff == 0 {
		return 0.0
	}

	similarity := 1.0 - float64(diffSum)/float64(maxDiff)
	if similarity < 0 {
		return 0.0
	}
	return similarity
}

// Clear 清空状态
func (m *AudioManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsOutputBuffer = m.ttsOutputBuffer[:0]
	m.ttsOutputIndex = 0
	m.lastTTSEndTime = time.Time{} // 重置TTS结束时间
}

// SetEchoSuppression 设置回音抑制开关
func (m *AudioManager) SetEchoSuppression(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.echoSuppression = enabled
}

// NotifyTTSEnd 通知TTS播放结束（用于宽限期计算）
func (m *AudioManager) NotifyTTSEnd() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastTTSEndTime = time.Now()
	m.logger.Debug("TTS播放结束，开始宽限期",
		zap.Duration("gracePeriod", ttsPlayingGracePeriod*time.Millisecond))
}
