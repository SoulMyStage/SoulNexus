package hardware

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

// StateManager status manager interface
type StateManager interface {
	// SetProcessing set process status
	SetProcessing(processing bool)

	// IsProcessing check if is processing
	IsProcessing() bool

	// SetTTSPlaying set tts play status
	SetTTSPlaying(playing bool)

	// IsTTSPlaying check tts if is playing
	IsTTSPlaying() bool

	// SetFatalError set fatal error status
	SetFatalError(fatal bool)

	// IsFatalError check if has fatal error
	IsFatalError() bool

	// UpdateASRText update asr text and return increment text
	UpdateASRText(text string, isLast bool) string

	// Clear clear status
	Clear()
}

// HardwareStateManager hardware status manager
type HardwareStateManager struct {
	mu                          sync.RWMutex
	processing                  bool
	ttsPlaying                  bool
	fatalError                  bool
	goodbyePending              bool // 新增：goodbye待处理标志
	lastASRText                 string
	lastProcessedText           string
	lastProcessedCumulativeText string
	asrCompleteTime             time.Time
	ttsCtx                      context.Context
	ttsCancel                   context.CancelFunc

	// optimization fields
	sentenceEndings []rune // sentence ending characters
}

// NewHardwareStateManager create status manager
func NewHardwareStateManager() *HardwareStateManager {
	return &HardwareStateManager{
		sentenceEndings: []rune{'。', '！', '？', '.', '!', '?'},
	}
}

// SetProcessing set process status
func (m *HardwareStateManager) SetProcessing(processing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processing = processing
	if processing {
		m.asrCompleteTime = time.Now()
	}
}

// IsProcessing check if is process
func (m *HardwareStateManager) IsProcessing() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.processing
}

// SetTTSPlaying set tts status
func (m *HardwareStateManager) SetTTSPlaying(playing bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ttsPlaying = playing
}

// IsTTSPlaying check if is tts playing
func (m *HardwareStateManager) IsTTSPlaying() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ttsPlaying
}

// SetFatalError set fatal error status
func (m *HardwareStateManager) SetFatalError(fatal bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fatalError = fatal
}

// IsFatalError check if has fatal error
func (m *HardwareStateManager) IsFatalError() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.fatalError
}

// SetGoodbyePending 设置goodbye待处理状态
func (m *HardwareStateManager) SetGoodbyePending(pending bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.goodbyePending = pending
}

// IsGoodbyePending 检查是否有goodbye待处理
func (m *HardwareStateManager) IsGoodbyePending() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.goodbyePending
}

// CanProcess check if can handle new request
// return (canProcess, isFatalError, isProcessing)
func (m *HardwareStateManager) CanProcess() (bool, bool, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	canProcess := !m.fatalError && !m.processing
	return canProcess, m.fatalError, m.processing
}

// UpdateASRText update asr text and return increment text
func (m *HardwareStateManager) UpdateASRText(text string, isLast bool) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if text == "" {
		return ""
	}

	m.lastASRText = text

	if isLast {
		if text == m.lastProcessedText {
			return ""
		}
		incremental := m.extractIncremental(text)
		if incremental == "" {
			return ""
		}
		m.lastProcessedText = text
		m.lastProcessedCumulativeText = text
		return incremental
	}

	newSentences := m.extractNewSentences(text)
	if newSentences != "" {
		lastProcessed := m.lastProcessedCumulativeText
		if lastProcessed == "" {
			if endIdx := m.findLastSentenceEnding(text); endIdx >= 0 {
				m.lastProcessedCumulativeText = text[:endIdx+1]
				return newSentences
			}
			m.lastProcessedCumulativeText = text
		} else {
			processedText := lastProcessed + newSentences
			if len(processedText) <= len(text) && strings.HasPrefix(text, processedText) {
				m.lastProcessedCumulativeText = processedText
			} else {
				if endIdx := m.findLastSentenceEnding(text); endIdx >= 0 {
					m.lastProcessedCumulativeText = text[:endIdx+1]
					return newSentences
				}
				m.lastProcessedCumulativeText = text
			}
		}
		return newSentences
	}
	return ""
}

