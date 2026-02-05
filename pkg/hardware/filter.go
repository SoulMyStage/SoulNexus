package hardware

import (
	"strings"
	"sync"
	"unicode"

	"go.uber.org/zap"
)

// FilterManager filter manager
type FilterManager struct {
	blacklist map[string]bool // black list
	counts    map[string]int  // filter counts
	mu        sync.RWMutex
	logger    *zap.Logger
}

// NewFilterManager create filter manager
func NewFilterManager(logger *zap.Logger) (*FilterManager, error) {
	m := &FilterManager{
		blacklist: make(map[string]bool),
		counts:    make(map[string]int),
		logger:    logger,
	}
	m.loadDefaultBlacklist()
	m.logger.Info("过滤词管理器初始化成功，使用默认黑名单",
		zap.Int("count", len(m.blacklist)),
	)

	return m, nil
}

// containsEmoji 检查文本是否包含emoji
func (m *FilterManager) containsEmoji(text string) bool {
	for _, r := range text {
		// 检查是否为emoji字符
		// Emoji通常在以下Unicode范围内：
		// U+1F600-U+1F64F (Emoticons)
		// U+1F300-U+1F5FF (Misc Symbols and Pictographs)
		// U+1F680-U+1F6FF (Transport and Map)
		// U+1F1E0-U+1F1FF (Regional indicators)
		// U+2600-U+26FF (Misc symbols)
		// U+2700-U+27BF (Dingbats)
		if (r >= 0x1F600 && r <= 0x1F64F) ||
			(r >= 0x1F300 && r <= 0x1F5FF) ||
			(r >= 0x1F680 && r <= 0x1F6FF) ||
			(r >= 0x1F1E0 && r <= 0x1F1FF) ||
			(r >= 0x2600 && r <= 0x26FF) ||
			(r >= 0x2700 && r <= 0x27BF) ||
			(r >= 0xFE00 && r <= 0xFE0F) || // Variation Selectors
			(r >= 0x1F900 && r <= 0x1F9FF) { // Supplemental Symbols and Pictographs
			return true
		}

		// 检查其他可能的emoji字符
		if unicode.Is(unicode.So, r) || unicode.Is(unicode.Sm, r) {
			// 一些emoji可能被归类为符号
			if r > 0x1F000 {
				return true
			}
		}
	}
	return false
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

	// Check if contains emoji
	if m.containsEmoji(text) {
		return true
	}

	// Check blacklist
	if m.blacklist[cleaned] || m.blacklist[strings.ToLower(cleaned)] {
		return true
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
