package hardware

import (
	"fmt"
	"strings"
	"sync"

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
	m.logger.Info(fmt.Sprintf("过滤词管理器初始化成功，使用默认黑名单 count:%d", len(m.blacklist)))
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
	m.logger.Info(fmt.Sprintf("记录被过滤的词：%s，当前数量：%d", cleaned, m.counts[cleaned]))
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
