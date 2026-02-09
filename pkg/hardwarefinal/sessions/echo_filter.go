package sessions

import (
	"context"
	"fmt"

	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/constants"
	"go.uber.org/zap"
)

// EchoFilterComponent 回音消除阶段
type EchoFilterComponent struct {
	logger *zap.Logger
}

func (s *EchoFilterComponent) Name() string {
	return constants.COMPONENT_ECHO_FILTER
}

func (s *EchoFilterComponent) Process(ctx context.Context, data interface{}) (interface{}, bool, error) {
	pcmData, ok := data.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("invalid data type: %T", data)
	}

	// TODO: 实现回音消除
	// filtered := s.filter.Filter(pcmData)

	s.logger.Debug("[EchoFilter] 回音消除完成", zap.Int("size", len(pcmData)))
	return pcmData, true, nil
}
