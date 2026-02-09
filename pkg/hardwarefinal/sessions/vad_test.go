package sessions

import (
	"math"
	"testing"
	"time"

	"go.uber.org/zap"
)

// generateSilence 生成静音音频
func generateSilence(duration time.Duration, sampleRate int) []byte {
	samples := int(duration.Seconds() * float64(sampleRate))
	data := make([]byte, samples*2) // 16-bit PCM
	return data
}

// generateTone 生成正弦波音频（模拟语音）
func generateTone(duration time.Duration, sampleRate int, frequency float64, amplitude float64) []byte {
	samples := int(duration.Seconds() * float64(sampleRate))
	data := make([]byte, samples*2) // 16-bit PCM

	for i := 0; i < samples; i++ {
		// 生成正弦波
		t := float64(i) / float64(sampleRate)
		value := amplitude * math.Sin(2*math.Pi*frequency*t)
		sample := int16(value * 32767)

		// 转换为 little-endian 16-bit PCM
		data[i*2] = byte(sample & 0xFF)
		data[i*2+1] = byte((sample >> 8) & 0xFF)
	}

	return data
}

func TestVADDetector_Basic(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultVADConfig()
	config.MinSpeechDuration = 100 * time.Millisecond
	config.MinSilenceDuration = 100 * time.Millisecond

	vad := NewVADDetector(config, logger)

	// 测试初始状态
	if vad.IsSpeech() {
		t.Error("初始状态应该是静音")
	}

	// 生成静音
	silence := generateSilence(50*time.Millisecond, 16000)
	vad.Process(silence)

	if vad.IsSpeech() {
		t.Error("静音应该被检测为静音")
	}

	// 生成语音（440Hz 正弦波，模拟人声）
	speech := generateTone(50*time.Millisecond, 16000, 440, 0.5)

	// 连续处理几帧语音
	for i := 0; i < 5; i++ {
		vad.Process(speech)
		time.Sleep(20 * time.Millisecond)
	}

	if !vad.IsSpeech() {
		t.Error("语音应该被检测为语音")
	}

	// 生成静音
	for i := 0; i < 10; i++ {
		vad.Process(silence)
		time.Sleep(20 * time.Millisecond)
	}

	if vad.IsSpeech() {
		t.Error("静音应该被检测为静音")
	}
}

func TestVADDetector_AdaptiveThreshold(t *testing.T) {
	logger := zap.NewNop()
	config := DefaultVADConfig()
	config.EnableAdaptive = true

	vad := NewVADDetector(config, logger)

	// 生成低能量噪声
	lowNoise := generateTone(50*time.Millisecond, 16000, 100, 0.05)

	// 处理几帧，让 VAD 适应背景噪声
	for i := 0; i < 10; i++ {
		vad.Process(lowNoise)
	}

	metrics := vad.GetMetrics()
	t.Logf("自适应阈值: %v", metrics["adaptive_threshold"])
	t.Logf("背景能量: %v", metrics["background_energy"])
}

func TestVADDetector_Sensitivity(t *testing.T) {
	logger := zap.NewNop()
	vad := NewVADDetector(DefaultVADConfig(), logger)

	// 测试不同灵敏度
	sensitivities := []float64{0.3, 0.5, 0.7, 0.9}

	for _, sensitivity := range sensitivities {
		vad.SetSensitivity(sensitivity)
		vad.Reset()

		// 生成中等能量的音频
		audio := generateTone(50*time.Millisecond, 16000, 440, 0.3)
		vad.Process(audio)

		t.Logf("灵敏度 %.1f: 检测到语音=%v", sensitivity, vad.IsSpeech())
	}
}

func BenchmarkVADDetector_Process(b *testing.B) {
	logger := zap.NewNop()
	vad := NewVADDetector(DefaultVADConfig(), logger)

	// 生成 20ms 的音频数据（16kHz, 16-bit）
	audio := generateTone(20*time.Millisecond, 16000, 440, 0.5)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		vad.Process(audio)
	}
}
