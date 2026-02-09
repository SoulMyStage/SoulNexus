package stream

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TextSegment 文本片段
type TextSegment struct {
	Text      string
	IsFinal   bool // 是否是最后一段
	Timestamp time.Time
	PlayID    string
}

// TextSegmenter text segmenter
type TextSegmenter struct {
	outputCh      chan<- TextSegment
	buffer        string
	lastUpdate    time.Time
	delayTimer    *time.Timer
	delayTimeout  time.Duration
	minChars      int
	maxChars      int
	currentPlayID string
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	logger        *zap.Logger
}

// NewTextSegmenter create text segmenter
func NewTextSegmenter(outputCh chan<- TextSegment, logger *zap.Logger) *TextSegmenter {
	return &TextSegmenter{
		outputCh:     outputCh,
		delayTimeout: 100 * time.Millisecond,
		minChars:     8,
		maxChars:     40,
		logger:       logger,
	}
}

// Start 启动分段器
func (s *TextSegmenter) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.logger.Info("TextSegmenter started")
	return nil
}

// Stop 停止分段器
func (s *TextSegmenter) Stop() error {
	s.cancel()
	if s.delayTimer != nil {
		s.delayTimer.Stop()
	}
	s.logger.Info("TextSegmenter stopped")
	return nil
}

// OnToken 处理 LLM 输出的 token
func (s *TextSegmenter) OnToken(token string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.buffer += token
	s.lastUpdate = time.Now()
	if strings.HasSuffix(s.buffer, "。") ||
		strings.HasSuffix(s.buffer, "！") ||
		strings.HasSuffix(s.buffer, "？") {
		s.flush(false)
		return
	}
	if strings.HasSuffix(s.buffer, "，") ||
		strings.HasSuffix(s.buffer, "、") {
		if s.delayTimer != nil {
			s.delayTimer.Stop()
		}
		s.delayTimer = time.AfterFunc(s.delayTimeout, func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if time.Since(s.lastUpdate) >= s.delayTimeout {
				if len([]rune(s.buffer)) >= s.minChars {
					s.flush(false)
				}
			}
		})
		return
	}
	if len([]rune(s.buffer)) > s.maxChars {
		s.flush(false)
	}
}

// OnComplete LLM 完成输出
func (s *TextSegmenter) OnComplete() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.delayTimer != nil {
		s.delayTimer.Stop()
	}

	// 立即刷新最后一段
	s.flush(true)
}

// SetPlayID 设置当前 PlayID
func (s *TextSegmenter) SetPlayID(playID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentPlayID = playID
}

// flush 刷新缓冲区（需要持有锁）
func (s *TextSegmenter) flush(isFinal bool) {
	if s.buffer == "" {
		return
	}

	segment := TextSegment{
		Text:      s.buffer,
		IsFinal:   isFinal,
		Timestamp: time.Now(),
		PlayID:    s.currentPlayID,
	}

	s.logger.Debug("Flushing segment",
		zap.String("text", s.buffer),
		zap.Bool("isFinal", isFinal),
		zap.Int("length", len([]rune(s.buffer))))
	select {
	case s.outputCh <- segment:
		s.buffer = ""
	case <-s.ctx.Done():
		return
	}
}

// Reset 重置分段器状态
func (s *TextSegmenter) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.delayTimer != nil {
		s.delayTimer.Stop()
		s.delayTimer = nil
	}
	s.buffer = ""
	s.lastUpdate = time.Time{}
}
