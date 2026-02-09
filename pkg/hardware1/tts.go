package hardware

import (
	"context"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/code-100-precent/LingEcho/pkg/synthesizer"
	"go.uber.org/zap"
)

// TTSService TTS服务实现
type TTSService struct {
	ctx                context.Context
	cancel             context.CancelFunc
	speaker            string
	synthesizer        synthesizer.SynthesisService
	errorHandler       *ErrHandler
	logger             *zap.Logger
	mu                 sync.RWMutex
	closed             bool
	enableTextSplit    bool           // 是否启用文本分割
	firstSegmentMinLen int            // 第一段最小长度
	firstSegmentMaxLen int            // 第一段最大长度
	minSplitLength     int            // 最小分割长度
	activeGoroutines   sync.WaitGroup // 跟踪活跃的goroutine
}

// TextSplitConfig 文本分割配置
type TextSplitConfig struct {
	Enable             bool `json:"enable"`                // 是否启用文本分割
	FirstSegmentMinLen int  `json:"first_segment_min_len"` // 第一段最小长度，默认3个字符
	FirstSegmentMaxLen int  `json:"first_segment_max_len"` // 第一段最大长度，默认5个字符
	MinSplitLength     int  `json:"min_split_length"`      // 总体最小分割长度，默认6个字符
}

// TextSegment 文本片段
type TextSegment struct {
	Text     string `json:"text"`     // 文本内容
	Index    int    `json:"index"`    // 片段索引（0表示第一部分，1表示第二部分）
	IsLast   bool   `json:"is_last"`  // 是否是最后一个片段
	Priority int    `json:"priority"` // 优先级（数字越小优先级越高）
}

// NewTTSService 创建TTS服务
func NewTTSService(
	ctx context.Context,
	speaker string,
	synthesizer synthesizer.SynthesisService,
	errorHandler *ErrHandler,
	logger *zap.Logger,
) *TTSService {
	// 创建可取消的context
	ctx, cancel := context.WithCancel(ctx)

	return &TTSService{
		ctx:                ctx,
		cancel:             cancel,
		speaker:            speaker,
		synthesizer:        synthesizer,
		errorHandler:       errorHandler,
		logger:             logger,
		enableTextSplit:    true, // 默认启用文本分割
		firstSegmentMinLen: 3,    // 第一段最小3个字符
		firstSegmentMaxLen: 5,    // 第一段最大5个字符（实现快速响应）
		minSplitLength:     6,    // 总体最小6个字符才分割
	}
}

// SetTextSplitConfig 设置文本分割配置
func (s *TTSService) SetTextSplitConfig(config TextSplitConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.enableTextSplit = config.Enable
	if config.FirstSegmentMinLen > 0 {
		s.firstSegmentMinLen = config.FirstSegmentMinLen
	}
	if config.FirstSegmentMaxLen > 0 {
		s.firstSegmentMaxLen = config.FirstSegmentMaxLen
	}
	if config.MinSplitLength > 0 {
		s.minSplitLength = config.MinSplitLength
	}

	s.logger.Info("TTS文本分割配置已更新",
		zap.Bool("enable", s.enableTextSplit),
		zap.Int("firstSegmentMinLen", s.firstSegmentMinLen),
		zap.Int("firstSegmentMaxLen", s.firstSegmentMaxLen),
		zap.Int("minSplitLength", s.minSplitLength),
	)
}

// Synthesize 合成语音
func (s *TTSService) Synthesize(ctx context.Context, text string) (<-chan []byte, error) {
	s.mu.RLock()
	closed := s.closed
	synthesizer := s.synthesizer
	enableTextSplit := s.enableTextSplit
	s.mu.RUnlock()

	if closed || synthesizer == nil {
		s.logger.Error("TTS服务不可用",
			zap.Bool("closed", closed),
			zap.Bool("synthesizerNil", synthesizer == nil),
		)
		return nil, NewRecoverableError("TTS", "服务已关闭", nil)
	}

	if text == "" {
		s.logger.Warn("TTS文本为空")
		return nil, NewRecoverableError("TTS", "文本为空", nil)
	}

	s.logger.Info("准备TTS合成",
		zap.String("text", text),
		zap.String("speaker", s.speaker),
		zap.Bool("enableTextSplit", enableTextSplit),
	)

	// 检查是否需要分割文本
	if enableTextSplit {
		return s.synthesizeWithSplit(ctx, text)
	} else {
		return s.synthesizeSingle(ctx, text)
	}
}

