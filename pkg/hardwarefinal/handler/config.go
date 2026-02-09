package handler

import (
	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gorilla/websocket"
)

const (
	DefaultTemperature          = 0.7
	DefaultLLMModel             = "deepseek-v3.1" // 默认LLM模型
	DefaultVADThreshold         = 500.0
	DefaultEnabledVAD           = true
	DefaultVADConsecutiveFrames = 2
	DefaultMaxLLMToken          = 100
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

func (ho *HardwareOptions) loadConfigs() *HardwareOptions {
	if ho.LLMModel == "" {
		ho.LLMModel = DefaultLLMModel
	}
	if ho.Temperature <= 0 {
		ho.Temperature = DefaultTemperature
	}
	if ho.EnableVAD == false {
		ho.EnableVAD = DefaultEnabledVAD
	}
	if ho.VADThreshold <= 0 {
		ho.VADThreshold = DefaultVADThreshold
	}
	if ho.VADConsecutiveFrames <= 0 {
		ho.VADConsecutiveFrames = DefaultVADConsecutiveFrames
	}
	if ho.MaxLLMToken <= 0 {
		ho.MaxLLMToken = DefaultMaxLLMToken
	}
	return ho
}
