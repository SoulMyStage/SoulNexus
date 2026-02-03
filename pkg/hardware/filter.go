package hardware

import (
	"regexp"
	"strings"
	"sync"
	"unicode"

	"go.uber.org/zap"
)

// FilterManager filter manager
type FilterManager struct {
	blacklist    map[string]bool // black list
	counts       map[string]int  // filter counts
	mu           sync.RWMutex
	logger       *zap.Logger
	emojiRegex   *regexp.Regexp // emoji regex pattern
	filterEmojis bool           // whether to filter emojis
}

// NewFilterManager create filter manager
func NewFilterManager(logger *zap.Logger) (*FilterManager, error) {
	m := &FilterManager{
		blacklist:    make(map[string]bool),
		counts:       make(map[string]int),
		logger:       logger,
		filterEmojis: true, // default to filter emojis
	}

	// Initialize emoji regex pattern
	if err := m.initEmojiRegex(); err != nil {
		return nil, err
	}

	m.loadDefaultBlacklist()
	m.logger.Info("过滤词管理器初始化成功，使用默认黑名单",
		zap.Int("count", len(m.blacklist)),
		zap.Bool("filterEmojis", m.filterEmojis),
	)

	return m, nil
}

// loadDefaultBlacklist load default black list
func (m *FilterManager) loadDefaultBlacklist() {
	defaultWords := []string{
		"嗯", "嗯。", "嗯嗯", "嗯嗯。",
		"啊", "啊。", "啊啊", "啊啊。",
		"呃", "呃。", "呃呃", "呃呃。",
		"额", "额。", "额额", "额额。",
		"哦", "哦。", "哦哦", "哦哦。",
		"噢", "噢。",
		"呀", "呀。",
		"哈", "哈。", "哈哈", "哈哈。",
		"嘿", "嘿。",
		"喂", "喂。",
		"哼", "哼。",
		"唉", "唉。",
		"哎", "哎。",
		"诶", "诶。",
		"欸", "欸。",
	}

	for _, word := range defaultWords {
		m.blacklist[word] = true
		m.blacklist[strings.ToLower(word)] = true
	}
}

// IsFiltered check if text is filtered
func (m *FilterManager) IsFiltered(text string) bool {
	if text == "" {
		return true
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	cleaned := strings.TrimSpace(text)
	cleaned = strings.Trim(cleaned, "。，、；：？！\"\"''（）【】《》")

	// Check blacklist
	if m.blacklist[cleaned] || m.blacklist[strings.ToLower(cleaned)] {
		return true
	}

	// Check emoji filtering
	if m.filterEmojis {
		// Filter if text contains only emojis
		if m.IsEmojiOnly(text) {
			return true
		}

		// Filter if text contains emojis (optional: you can make this configurable)
		if m.ContainsEmojiUnicode(text) {
			m.logger.Debug("文本包含emoji，已过滤", zap.String("text", text))
			return true
		}
	}

	return false
}

// RecordFiltered record filtered text
func (m *FilterManager) RecordFiltered(text string) {
	if text == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cleaned := strings.TrimSpace(text)
	cleaned = strings.Trim(cleaned, "。，、；：？！\"\"''（）【】《》")

	m.counts[cleaned]++

	m.logger.Debug("记录被过滤的词",
		zap.String("text", cleaned),
		zap.Int("count", m.counts[cleaned]),
	)
}

// GetFilteredCount get filtered count
func (m *FilterManager) GetFilteredCount(text string) int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	cleaned := strings.TrimSpace(text)
	cleaned = strings.Trim(cleaned, "。，、；：？！\"\"''（）【】《》")
	return m.counts[cleaned]
}

// GetAllCounts get all filtered counts
func (m *FilterManager) GetAllCounts() map[string]int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]int)
	for k, v := range m.counts {
		result[k] = v
	}
	return result
}

