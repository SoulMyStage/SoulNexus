package hardware

import (
	"testing"

	"go.uber.org/zap"
)

func TestFilterManager_EmojiFiltering(t *testing.T) {
	logger := zap.NewNop()
	fm, err := NewFilterManager(logger)
	if err != nil {
		t.Fatalf("Failed to create FilterManager: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{
			name:     "Pure text should not be filtered",
			text:     "Hello world",
			expected: false,
		},
		{
			name:     "Text with emoji should be filtered",
			text:     "Hello 😀 world",
			expected: true,
		},
		{
			name:     "Only emoji should be filtered",
			text:     "😀😃😄",
			expected: true,
		},
		{
			name:     "Mixed emoji and symbols",
			text:     "🚀🔥💻",
			expected: true,
		},
		{
			name:     "Heart emoji",
			text:     "❤️",
			expected: true,
		},
		{
			name:     "Thumbs up",
			text:     "👍",
			expected: true,
		},
		{
			name:     "Empty text should be filtered",
			text:     "",
			expected: true,
		},
		{
			name:     "Whitespace only",
			text:     "   ",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fm.IsFiltered(tt.text)
			if result != tt.expected {
				t.Errorf("IsFiltered(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}
