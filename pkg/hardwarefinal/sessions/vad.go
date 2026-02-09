package sessions

import (
	"context"
	"fmt"

	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/constants"
	"go.uber.org/zap"
)

// VADComponent VAD component
type VADComponent struct {
	logger *zap.Logger
}

func (s *VADComponent) Name() string {
	return constants.COMPONENT_VAD
}

func (s *VADComponent) Process(ctx context.Context, data interface{}) (interface{}, bool, error) {
	pcmData, ok := data.([]byte)
	if !ok {
		return nil, false, fmt.Errorf("invalid data type")
	}

	// TODO: 实现 VAD 检测
	// hasVoice := s.detector.HasVoice(pcmData)
	hasVoice := true // 暂时返回 true，让所有音频通过

	if !hasVoice {
		s.logger.Debug("[VAD] 未检测到语音")
		return nil, false, nil // 中断管道
	}

	s.logger.Debug("[VAD] 检测到语音", zap.Int("size", len(pcmData)))
	return pcmData, true, nil
}