func (s *TTSService) synthesizeWithSplit(ctx context.Context, text string) (<-chan []byte, error) {
	// 分割文本
	segments := s.splitText(text)

	if len(segments) <= 1 {
		// 文本太短，不需要分割，使用单一合成
		s.logger.Info("文本长度不足，使用单一合成", zap.String("text", text))
		return s.synthesizeSingle(ctx, text)
	}

	s.logger.Info("文本已分割",
		zap.Int("segmentCount", len(segments)),
		zap.String("firstSegment", segments[0].Text),
		zap.String("secondSegment", segments[1].Text),
	)

	// 创建合并的音频通道
	audioChan := make(chan []byte, 400) // 增大缓冲区以容纳多个片段

	// 在goroutine中处理分割合成
	go func() {
		s.activeGoroutines.Add(1)
		defer func() {
			s.activeGoroutines.Done()
			if r := recover(); r != nil {
				s.logger.Error("TTS分割合成goroutine发生panic", zap.Any("panic", r))
			}
			close(audioChan)
		}()

		s.synthesizeSegments(ctx, segments, audioChan)
	}()

	return audioChan, nil
}

// synthesizeSegments 合成多个文本片段
func (s *TTSService) synthesizeSegments(ctx context.Context, segments []TextSegment, audioChan chan<- []byte) {
	// 按优先级顺序合成片段
	for _, segment := range segments {
		select {
		case <-ctx.Done():
			s.logger.Info("TTS分割合成被取消")
			return
		default:
		}

		// 每次循环都重新获取synthesizer，确保使用最新的
		s.mu.RLock()
		synthesizer := s.synthesizer
		closed := s.closed
		s.mu.RUnlock()

		if closed || synthesizer == nil {
			s.logger.Warn("TTS服务已关闭或合成器不可用，停止合成")
			return
		}

		s.logger.Info("开始合成片段",
			zap.Int("index", segment.Index),
			zap.String("text", segment.Text),
			zap.Bool("isLast", segment.IsLast),
		)

		// 创建片段处理器
		handler := &segmentHandler{
			audioChan:    audioChan,
			ctx:          ctx,
			logger:       s.logger,
			text:         segment.Text,
			segmentIndex: segment.Index,
			isLast:       segment.IsLast,
		}

		// 合成当前片段
		err := synthesizer.Synthesize(ctx, handler, segment.Text)
		if err != nil {
			select {
			case <-ctx.Done():
				s.logger.Info("TTS片段合成被取消", zap.Int("segmentIndex", segment.Index))
				return
			default:
				classified := s.errorHandler.Classify(err, "TTS")
				s.logger.Error("TTS片段合成失败",
					zap.Error(classified),
					zap.Int("segmentIndex", segment.Index),
					zap.String("text", segment.Text),
				)
				// 发送错误信号
				select {
				case <-ctx.Done():
				case audioChan <- nil:
				default:
				}
				return
			}
		}

		s.logger.Info("片段合成完成",
			zap.Int("segmentIndex", segment.Index),
			zap.String("text", segment.Text),
		)
	}

	s.logger.Info("所有片段合成完成", zap.Int("totalSegments", len(segments)))
}

// synthesizeSingle 单一文本合成（原有逻辑）
func (s *TTSService) synthesizeSingle(ctx context.Context, text string) (<-chan []byte, error) {
	s.mu.RLock()
	synthesizer := s.synthesizer
	s.mu.RUnlock()

	// 创建音频通道（大幅增大缓冲区，适应快速TTS合成）
	audioChan := make(chan []byte, 200)

	// 创建SynthesisHandler
	handler := &synthesisHandler{
		audioChan: audioChan,
		ctx:       ctx,
		logger:    s.logger,
		text:      text,
	}

	// 在goroutine中合成
	go func() {
		s.activeGoroutines.Add(1)
		defer func() {
			s.activeGoroutines.Done()
			// 安全关闭channel，避免panic
			if r := recover(); r != nil {
				s.logger.Error("TTS合成goroutine发生panic", zap.Any("panic", r))
			}
			// 使用select确保安全关闭channel
			select {
			case <-ctx.Done():
				// Context已取消，可能channel已被关闭，不再关闭
				s.logger.Debug("TTS合成context已取消，跳过channel关闭")
			default:
				// 正常情况下关闭channel
				close(audioChan)
			}
		}()

		s.logger.Info("开始TTS合成", zap.String("text", text))
		err := synthesizer.Synthesize(ctx, handler, text)
		if err != nil {
			// 检查是否是因为context取消导致的错误
			select {
			case <-ctx.Done():
				s.logger.Info("TTS合成被取消", zap.String("text", text))
				return
			default:
				classified := s.errorHandler.Classify(err, "TTS")
				s.logger.Error("TTS合成失败", zap.Error(classified), zap.String("text", text))
				// 发送错误信号
				select {
				case <-ctx.Done():
				case audioChan <- nil: // nil表示错误
				default:
					// channel可能已满或关闭，不阻塞
				}
			}
		} else {
			s.logger.Info("TTS合成成功完成",
				zap.String("text", text),
				zap.Int("totalChunks", handler.chunkCount),
				zap.Int("totalBytes", handler.totalBytes),
			)
		}
	}()

	return audioChan, nil
}

