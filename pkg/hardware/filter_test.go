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

func TestFilterManager_ContainsEmoji(t *testing.T) {
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
			name:     "No emoji",
			text:     "Hello world",
			expected: false,
		},
		{
			name:     "Contains emoji",
			text:     "Hello 😀",
			expected: true,
		},
		{
			name:     "Multiple emojis",
			text:     "😀😃😄",
			expected: true,
		},
		{
			name:     "Emoji at end",
			text:     "Good job 👍",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fm.ContainsEmojiUnicode(tt.text)
			if result != tt.expected {
				t.Errorf("ContainsEmojiUnicode(%q) = %v, expected %v", tt.text, result, tt.expected)
			}
		})
	}
}

func TestFilterManager_RemoveEmojis(t *testing.T) {
	logger := zap.NewNop()
	fm, err := NewFilterManager(logger)
	if err != nil {
		t.Fatalf("Failed to create FilterManager: %v", err)
	}

	tests := []struct {
		name     string
		text     string
		expected string
	}{
		{
			name:     "No emoji",
			text:     "Hello world",
			expected: "Hello world",
		},
		{
			name:     "Remove single emoji",
			text:     "Hello 😀 world",
			expected: "Hello  world",
		},
		{
			name:     "Remove multiple emojis",
			text:     "😀😃😄 Hello",
			expected: " Hello",
		},
		{
			name:     "Only emojis",
			text:     "😀😃😄",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := fm.RemoveEmojisUnicode(tt.text)
			if result != tt.expected {
				t.Errorf("RemoveEmojisUnicode(%q) = %q, expected %q", tt.text, result, tt.expected)
			}
		})
	}
}

func TestFilterManager_EmojiFilteringToggle(t *testing.T) {
	logger := zap.NewNop()
	fm, err := NewFilterManager(logger)
	if err != nil {
		t.Fatalf("Failed to create FilterManager: %v", err)
	}

	text := "Hello 😀 world"

	// Emoji filtering should be enabled by default
	if !fm.IsEmojiFilteringEnabled() {
		t.Error("Emoji filtering should be enabled by default")
	}

	// Should be filtered when emoji filtering is enabled
	if !fm.IsFiltered(text) {
		t.Error("Text with emoji should be filtered when emoji filtering is enabled")
	}

	// Disable emoji filtering
	fm.SetEmojiFiltering(false)
	if fm.IsEmojiFilteringEnabled() {
		t.Error("Emoji filtering should be disabled")
	}

	// Should not be filtered when emoji filtering is disabled
	if fm.IsFiltered(text) {
		t.Error("Text with emoji should not be filtered when emoji filtering is disabled")
	}
}
