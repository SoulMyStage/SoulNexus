package sessions

import (
	"testing"
)

func TestASRStateManager_UpdateASRText_Final(t *testing.T) {
	manager := NewASRStateManager()

	// 测试最终结果
	result := manager.UpdateASRText("你好，世界", true)
	if result != "你好，世界" {
		t.Errorf("Expected '你好，世界', got '%s'", result)
	}

	// 相同文本应该被过滤
	result = manager.UpdateASRText("你好，世界", true)
	if result != "" {
		t.Errorf("Expected empty string for duplicate, got '%s'", result)
	}

	// 新文本应该返回增量
	result = manager.UpdateASRText("你好，世界！很高兴见到你", true)
	if result == "" {
		t.Error("Expected incremental text, got empty string")
	}
}

func TestASRStateManager_UpdateASRText_Intermediate(t *testing.T) {
	manager := NewASRStateManager()

	// 中间结果：不完整的句子应该被过滤
	result := manager.UpdateASRText("你好", false)
	if result != "" {
		t.Errorf("Expected empty for incomplete sentence, got '%s'", result)
	}

	// 中间结果：完整的句子应该返回
	result = manager.UpdateASRText("你好，世界。", false)
	if result == "" {
		t.Error("Expected complete sentence, got empty string")
	}

	// 继续累积
	result = manager.UpdateASRText("你好，世界。今天天气不错", false)
	if result != "" {
		t.Errorf("Expected empty for incomplete continuation, got '%s'", result)
	}

	// 新的完整句子
	result = manager.UpdateASRText("你好，世界。今天天气不错。", false)
	if result == "" {
		t.Error("Expected new complete sentence, got empty string")
	}
}

func TestASRStateManager_IncompleteThenComplete(t *testing.T) {
	manager := NewASRStateManager()

	// 第一次：不完整的句子（没有结束符）
	result := manager.UpdateASRText("喂喂喂可以听到我说话吗", false)
	if result != "" {
		t.Errorf("Expected empty for incomplete sentence, got '%s'", result)
	}

	// 第二次：加了标点符号，但内容相似，应该被过滤
	result = manager.UpdateASRText("喂喂喂，可以听到我说话吗？", false)
	if result != "" {
		t.Errorf("Expected empty for similar text with punctuation, got '%s'", result)
	}

	// 验证状态已更新（保留了最新版本）
	if manager.lastProcessedCumulativeText != "喂喂喂，可以听到我说话吗？" {
		t.Errorf("Expected lastProcessedCumulativeText to be updated to latest version, got '%s'",
			manager.lastProcessedCumulativeText)
	}
}

func TestASRStateManager_Clear(t *testing.T) {
	manager := NewASRStateManager()

	// 设置一些状态
	manager.UpdateASRText("你好，世界", true)

	// 清空
	manager.Clear()

	// 应该能再次处理相同的文本
	result := manager.UpdateASRText("你好，世界", true)
	if result != "你好，世界" {
		t.Errorf("Expected '你好，世界' after clear, got '%s'", result)
	}
}

func TestNormalizeTextFast(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"你好，世界！", "你好世界"},
		{"Hello World", "HeloWorld"}, // 'l' 重复会被去重
		{"  空格  测试  ", "空格测试"},
		{"123abc中文", "123abc中文"},
		{"", ""},
		{"aabbcc", "abc"}, // 连续重复字符会被去重
	}

	for _, tt := range tests {
		result := normalizeTextFast(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeTextFast(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestCalculateSimilarityFast(t *testing.T) {
	tests := []struct {
		text1  string
		text2  string
		minSim float64
		maxSim float64
	}{
		{"你好", "你好", 1.0, 1.0},
		{"", "", 1.0, 1.0},
		{"你好", "", 0.0, 0.0},
		{"你好世界", "你好", 0.4, 0.6},
		{"完全不同", "totally different", 0.0, 0.1},
	}

	for _, tt := range tests {
		result := calculateSimilarityFast(tt.text1, tt.text2)
		if result < tt.minSim || result > tt.maxSim {
			t.Errorf("calculateSimilarityFast(%q, %q) = %f, want between %f and %f",
				tt.text1, tt.text2, result, tt.minSim, tt.maxSim)
		}
	}
}
