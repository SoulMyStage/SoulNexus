package sessions

import (
	"context"
	"fmt"

	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/constants"
	"go.uber.org/zap"
)

// FilterWordComponent filter check stage
type FilterWordComponent struct {
	logger      *zap.Logger
	filterWords map[string]bool
}

func (s *FilterWordComponent) Name() string {
	return constants.COMPONENT_FILTER_WORD
}

func (s *FilterWordComponent) Process(ctx context.Context, data interface{}) (interface{}, bool, error) {
	text, ok := data.(string)
	if !ok {
		return nil, false, fmt.Errorf("invalid data type")
	}

	// TODO: 实现过滤词检测
	// isFiltered := s.checkFilterWords(text)
	isFiltered := false

	if isFiltered {
		s.logger.Info("[FilterWord] 文本被过滤", zap.String("text", text))
		return nil, false, nil // 中断管道
	}

	s.logger.Debug("[FilterWord] 文本通过", zap.String("text", text))
	return text, true, nil
}