// synthesisHandler 实现 SynthesisHandler 接口
type synthesisHandler struct {
	audioChan  chan []byte
	ctx        context.Context
	logger     *zap.Logger
	text       string
	chunkCount int
	totalBytes int
}

func (h *synthesisHandler) OnMessage(data []byte) {
	if len(data) > 0 {
		h.chunkCount++
		h.totalBytes += len(data)

		// 每10个chunk记录一次进度
		if h.chunkCount%10 == 1 {
			h.logger.Debug("TTS音频数据接收中",
				zap.Int("chunkCount", h.chunkCount),
				zap.Int("chunkSize", len(data)),
				zap.Int("totalBytes", h.totalBytes),
			)
		}
	}

	select {
	case <-h.ctx.Done():
		// Context已取消，不再发送数据
		return
	case h.audioChan <- data:
		// 成功发送音频数据
	default:
		// 通道满了或已关闭，记录警告但不阻塞（避免TTS合成被阻塞）
		h.logger.Warn("TTS音频通道满或已关闭，丢弃数据",
			zap.Int("chunkSize", len(data)),
			zap.Int("chunkCount", h.chunkCount),
		)
	}
}

func (h *synthesisHandler) OnTimestamp(timestamp synthesizer.SentenceTimestamp) {
	// 暂时不处理时间戳
}

// segmentHandler 片段处理器
type segmentHandler struct {
	audioChan    chan<- []byte
	ctx          context.Context
	logger       *zap.Logger
	text         string
	segmentIndex int
	isLast       bool
	chunkCount   int
	totalBytes   int
}

func (h *segmentHandler) OnMessage(data []byte) {
	if len(data) > 0 {
		h.chunkCount++
		h.totalBytes += len(data)

		// 每10个chunk记录一次进度
		if h.chunkCount%10 == 1 {
			h.logger.Debug("TTS片段音频数据接收中",
				zap.Int("segmentIndex", h.segmentIndex),
				zap.Int("chunkCount", h.chunkCount),
				zap.Int("chunkSize", len(data)),
				zap.Int("totalBytes", h.totalBytes),
			)
		}
	}

	select {
	case <-h.ctx.Done():
		// Context已取消，不再发送数据
		return
	case h.audioChan <- data:
		// 成功发送音频数据
	default:
		// 通道满了或已关闭，记录警告但不阻塞
		h.logger.Warn("TTS片段音频通道满或已关闭，丢弃数据",
			zap.Int("segmentIndex", h.segmentIndex),
			zap.Int("chunkSize", len(data)),
			zap.Int("chunkCount", h.chunkCount),
		)
	}
}

func (h *segmentHandler) OnTimestamp(timestamp synthesizer.SentenceTimestamp) {
	// 暂时不处理时间戳
}

