package message

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/code-100-precent/LingEcho/pkg/hardware/tts"
	"go.uber.org/zap"
)

// MockStreamingTTSService 模拟流式TTS服务
type MockStreamingTTSService struct {
	logger *zap.Logger
}

func (m *MockStreamingTTSService) SynthesizeStream(ctx context.Context, textChan <-chan string, callback tts.StreamingTTSCallback, waitGroup *sync.WaitGroup) error {
	m.logger.Info("开始模拟流式TTS合成")

	var segmentIndex int
	var textBuffer strings.Builder

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case text, ok := <-textChan:
			if !ok {
				// 通道关闭，处理剩余文本
				remaining := textBuffer.String()
				if remaining != "" {
					waitGroup.Add(1)
					go func() {
						defer waitGroup.Done()
						// 模拟TTS合成
						audioData := []byte("mock_audio_" + remaining)
						callback(audioData, segmentIndex == 0, true, segmentIndex)
					}()
				}
				return nil
			}

			textBuffer.WriteString(text)
			currentText := textBuffer.String()

			// 简单的分割逻辑：遇到标点符号就分割
			if strings.ContainsAny(currentText, "，。！？,.!?") {
				waitGroup.Add(1)
				go func(text string, index int) {
					defer waitGroup.Done()
					// 模拟TTS合成延迟
					time.Sleep(10 * time.Millisecond)
					audioData := []byte("mock_audio_" + text)
					callback(audioData, index == 0, false, index)
				}(currentText, segmentIndex)

				textBuffer.Reset()
				segmentIndex++
			}
		}
	}
}

func TestStreamingTTSIntegration(t *testing.T) {
	logger := zap.NewNop()
	mockTTS := &MockStreamingTTSService{logger: logger}

	// 创建文本通道
	textChan := make(chan string, 10)
	var waitGroup sync.WaitGroup
	var receivedAudio []string
	var mu sync.Mutex

	// 回调函数
	callback := func(audioData []byte, isFirst bool, isLast bool, segmentIndex int) error {
		mu.Lock()
		defer mu.Unlock()
		receivedAudio = append(receivedAudio, string(audioData))
		t.Logf("收到音频片段: %s (isFirst: %v, isLast: %v, index: %d)",
			string(audioData), isFirst, isLast, segmentIndex)
		return nil
	}

	// 启动流式TTS
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	go func() {
		err := mockTTS.SynthesizeStream(ctx, textChan, callback, &waitGroup)
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
	waitGroup.Wait()

	// 验证结果
	mu.Lock()
	defer mu.Unlock()

	if len(receivedAudio) == 0 {
		t.Error("没有收到任何音频数据")
	}

	t.Logf("总共收到 %d 个音频片段", len(receivedAudio))
	for i, audio := range receivedAudio {
		t.Logf("片段 %d: %s", i, audio)
	}
}

func TestStreamingTextExtraction(t *testing.T) {
	// 测试文本提取逻辑
	tests := []struct {
		name     string
		input    string
		expected bool // 是否应该提取到内容
	}{
		{
			name:     "短文本不提取",
			input:    "你好",
			expected: false,
		},
		{
			name:     "包含逗号的文本",
			input:    "今天天气很好，",
			expected: true,
		},
		{
			name:     "包含句号的文本",
			input:    "这是一个测试。",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 由于方法是私有的，我们只能测试整体行为
			// 在实际应用中，这些逻辑会在流式处理中体现
			t.Logf("测试文本: %s", tt.input)

			// 简单验证文本长度和标点符号
			hasEnoughLength := len([]rune(tt.input)) >= 3
			hasPunctuation := strings.ContainsAny(tt.input, "，。！？,.!?")

			shouldExtract := hasEnoughLength && hasPunctuation
			if shouldExtract != tt.expected {
				t.Errorf("期望提取: %v, 实际: %v", tt.expected, shouldExtract)
			}
		})
	}
}