// extractIncremental extract increment text
func (m *HardwareStateManager) extractIncremental(current string) string {
	if m.lastProcessedCumulativeText == "" {
		return current
	}

	if current == m.lastProcessedCumulativeText {
		return ""
	}

	normalizedLast := normalizeTextFast(m.lastProcessedCumulativeText)
	normalizedCurrent := normalizeTextFast(current)

	if normalizedCurrent == normalizedLast {
		return ""
	}

	similarity := calculateSimilarityFast(normalizedCurrent, normalizedLast)
	if similarity > TextSimilarityThreshold {
		return ""
	}

	if strings.HasPrefix(current, m.lastProcessedCumulativeText) {
		incremental := current[len(m.lastProcessedCumulativeText):]
		normalizedIncremental := normalizeTextFast(incremental)
		if normalizedIncremental == "" {
			return ""
		}

		if len(normalizedIncremental) < len(normalizedLast)/2 {
			incSimilarity := calculateSimilarityFast(normalizedIncremental, normalizedLast)
			if incSimilarity > TextSimilarityThreshold {
				return ""
			}
		}
		return strings.TrimSpace(incremental)
	}

	lastSentence := m.extractLastSentence(current)
	if lastSentence != "" && lastSentence != m.lastProcessedCumulativeText {
		normalizedLastSentence := normalizeTextFast(lastSentence)
		if normalizedLastSentence != normalizedLast {
			similarity := calculateSimilarityFast(normalizedLastSentence, normalizedLast)
			if similarity <= TextSimilarityThreshold {
				return lastSentence
			}
		}
	}
	return current
}

// SetTTSCtx set tts context
func (m *HardwareStateManager) SetTTSCtx(ctx context.Context, cancel context.CancelFunc) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ttsCancel != nil {
		m.ttsCancel()
	}
	m.ttsCtx = ctx
	m.ttsCancel = cancel
}

// GetTTSCtx get tts context
func (m *HardwareStateManager) GetTTSCtx() context.Context {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.ttsCtx
}

// CancelTTS cancel tts
func (m *HardwareStateManager) CancelTTS() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.ttsCancel != nil {
		m.ttsCancel()
		m.ttsCancel = nil
		m.ttsCtx = nil
	}
}

// GetASRCompleteTime get asr complete time
func (m *HardwareStateManager) GetASRCompleteTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.asrCompleteTime
}

// Clear clear status
func (m *HardwareStateManager) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processing = false
	m.ttsPlaying = false
	m.fatalError = false
	m.goodbyePending = false // 新增：清空goodbye待处理标志
	m.lastASRText = ""
	m.lastProcessedText = ""
	m.lastProcessedCumulativeText = ""

	if m.ttsCancel != nil {
		m.ttsCancel()
		m.ttsCancel = nil
		m.ttsCtx = nil
	}
}

// extractNewSentences get new sentences from cumulative text
func (m *HardwareStateManager) extractNewSentences(current string) string {
	if current == "" {
		return ""
	}

	lastProcessed := m.lastProcessedCumulativeText
	if lastProcessed == "" {
		if m.isCompleteSentence(current) {
			return current
		}
		return ""
	}

	if !strings.HasPrefix(current, lastProcessed) {
		if m.isCompleteSentence(current) {
			return current
		}
		return ""
	}

	newText := current[len(lastProcessed):]
	if newText == "" {
		return ""
	}

	// find sentence ending
	for i, r := range newText {
		for _, ending := range m.sentenceEndings {
			if r == ending {
				return newText[:i+1]
			}
		}
	}

	return ""
}