// splitText 智能分割文本（第一段最小化策略）
func (s *TTSService) splitText(text string) []TextSegment {
	s.mu.RLock()
	firstSegmentMinLen := s.firstSegmentMinLen
	firstSegmentMaxLen := s.firstSegmentMaxLen
	minSplitLength := s.minSplitLength
	s.mu.RUnlock()

	// 清理文本
	text = strings.TrimSpace(text)
	if len(text) < minSplitLength {
		// 文本太短，不分割
		return []TextSegment{
			{
				Text:     text,
				Index:    0,
				IsLast:   true,
				Priority: 0,
			},
		}
	}

	textRunes := []rune(text)
	textLength := len(textRunes)

	// 第一段最小化分割策略
	firstSegmentEnd := s.findFirstSegmentEnd(textRunes, firstSegmentMinLen, firstSegmentMaxLen)

	if firstSegmentEnd <= 0 || firstSegmentEnd >= textLength {
		// 找不到合适的分割点，不分割
		return []TextSegment{
			{
				Text:     text,
				Index:    0,
				IsLast:   true,
				Priority: 0,
			},
		}
	}

	// 分割文本
	firstPart := strings.TrimSpace(string(textRunes[:firstSegmentEnd]))
	remainingPart := strings.TrimSpace(string(textRunes[firstSegmentEnd:]))

	// 检查分割后的部分是否有效
	if len(firstPart) < 2 || len(remainingPart) < 2 {
		// 分割后的部分太短，不分割
		return []TextSegment{
			{
				Text:     text,
				Index:    0,
				IsLast:   true,
				Priority: 0,
			},
		}
	}

	s.logger.Info("TTS文本分割完成（第一段最小化）",
		zap.String("originalText", text),
		zap.String("firstPart", firstPart),
		zap.String("remainingPart", remainingPart),
		zap.Int("firstPartLen", len([]rune(firstPart))),
		zap.Int("remainingPartLen", len([]rune(remainingPart))),
	)

	// 检查剩余部分是否需要进一步分割
	segments := []TextSegment{
		{
			Text:     firstPart,
			Index:    0,
			IsLast:   false,
			Priority: 0, // 第一部分优先级最高
		},
	}

	// 对剩余部分进行进一步分割（如果需要）
	remainingSegments := s.splitRemainingText(remainingPart, 1)
	segments = append(segments, remainingSegments...)

	return segments
}

// findFirstSegmentEnd 寻找第一段的结束位置（最小化策略）
func (s *TTSService) findFirstSegmentEnd(textRunes []rune, minLen, maxLen int) int {
	textLength := len(textRunes)

	// 确保不超过文本长度
	if maxLen > textLength {
		maxLen = textLength
	}
	if minLen > textLength {
		minLen = textLength
	}

	// 第一段分割符优先级（优先使用弱分割符，实现最小化）
	firstSegmentSeparators := []struct {
		chars    []rune
		priority int
	}{
		// 最高优先级：逗号、顿号（实现最小化分割）
		{[]rune{'，', ',', '、'}, 1},
		// 中等优先级：分号、冒号
		{[]rune{'；', ';', '：', ':'}, 2},
		// 较低优先级：句号、感叹号、问号（避免过早结束）
		{[]rune{'。', '！', '？', '.', '!', '?'}, 3},
		// 最低优先级：换行符
		{[]rune{'\n'}, 4},
	}

	bestEnd := -1
	bestPriority := 999

	// 在最小长度到最大长度范围内寻找分割点
	for i := minLen; i <= maxLen && i < textLength; i++ {
		char := textRunes[i]

		for _, sep := range firstSegmentSeparators {
			for _, sepChar := range sep.chars {
				if char == sepChar {
					// 计算距离权重（越靠近最小长度越好）
					distanceFromMin := i - minLen
					adjustedPriority := sep.priority + distanceFromMin/5 // 距离权重较小

					if adjustedPriority < bestPriority {
						bestPriority = adjustedPriority
						bestEnd = i + 1 // 在分割符后分割
					}
				}
			}
		}
	}

	// 如果找到了分割符，使用它
	if bestEnd > 0 {
		return bestEnd
	}

	// 没找到分割符，寻找空格（在最小长度附近）
	for i := minLen; i <= maxLen && i < textLength; i++ {
		if unicode.IsSpace(textRunes[i]) {
			return i + 1 // 在空格后分割
		}
	}

	// 都没找到，使用最大长度作为分割点
	if maxLen < textLength {
		return maxLen
	}

	return -1 // 无法分割
}

