package handler

import (
	"context"

	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/protocol"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// HardwareHandler hardware handler
type HardwareHandler struct {
	logger *zap.Logger
	db     *gorm.DB
}

// NewHardwareHandler create hardware handler
func NewHardwareHandler(db *gorm.DB, logger *zap.Logger) *HardwareHandler {
	if logger == nil {
		logger = zap.L()
	}
	return &HardwareHandler{
		logger: logger,
		db:     db,
	}
}

func (h *HardwareHandler) HandlerHardwareWebsocket(
	ctx context.Context,
	options *HardwareOptions) {
	if options == nil || options.AssistantID == 0 {
		h.logger.Error("[Handler] options is nil or assistantID is 0")
		return
	}

	options.loadConfigs()
	defer options.Conn.Close()

	h.logger.Info("[Handler] 创建 HardwareSession",
		zap.Uint("assistantID", options.AssistantID),
		zap.String("language", options.Language))

	session := protocol.NewHardwareSession(ctx, &protocol.HardwareSessionOption{
		Conn:                 options.Conn,
		Logger:               h.logger,
		AssistantID:          options.AssistantID,
		LLMModel:             options.LLMModel,
		Credential:           options.Credential,
		SystemPrompt:         options.SystemPrompt,
		MaxToken:             options.MaxLLMToken,
		Speaker:              options.Speaker,
		EnableVAD:            options.EnableVAD,
		VADThreshold:         options.VADThreshold,
		VADConsecutiveFrames: options.VADConsecutiveFrames,
	})
	if err := session.Start(); err != nil {
		h.logger.Error("[Handler] 启动会话失败", zap.Error(err))
		return
	}
	<-ctx.Done()
	if err := session.Stop(); err != nil {
		h.logger.Error("[Handler] 停止会话失败", zap.Error(err))
	}
}
