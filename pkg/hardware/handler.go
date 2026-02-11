package hardware

import (
	"context"
	"fmt"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/cache"
	"github.com/code-100-precent/LingEcho/pkg/hardware/constants"
	"github.com/code-100-precent/LingEcho/pkg/hardware/protocol"
	"github.com/code-100-precent/LingEcho/pkg/voiceprint"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type HardwareOptions struct {
	Conn                 *websocket.Conn
	AssistantID          uint
	Credential           *models.UserCredential
	Language             string
	Speaker              string
	Temperature          float64
	SystemPrompt         string
	KnowledgeKey         string
	UserID               uint
	DeviceID             *string
	MacAddress           string
	LLMModel             string
	MaxLLMToken          int
	EnableVAD            bool
	VADThreshold         float64
	VADConsecutiveFrames int
}

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

// HandlerHardwareWebsocket handler hardware websocket
func (h *HardwareHandler) HandlerHardwareWebsocket(
	ctx context.Context,
	options *HardwareOptions) {
	if options == nil || options.AssistantID == 0 {
		h.logger.Error("[Handler] --- options is nil or assistantID is 0")
		return
	}
	options.loadConfigs()
	defer options.Conn.Close()
	h.logger.Info(fmt.Sprintf("[Handler] --- 创建 HardwareSession assistantID: %d", options.AssistantID))

	// 初始化声纹识别服务
	voiceprintConfig := voiceprint.DefaultConfig()
	voiceprintService, err := voiceprint.NewService(voiceprintConfig, cache.GetGlobalCache())
	if err != nil {
		h.logger.Warn("[Handler] --- 初始化声纹识别服务失败", zap.Error(err))
		voiceprintService = nil
	}

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
		DB:                   h.db,
		VoiceprintService:    voiceprintService,
	})
	if err := session.Start(); err != nil {
		h.logger.Error("[Handler] --- 启动会话失败", zap.Error(err))
		return
	}
	<-ctx.Done()
	if err := session.Stop(); err != nil {
		h.logger.Error("[Handler] --- 停止会话失败", zap.Error(err))
	}
}

func (ho *HardwareOptions) loadConfigs() *HardwareOptions {
	if ho.LLMModel == "" {
		ho.LLMModel = constants.DefaultLLMModel
	}
	if ho.Temperature <= 0 {
		ho.Temperature = constants.DefaultTemperature
	}
	if ho.EnableVAD == false {
		ho.EnableVAD = constants.DefaultEnabledVAD
	}
	if ho.VADThreshold <= 0 {
		ho.VADThreshold = constants.DefaultVADThreshold
	}
	if ho.VADConsecutiveFrames <= 0 {
		ho.VADConsecutiveFrames = constants.DefaultVADConsecutiveFrames
	}
	if ho.MaxLLMToken <= 0 {
		ho.MaxLLMToken = constants.DefaultMaxLLMToken
	}
	return ho
}