// splitRemainingText 分割剩余文本（使用传统策略）
func (s *TTSService) splitRemainingText(text string, startIndex int) []TextSegment {
	// 对于剩余部分，使用传统的完整句子分割策略
	textRunes := []rune(text)
	textLength := len(textRunes)

	// 如果剩余文本较短，直接返回
	if textLength < 30 { // 剩余部分小于30个字符，不再分割
		return []TextSegment{
			{
				Text:     text,
				Index:    startIndex,
				IsLast:   true,
				Priority: startIndex,
			},
		}
	}

	// 寻找完整句子的分割点
	sentenceEnd := s.findSentenceEnd(textRunes, textLength/2) // 在中间位置寻找句子结束

	if sentenceEnd > 0 && sentenceEnd < textLength {
		firstPart := strings.TrimSpace(string(textRunes[:sentenceEnd]))
		secondPart := strings.TrimSpace(string(textRunes[sentenceEnd:]))

		if len(firstPart) >= 5 && len(secondPart) >= 5 {
			return []TextSegment{
				{
					Text:     firstPart,
					Index:    startIndex,
					IsLast:   false,
					Priority: startIndex,
				},
				{
					Text:     secondPart,
					Index:    startIndex + 1,
					IsLast:   true,
					Priority: startIndex + 1,
				},
			}
		}
	}

	// 无法进一步分割，返回整个剩余部分
	return []TextSegment{
		{
			Text:     text,
			Index:    startIndex,
			IsLast:   true,
			Priority: startIndex,
		},
	}
}

// findSentenceEnd 寻找句子结束位置（用于剩余文本分割）
func (s *TTSService) findSentenceEnd(textRunes []rune, preferredPos int) int {
	textLength := len(textRunes)
	searchRange := textLength / 4 // 搜索范围为文本长度的1/4

	minPos := preferredPos - searchRange
	maxPos := preferredPos + searchRange

	if minPos < 0 {
		minPos = 0
	}
	if maxPos >= textLength {
		maxPos = textLength - 1
	}

	// 句子结束符（强分割符）
	sentenceEnders := []rune{'。', '！', '？', '.', '!', '?'}

	bestPos := -1
	bestDistance := textLength

	// 寻找最接近首选位置的句子结束符
	for i := minPos; i <= maxPos; i++ {
		char := textRunes[i]
		for _, ender := range sentenceEnders {
			if char == ender {
				distance := abs(i - preferredPos)
				if distance < bestDistance {
					bestDistance = distance
					bestPos = i + 1 // 在句子结束符后分割
				}
			}
		}
	}

	return bestPos
}

// UpdateSpeaker 更新发音人和合成器
func (s *TTSService) UpdateSpeaker(speakerID string, synthesizer synthesizer.SynthesisService) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("开始更新发音人",
		zap.String("newSpeakerID", speakerID),
		zap.String("oldSpeaker", s.speaker),
	)

	// 1. 取消所有正在运行的goroutine
	if s.cancel != nil {
		s.cancel()
	}

	// 2. 等待所有goroutine完成（最多等待2秒）
	done := make(chan struct{})
	go func() {
		s.activeGoroutines.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Debug("所有TTS goroutine已停止")
	case <-time.After(2 * time.Second):
		s.logger.Warn("等待TTS goroutine停止超时，强制继续")
	}

	// 3. 安全关闭旧的合成器
	if s.synthesizer != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Warn("关闭旧合成器时发生panic，已恢复",
						zap.Any("panic", r),
						zap.String("oldSpeaker", s.speaker),
					)
				}
			}()

			if err := s.synthesizer.Close(); err != nil {
				s.logger.Warn("关闭旧合成器失败",
					zap.Error(err),
					zap.String("oldSpeaker", s.speaker),
				)
			}
		}()
	}

	// 4. 创建新的context和cancel函数
	s.ctx, s.cancel = context.WithCancel(context.Background())

	// 5. 更新发音人和合成器
	s.speaker = speakerID
	s.synthesizer = synthesizer

	s.logger.Info("TTS发音人已更新",
		zap.String("speakerID", speakerID),
		zap.Bool("enableTextSplit", s.enableTextSplit),
		zap.Int("firstSegmentMinLen", s.firstSegmentMinLen),
		zap.Int("firstSegmentMaxLen", s.firstSegmentMaxLen),
		zap.Int("minSplitLength", s.minSplitLength),
	)
}

// StreamingTTSCallback 流式TTS回调函数
type StreamingTTSCallback func(audioData []byte, isFirst bool, isLast bool, segmentIndex int) error

