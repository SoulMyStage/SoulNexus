package hardware

import (
	"context"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// HardwareHandler hardware connection handler
type HardwareHandler struct {
	logger *zap.Logger
}

// NewHandler create new handler
func NewHandler(logger *zap.Logger) *HardwareHandler {
	if logger == nil {
		logger = zap.L()
	}
	return &HardwareHandler{
		logger: logger,
	}
}

// HandleWebSocket handler websocket connection
func (h *HardwareHandler) HandleWebSocket(
	ctx context.Context,
	conn *websocket.Conn,
	credential *models.UserCredential,
	assistantID int,
	language, speaker string,
	temperature float64,
	systemPrompt string,
	knowledgeKey string,
	db *gorm.DB,
	userID uint,
	deviceID *string,
	macAddress string,
) {
	defer conn.Close()
	llmModel := DefaultLLMModel
	assistantTemperature := 0.6
	assistantMaxTokens := 70
	enableVAD := true
	vadThreshold := 500.0
	vadConsecutiveFrames := 2
	if assistantID > 0 && db != nil {
		var assistant models.Assistant
		if err := db.First(&assistant, assistantID).Error; err == nil {
			if assistant.LLMModel != "" {
				llmModel = assistant.LLMModel
			}
			if assistant.Temperature > 0 {
				assistantTemperature = float64(assistant.Temperature)
			}
			if assistant.MaxTokens > 0 {
				assistantMaxTokens = assistant.MaxTokens
			}
			enableVAD = assistant.EnableVAD
			if assistant.VADThreshold > 0 {
				vadThreshold = assistant.VADThreshold
			}
			if assistant.VADConsecutiveFrames > 0 {
				vadConsecutiveFrames = assistant.VADConsecutiveFrames
			}
		}
	}
	if temperature <= 0 {
		temperature = assistantTemperature
	}
	session, err := NewSession(&SessionConfig{
		DB:                   db,
		Conn:                 conn,
		LLMProvider:          credential.LLMProvider,
		LLMApiKey:            credential.LLMApiKey,
		LLMApiURL:            credential.LLMApiURL,
		AsrConfig:            credential.AsrConfig,
		TtsConfig:            credential.TtsConfig,
		AssistantID:          uint(assistantID),
		Language:             language,
		Speaker:              speaker,
		Temperature:          temperature,
		MaxTokens:            assistantMaxTokens,
		SystemPrompt:         systemPrompt,
		KnowledgeKey:         knowledgeKey,
		LLMModel:             llmModel,
		Logger:               h.logger,
		Context:              ctx,
		EnableVAD:            enableVAD,
		VADThreshold:         vadThreshold,
		VADConsecutiveFrames: vadConsecutiveFrames,
		UserID:               userID,
		DeviceID:             deviceID,
		MacAddress:           macAddress,
	})
	if err != nil {
		h.logger.Error("create session error", zap.Error(err))
		return
	}
	if err := session.Start(); err != nil {
		h.logger.Error("start session error", zap.Error(err))
		return
	}
	<-ctx.Done()
	if err := session.Stop(); err != nil {
		h.logger.Error("stop session failed", zap.Error(err))
	}
}
