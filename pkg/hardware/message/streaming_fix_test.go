package message

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/pkg/hardware/tts"
	"go.uber.org/zap"
)

// MockStreamingTTSServiceFixed 修复后的模拟流式TTS服务
type MockStreamingTTSServiceFixed struct {
	logger *zap.Logger
}

func (m *MockStreamingTTSServiceFixed) SynthesizeStream(ctx context.Context, textChan <-chan string, callback tts.StreamingTTSCallback) error {
	m.logger.Info("开始修复后的流式TTS合成")

	var segmentIndex int
	var textBuffer strings.Builder
	var isFirstAudioSent bool

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case text, ok := <-textChan:
			if !ok {
				// 通道关闭，处理剩余文本并发送结束信号
				remaining := textBuffer.String()
				if remaining != "" {
					// 模拟TTS合成
					audioData := []byte("mock_audio_" + remaining)
					isFirst := !isFirstAudioSent
					if isFirst {
						isFirstAudioSent = true
					}
					callback(audioData, isFirst, false, segmentIndex) // 发送音频数据
					segmentIndex++
				}
				// 发送结束信号
				callback([]byte{}, false, true, segmentIndex)
				return nil
			}

			textBuffer.WriteString(text)
			currentText := textBuffer.String()

			// 简单的分割逻辑：遇到标点符号就分割
			if strings.ContainsAny(currentText, "，。！？,.!?") {
				// 模拟TTS合成延迟
				time.Sleep(10 * time.Millisecond)
				audioData := []byte("mock_audio_" + currentText)

				isFirst := !isFirstAudioSent
				if isFirst {
					isFirstAudioSent = true
				}

				callback(audioData, isFirst, false, segmentIndex)

				textBuffer.Reset()
				segmentIndex++
			}
		}
	}
}

func TestStreamingTTSStateHandling(t *testing.T) {
	logger := zap.NewNop()
	mockTTS := &MockStreamingTTSServiceFixed{logger: logger}

	// 创建文本通道
	textChan := make(chan string, 10)
	var receivedAudio []string
	var stateChanges []string

	// 回调函数
	callback := func(audioData []byte, isFirst bool, isLast bool, segmentIndex int) error {
		receivedAudio = append(receivedAudio, string(audioData))

		if isFirst {
			stateChanges = append(stateChanges, "TTS_START")
		}
		if isLast {
			stateChanges = append(stateChanges, "TTS_END")
		}

		t.Logf("收到音频片段: %s (isFirst: %v, isLast: %v, index: %d)",
			string(audioData), isFirst, isLast, segmentIndex)
		return nil
	}

	// 启动流式TTS
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		err := mockTTS.SynthesizeStream(ctx, textChan, callback)
		if err != nil {
			t.Errorf("流式TTS合成失败: %v", err)
		}
	}()

	// 模拟LLM流式输出
	testTexts := []string{"今天", "天气", "很好，", "阳光", "明媚。"}

	for _, text := range testTexts {
		textChan <- text
		time.Sleep(20 * time.Millisecond) // 模拟LLM输出间隔
	}

	close(textChan)
	time.Sleep(100 * time.Millisecond) // 等待处理完成

	// 验证状态变化
	if len(stateChanges) < 2 {
		t.Errorf("期望至少有TTS_START和TTS_END状态变化，实际: %v", stateChanges)
	}

	if stateChanges[0] != "TTS_START" {
		t.Errorf("第一个状态变化应该是TTS_START，实际: %s", stateChanges[0])
	}

	if stateChanges[len(stateChanges)-1] != "TTS_END" {
		t.Errorf("最后一个状态变化应该是TTS_END，实际: %s", stateChanges[len(stateChanges)-1])
	}

	// 验证音频数据
	if len(receivedAudio) == 0 {
		t.Error("没有收到任何音频数据")
	}

	t.Logf("状态变化: %v", stateChanges)
	t.Logf("总共收到 %d 个音频片段", len(receivedAudio))
	for i, audio := range receivedAudio {
		t.Logf("片段 %d: %s", i, audio)
	}
}

func TestStreamingTTSEndSignal(t *testing.T) {
	logger := zap.NewNop()
	mockTTS := &MockStreamingTTSServiceFixed{logger: logger}

	textChan := make(chan string, 10)
	var endSignalReceived bool

	callback := func(audioData []byte, isFirst bool, isLast bool, segmentIndex int) error {
		if isLast {
			endSignalReceived = true
			t.Logf("收到TTS结束信号: isLast=%v, segmentIndex=%d, dataLen=%d",
				isLast, segmentIndex, len(audioData))
		}
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	go func() {
		err := mockTTS.SynthesizeStream(ctx, textChan, callback)
		if err != nil {
			t.Errorf("流式TTS合成失败: %v", err)
		}
	}()

	// 发送一些文本然后关闭通道
	textChan <- "测试文本。"
	time.Sleep(50 * time.Millisecond)
	close(textChan)

	// 等待处理完成
	time.Sleep(100 * time.Millisecond)

	if !endSignalReceived {
		t.Error("没有收到TTS结束信号")
	}
}