// SynthesizeStream 流式合成语音（接收LLM流式输出）
func (s *TTSService) SynthesizeStream(ctx context.Context, textChan <-chan string, callback StreamingTTSCallback) error {
	s.mu.RLock()
	closed := s.closed
	synthesizer := s.synthesizer
	enableTextSplit := s.enableTextSplit
	s.mu.RUnlock()

	if closed || synthesizer == nil {
		s.logger.Error("TTS服务不可用")
		return NewRecoverableError("TTS", "服务已关闭", nil)
	}

	s.logger.Info("开始流式TTS合成")

	var textBuffer strings.Builder
	var segmentIndex int
	var firstSegmentSent bool

	for {
		select {
		case <-ctx.Done():
			s.logger.Info("流式TTS合成被取消")
			return ctx.Err()

		case text, ok := <-textChan:
			if !ok {
				// 通道关闭，处理剩余文本
				remaining := textBuffer.String()
				s.logger.Info("LLM流式输出完成，处理剩余文本",
					zap.String("remaining", remaining),
					zap.Int("remainingLen", len([]rune(remaining))),
					zap.Int("segmentIndex", segmentIndex),
					zap.Bool("firstSegmentSent", firstSegmentSent),
				)

				if remaining != "" {
					// 尝试分割剩余文本
					if firstSegmentSent {
						// 已经发送过第一段，尝试提取完整句子
						if segment := s.tryExtractCompleteSentence(remaining); segment != "" {
							s.logger.Info("从剩余文本提取句子", zap.String("segment", segment))
							err := s.synthesizeTextSegment(ctx, segment, segmentIndex, false, false, callback)
							if err != nil {
								s.logger.Error("合成剩余句子失败", zap.Error(err))
							}
							segmentIndex++

							// 更新剩余文本
							remaining = strings.TrimSpace(remaining[len(segment):])
						}
					}

					// 处理最后的剩余文本
					if remaining != "" {
						// 检查剩余文本是否有意义（不只是标点符号）
						remainingRunes := []rune(strings.TrimSpace(remaining))
						if len(remainingRunes) >= 2 { // 至少2个字符才值得合成
							s.logger.Info("处理最后剩余文本", zap.String("text", remaining))
							err := s.synthesizeTextSegment(ctx, remaining, segmentIndex, true, !firstSegmentSent, callback)
							if err != nil {
								s.logger.Error("合成最后剩余文本失败", zap.Error(err))
							}
						}
					}
				}
				s.logger.Info("流式TTS合成完成")
				return nil
			}

			if text == "" {
				continue
			}

			// 将新文本添加到缓冲区
			textBuffer.WriteString(text)
			currentText := textBuffer.String()

			s.logger.Debug("收到流式文本",
				zap.String("newText", text),
				zap.String("currentBuffer", currentText),
				zap.Int("bufferLen", len([]rune(currentText))),
			)

			// 立即检查是否可以提取第一段进行合成（更激进的策略）
			if !firstSegmentSent && enableTextSplit {
				if segment := s.tryExtractFirstSegmentAggressive(currentText); segment != "" {
					s.logger.Info("立即提取第一段进行合成",
						zap.String("segment", segment),
						zap.Int("segmentLen", len([]rune(segment))),
					)

					// 立即合成第一段
					err := s.synthesizeTextSegment(ctx, segment, segmentIndex, false, true, callback)
					if err != nil {
						s.logger.Error("合成第一段失败", zap.Error(err))
					}

					// 从缓冲区移除已处理的文本
					remaining := strings.TrimSpace(currentText[len(segment):])
					textBuffer.Reset()
					textBuffer.WriteString(remaining)

					segmentIndex++
					firstSegmentSent = true
				}
			}

			// 检查是否有完整的句子可以合成（第一段之后）
			if firstSegmentSent {
				currentBuffer := textBuffer.String()
				s.logger.Debug("检查第二段分割",
					zap.String("currentBuffer", currentBuffer),
					zap.Int("bufferLen", len([]rune(currentBuffer))),
				)

				if segment := s.tryExtractCompleteSentence(currentBuffer); segment != "" {
					s.logger.Info("提取完整句子进行合成",
						zap.String("segment", segment),
						zap.Int("segmentLen", len([]rune(segment))),
					)

					// 合成句子
					err := s.synthesizeTextSegment(ctx, segment, segmentIndex, false, false, callback)
					if err != nil {
						s.logger.Error("合成句子失败", zap.Error(err))
					}

					// 从缓冲区移除已处理的文本
					remaining := strings.TrimSpace(currentBuffer[len(segment):])
					textBuffer.Reset()
					textBuffer.WriteString(remaining)

					segmentIndex++
				} else {
					s.logger.Debug("未找到可分割的第二段",
						zap.String("buffer", currentBuffer),
						zap.Int("bufferLen", len([]rune(currentBuffer))),
					)
				}
			}
		}
	}
}

