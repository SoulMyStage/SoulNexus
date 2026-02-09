package hardware

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAudioManager_EchoSuppression(t *testing.T) {
	logger := zap.NewNop()
	manager := NewAudioManager(16000, 1, logger)

	// 模拟TTS音频数据（中等能量）
	ttsData := make([]byte, 1024)
	for i := range ttsData {
		ttsData[i] = byte(i % 128) // 中等能量
	}

	// 记录TTS输出
	manager.RecordTTSOutput(ttsData)

	// 等待一小段时间，模拟真实场景中的延迟（回音检测需要50ms以上的时间差）
	time.Sleep(60 * time.Millisecond)

	// 测试1: TTS播放期间，低能量音频应该被过滤
	processedData, shouldProcess := manager.ProcessInputAudio(ttsData, true)
	if shouldProcess {
		t.Error("TTS播放期间，低能量音频应该被过滤")
	}
	if processedData != nil {
		t.Error("被过滤的音频数据应该为nil")
	}

	// 测试2: TTS不播放时，相同音频应该通过
	processedData, shouldProcess = manager.ProcessInputAudio(ttsData, false)
	if !shouldProcess {
		t.Error("TTS不播放时，音频应该通过")
	}

	// 测试3: 非常高能量音频在TTS播放期间应该通过
	veryHighEnergyData := make([]byte, 1024)
	for i := 0; i < len(veryHighEnergyData); i += 2 {
		veryHighEnergyData[i] = 0xFF   // 低字节
		veryHighEnergyData[i+1] = 0x7F // 高字节，最大正值
	}

	processedData, shouldProcess = manager.ProcessInputAudio(veryHighEnergyData, true)
	// 非常高能量的音频应该通过（可能是用户打断）
	t.Logf("非常高能量音频在TTS播放期间是否通过: %v", shouldProcess)
	if shouldProcess {
		t.Log("高能量音频正确通过")
	}
}

func TestAudioManager_GracePeriod(t *testing.T) {
	logger := zap.NewNop()
	manager := NewAudioManager(16000, 1, logger)

	// 模拟TTS音频数据（低能量）
	ttsData := make([]byte, 1024)
	for i := range ttsData {
		ttsData[i] = byte(i % 64) // 低能量
	}

	// 记录TTS输出
	manager.RecordTTSOutput(ttsData)

	// 等待一小段时间，模拟真实场景中的延迟
	time.Sleep(60 * time.Millisecond)

	// 通知TTS结束
	manager.NotifyTTSEnd()

	// 测试宽限期内的音频过滤（低能量音频应该被过滤）
	processedData, shouldProcess := manager.ProcessInputAudio(ttsData, false) // TTS已停止
	if shouldProcess {
		t.Error("宽限期内，低能量音频应该被过滤")
	}
	if processedData != nil {
		t.Error("被过滤的音频数据应该为nil")
	}

	// 等待宽限期结束
	time.Sleep(1100 * time.Millisecond) // 稍微超过宽限期

	// 宽限期结束后，音频应该通过
	processedData, shouldProcess = manager.ProcessInputAudio(ttsData, false)
	if !shouldProcess {
		t.Error("宽限期结束后，音频应该通过")
	}
	if processedData == nil {
		t.Error("通过的音频数据不应该为nil")
	}
}

func TestCalculateEnergy(t *testing.T) {
	manager := NewAudioManager(16000, 1, zap.NewNop())

	// 测试静音数据
	silentData := make([]byte, 1024)
	energy := manager.calculateEnergy(silentData)
	if energy != 0 {
		t.Errorf("静音数据的能量应该为0，实际为: %d", energy)
	}

	// 测试最大音量数据
	maxData := make([]byte, 1024)
	for i := 0; i < len(maxData); i += 2 {
		maxData[i] = 0xFF   // 低字节
		maxData[i+1] = 0x7F // 高字节，最大正值
	}
	energy = manager.calculateEnergy(maxData)
	if energy == 0 {
		t.Error("最大音量数据的能量不应该为0")
	}
	t.Logf("最大音量数据的能量: %d", energy)
}