// initEmojiRegex initialize emoji regex pattern
func (m *FilterManager) initEmojiRegex() error {
	// Comprehensive emoji regex pattern covering all Unicode emoji ranges
	emojiPattern := `[\x{1F600}-\x{1F64F}]|` + // Emoticons
		`[\x{1F300}-\x{1F5FF}]|` + // Misc Symbols and Pictographs
		`[\x{1F680}-\x{1F6FF}]|` + // Transport and Map
		`[\x{1F1E0}-\x{1F1FF}]|` + // Regional indicator symbols
		`[\x{2600}-\x{26FF}]|` + // Misc symbols
		`[\x{2700}-\x{27BF}]|` + // Dingbats
		`[\x{1F900}-\x{1F9FF}]|` + // Supplemental Symbols and Pictographs
		`[\x{1F018}-\x{1F270}]|` + // Various asian symbols
		`[\x{238C}-\x{2454}]|` + // Misc items
		`[\x{20D0}-\x{20FF}]|` + // Combining Diacritical Marks for Symbols
		`[\x{FE00}-\x{FE0F}]|` + // Variation Selectors
		`[\x{1F004}]|[\x{1F0CF}]|` + // Mahjong/Playing cards
		`[\x{1F170}-\x{1F251}]` // Enclosed characters

	var err error
	m.emojiRegex, err = regexp.Compile(emojiPattern)
	if err != nil {
		m.logger.Error("初始化emoji正则表达式失败", zap.Error(err))
		return err
	}

	m.logger.Info("emoji正则表达式初始化成功")
	return nil
}

// SetEmojiFiltering enable or disable emoji filtering
func (m *FilterManager) SetEmojiFiltering(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.filterEmojis = enabled
	m.logger.Info("emoji过滤设置已更新", zap.Bool("enabled", enabled))
}

// IsEmojiFilteringEnabled check if emoji filtering is enabled
func (m *FilterManager) IsEmojiFilteringEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.filterEmojis
}

// ContainsEmoji check if text contains emoji
func (m *FilterManager) ContainsEmoji(text string) bool {
	if m.emojiRegex == nil {
		return false
	}
	return m.emojiRegex.MatchString(text)
}

// ContainsEmojiUnicode check if text contains emoji using Unicode ranges
func (m *FilterManager) ContainsEmojiUnicode(text string) bool {
	for _, r := range text {
		if m.isEmojiRune(r) {
			return true
		}
	}
	return false
}

// isEmojiRune check if a rune is an emoji
func (m *FilterManager) isEmojiRune(r rune) bool {
	// Check various Unicode emoji ranges
	return (r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F1E0 && r <= 0x1F1FF) || // Regional indicator symbols
		(r >= 0x2600 && r <= 0x26FF) || // Misc symbols
		(r >= 0x2700 && r <= 0x27BF) || // Dingbats
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental Symbols and Pictographs
		(r >= 0x1F018 && r <= 0x1F270) || // Various asian symbols
		(r >= 0x238C && r <= 0x2454) || // Misc items
		(r >= 0x20D0 && r <= 0x20FF) || // Combining Diacritical Marks for Symbols
		(r >= 0xFE00 && r <= 0xFE0F) || // Variation Selectors
		r == 0x1F004 || r == 0x1F0CF || // Mahjong/Playing cards
		(r >= 0x1F170 && r <= 0x1F251) || // Enclosed characters
		unicode.Is(unicode.So, r) // Other symbols
}

// RemoveEmojis remove all emojis from text
func (m *FilterManager) RemoveEmojis(text string) string {
	if m.emojiRegex == nil {
		return text
	}
	return m.emojiRegex.ReplaceAllString(text, "")
}

// RemoveEmojisUnicode remove all emojis from text using Unicode check
func (m *FilterManager) RemoveEmojisUnicode(text string) string {
	var result strings.Builder
	for _, r := range text {
		if !m.isEmojiRune(r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// IsEmojiOnly check if text contains only emojis and whitespace
func (m *FilterManager) IsEmojiOnly(text string) bool {
	cleaned := strings.TrimSpace(text)
	if cleaned == "" {
		return false
	}

	for _, r := range cleaned {
		if !m.isEmojiRune(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}