// tryExtractFirstSegmentAggressive 更激进的第一段提取策略（立即响应）
func (s *TTSService) tryExtractFirstSegmentAggressive(text string) string {
	s.mu.RLock()
	firstSegmentMinLen := s.firstSegmentMinLen
	firstSegmentMaxLen := s.firstSegmentMaxLen
	s.mu.RUnlock()

	textRunes := []rune(strings.TrimSpace(text))
	textLength := len(textRunes)

	// 超激进：只要达到最小长度就开始寻找分割点
	if textLength < firstSegmentMinLen {
		return "" // 文本太短，等待更多内容
	}

	// 第一段分割符（优先使用弱分割符，更容易触发）
	weakSeparators := []rune{'～', '~', '，', ',', '、', '；', ';', '：', ':'}
	strongSeparators := []rune{'。', '！', '？', '.', '!', '?', '\n'}

	// 超激进策略1：在最小长度处立即寻找弱分割符
	for i := firstSegmentMinLen; i <= min(textLength, firstSegmentMaxLen); i++ {
		if i >= textLength {
			break
		}

		char := textRunes[i-1] // 检查当前位置的字符
		// 优先检查弱分割符
		for _, sep := range weakSeparators {
			if char == sep {
				segment := strings.TrimSpace(string(textRunes[:i]))
				if len([]rune(segment)) >= firstSegmentMinLen {
					return segment
				}
			}
		}
	}

	// 超激进策略2：检查强分割符
	for i := firstSegmentMinLen; i <= min(textLength, firstSegmentMaxLen); i++ {
		if i >= textLength {
			break
		}

		char := textRunes[i-1]
		for _, sep := range strongSeparators {
			if char == sep {
				segment := strings.TrimSpace(string(textRunes[:i]))
				if len([]rune(segment)) >= firstSegmentMinLen {
					return segment
				}
			}
		}
	}

	// 超激进策略3：词汇边界立即分割（最关键的优化）
	if textLength >= firstSegmentMinLen {
		// 寻找词汇边界（中文常见的词汇分割点）
		wordBoundaries := []string{"哦", "啊", "呢", "吧", "的", "了", "在", "是", "有", "我", "你", "他", "她", "它"}

		for i := firstSegmentMinLen; i <= min(textLength, firstSegmentMinLen+2); i++ {
			for _, boundary := range wordBoundaries {
				if i >= len(boundary) && strings.HasSuffix(string(textRunes[:i]), boundary) {
					segment := strings.TrimSpace(string(textRunes[:i]))
					if len([]rune(segment)) >= firstSegmentMinLen {
						s.logger.Debug("通过词汇边界提取第一段",
							zap.String("boundary", boundary),
							zap.String("segment", segment),
						)
						return segment
					}
				}
			}
		}
	}

	// 超激进策略4：如果文本已经比较长，在空格处分割
	if textLength >= firstSegmentMaxLen {
		for i := firstSegmentMaxLen; i >= firstSegmentMinLen; i-- {
			if i < textLength && unicode.IsSpace(textRunes[i]) {
				return strings.TrimSpace(string(textRunes[:i]))
			}
		}
		// 最激进：强制在最大长度处分割
		return strings.TrimSpace(string(textRunes[:firstSegmentMaxLen]))
	}

	// 超激进策略5：如果文本长度达到5个字符，立即分割（最激进的策略）
	if textLength >= 5 {
		segment := strings.TrimSpace(string(textRunes[:5]))
		s.logger.Debug("超激进5字符分割",
			zap.String("segment", segment),
			zap.Int("textLength", textLength),
		)
		return segment
	}

	return "" // 等待更多内容
}