// isCompleteSentence check if is complete sentence
func (m *HardwareStateManager) isCompleteSentence(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}

	for _, r := range text {
		for _, ending := range m.sentenceEndings {
			if r == ending {
				return true
			}
		}
	}
	return false
}

// findLastSentenceEnding find last sentence ending position
func (m *HardwareStateManager) findLastSentenceEnding(text string) int {
	for i := len(text) - 1; i >= 0; i-- {
		r, _ := utf8.DecodeRuneInString(text[i:])
		for _, ending := range m.sentenceEndings {
			if r == ending {
				return i
			}
		}
	}
	return -1
}

// extractLastSentence extract last sentence
func (m *HardwareStateManager) extractLastSentence(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	lastIndex := m.findLastSentenceEnding(text)
	if lastIndex < 0 {
		return text
	}

	// find sentence start position
	sentenceStart := 0
	for i := lastIndex - 1; i >= 0; i-- {
		r, size := utf8.DecodeRuneInString(text[i:])
		for _, ending := range m.sentenceEndings {
			if r == ending {
				sentenceStart = i + size
				// skip whitespace
				for sentenceStart < len(text) && (text[sentenceStart] == ' ' || text[sentenceStart] == '\t') {
					sentenceStart++
				}
				result := strings.TrimSpace(text[sentenceStart : lastIndex+1])
				if result != "" {
					return result
				}
				return text[sentenceStart : lastIndex+1]
			}
		}
	}

	result := strings.TrimSpace(text[:lastIndex+1])
	if result != "" {
		return result
	}
	return text[:lastIndex+1]
}

// normalizeTextFast normalize text for comparison
func normalizeTextFast(text string) string {
	if text == "" {
		return ""
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	var result strings.Builder
	result.Grow(len(text))

	var lastChar rune
	var hasLastChar bool

	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.IsLetter(r) || unicode.IsNumber(r) {
			if !hasLastChar || r != lastChar {
				result.WriteRune(r)
				lastChar = r
				hasLastChar = true
			}
		}
	}

	return result.String()
}

// calculateSimilarityFast calculate similarity with optimization
func calculateSimilarityFast(text1, text2 string) float64 {
	if text1 == "" && text2 == "" {
		return 1.0
	}
	if text1 == "" || text2 == "" {
		return 0.0
	}
	if text1 == text2 {
		return 1.0
	}

	len1, len2 := len(text1), len(text2)
	maxLen := len1
	if len2 > maxLen {
		maxLen = len2
	}

	if maxLen == 0 {
		return 1.0
	}

	// for very short strings, use simple comparison
	if maxLen <= 3 {
		if text1 == text2 {
			return 1.0
		}
		return 0.0
	}

	// for strings with big length difference, return low similarity
	if abs(len1-len2) > maxLen/2 {
		return 0.0
	}

	distance := levenshteinDistanceFast(text1, text2)
	return 1.0 - float64(distance)/float64(maxLen)
}

// levenshteinDistanceFast calculate levenshtein distance with optimization
func levenshteinDistanceFast(s1, s2 string) int {
	len1, len2 := len(s1), len(s2)

	if len1 == 0 {
		return len2
	}
	if len2 == 0 {
		return len1
	}
	if s1 == s2 {
		return 0
	}

	// use single row optimization
	if len1 > len2 {
		s1, s2 = s2, s1
		len1, len2 = len2, len1
	}

	prevRow := make([]int, len1+1)
	currRow := make([]int, len1+1)

	for i := 0; i <= len1; i++ {
		prevRow[i] = i
	}

	for i := 1; i <= len2; i++ {
		currRow[0] = i
		for j := 1; j <= len1; j++ {
			cost := 0
			if s2[i-1] != s1[j-1] {
				cost = 1
			}

			currRow[j] = min3(
				currRow[j-1]+1,    // insertion
				prevRow[j]+1,      // deletion
				prevRow[j-1]+cost, // substitution
			)
		}
		prevRow, currRow = currRow, prevRow
	}

	return prevRow[len1]
}