// tryExtractCompleteSentence 尝试提取完整句子
func (s *TTSService) tryExtractCompleteSentence(text string) string {
	textRunes := []rune(strings.TrimSpace(text))
	textLength := len(textRunes)

	if textLength < 3 { // 最小长度要求
		return "" // 文本太短
	}

	// 句子结束符（扩展列表，包含中文常用符号）
	sentenceEnders := []rune{'。', '！', '？', '.', '!', '?', '～', '~', '，', ',', '；', ';', '：', ':'}

	// 寻找句子结束符（从第3个字符开始）
	for i := 3; i < textLength; i++ {
		char := textRunes[i]
		for _, ender := range sentenceEnders {
			if char == ender {
				sentence := strings.TrimSpace(string(textRunes[:i+1]))
				if len([]rune(sentence)) >= 3 {
					return sentence
				}
			}
		}
	}

	// 如果文本较长但没有结束符，按长度分割
	if textLength > 15 {
		// 寻找逗号或分号进行分割
		for i := 8; i < textLength && i < 15; i++ {
			char := textRunes[i]
			if char == '，' || char == ',' || char == '；' || char == ';' || char == '：' || char == ':' {
				return strings.TrimSpace(string(textRunes[:i+1]))
			}
		}
		// 强制分割
		return strings.TrimSpace(string(textRunes[:15]))
	}

	// 关键改进：如果没有找到分割点，但文本不为空且长度合理，直接返回整个文本
	if textLength >= 3 {
		s.logger.Debug("没有找到分割点，直接合成整个文本",
			zap.String("text", text),
			zap.Int("textLen", textLength),
		)
		return strings.TrimSpace(text)
	}

	return "" // 文本太短，等待更多内容
}

// synthesizeTextSegment 合成文本片段
func (s *TTSService) synthesizeTextSegment(ctx context.Context, text string, segmentIndex int, isLast bool, isFirst bool, callback StreamingTTSCallback) error {
	if text == "" {
		return nil
	}

	s.logger.Info("开始合成文本片段",
		zap.String("text", text),
		zap.Int("segmentIndex", segmentIndex),
		zap.Bool("isFirst", isFirst),
		zap.Bool("isLast", isLast),
	)

	// 创建片段处理器
	handler := &streamingSegmentHandler{
		callback:     callback,
		ctx:          ctx,
		logger:       s.logger,
		text:         text,
		segmentIndex: segmentIndex,
		isFirst:      isFirst,
		isLast:       isLast,
	}

	// 合成当前片段
	err := s.synthesizer.Synthesize(ctx, handler, text)
	if err != nil {
		classified := s.errorHandler.Classify(err, "TTS")
		s.logger.Error("TTS片段合成失败",
			zap.Error(classified),
			zap.Int("segmentIndex", segmentIndex),
			zap.String("text", text),
		)
		return classified
	}

	s.logger.Info("文本片段合成完成",
		zap.Int("segmentIndex", segmentIndex),
		zap.String("text", text),
	)

	return nil
}

// streamingSegmentHandler 流式片段处理器
type streamingSegmentHandler struct {
	callback     StreamingTTSCallback
	ctx          context.Context
	logger       *zap.Logger
	text         string
	segmentIndex int
	isFirst      bool
	isLast       bool
	chunkCount   int
	totalBytes   int
}

func (h *streamingSegmentHandler) OnMessage(data []byte) {
	if len(data) > 0 {
		h.chunkCount++
		h.totalBytes += len(data)

		// 每10个chunk记录一次进度
		if h.chunkCount%10 == 1 {
			h.logger.Debug("流式TTS音频数据接收中",
				zap.Int("segmentIndex", h.segmentIndex),
				zap.Int("chunkCount", h.chunkCount),
				zap.Int("chunkSize", len(data)),
				zap.Int("totalBytes", h.totalBytes),
				zap.Bool("isFirst", h.isFirst),
			)
		}
	}

	select {
	case <-h.ctx.Done():
		// Context已取消，不再发送数据
		return
	default:
		// 调用回调函数发送音频数据
		if err := h.callback(data, h.isFirst && h.chunkCount == 1, h.isLast && len(data) == 0, h.segmentIndex); err != nil {
			h.logger.Warn("流式TTS回调失败",
				zap.Error(err),
				zap.Int("segmentIndex", h.segmentIndex),
				zap.Int("chunkSize", len(data)),
			)
		}
	}
}

func (h *streamingSegmentHandler) OnTimestamp(timestamp synthesizer.SentenceTimestamp) {
	// 暂时不处理时间戳
}

// Close 关闭服务
func (s *TTSService) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	if s.synthesizer != nil {
		s.synthesizer.Close()
	}

	s.closed = true
	return nil
}
