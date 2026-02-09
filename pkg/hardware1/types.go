package hardware

import (
	"context"
	"fmt"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SessionConfig session config
type SessionConfig struct {
	Conn                 *websocket.Conn
	LLMProvider          string
	LLMApiKey            string
	LLMApiURL            string
	AsrConfig            models.ProviderConfig
	TtsConfig            models.ProviderConfig
	AssistantID          uint
	Language             string
	Speaker              string
	Temperature          float64
	MaxTokens            int
	SystemPrompt         string
	KnowledgeKey         string
	LLMModel             string
	DB                   *gorm.DB
	Logger               *zap.Logger
	Context              context.Context
	UserID               uint    // 用户ID
	DeviceID             *string // 设备ID（MAC地址）
	MacAddress           string  // MAC地址
	EnableVAD            bool    // 是否启用VAD
	VADThreshold         float64 // VAD阈值
	VADConsecutiveFrames int     // 需要连续超过阈值的帧数
}

// ErrorType error type enumeration
type ErrorType int

const (
	// ErrorTypeFatal fatal error (requires disconnection)
	ErrorTypeFatal ErrorType = iota
	// ErrorTypeRecoverable recoverable error (can retry)
	ErrorTypeRecoverable
	// ErrorTypeTransient transient error (temporary failure, will auto-recover)
	ErrorTypeTransient
)

// String returns string representation of ErrorType
func (et ErrorType) String() string {
	switch et {
	case ErrorTypeFatal:
		return "fatal"
	case ErrorTypeRecoverable:
		return "recoverable"
	case ErrorTypeTransient:
		return "transient"
	default:
		return "unknown"
	}
}

// Error unified error structure
type Error struct {
	Type    ErrorType
	Service string
	Message string
	Err     error
}

func (e *Error) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Service, e.Message, e.Err)
	}
	return fmt.Sprintf("[%s] %s", e.Service, e.Message)
}

// Unwrap returns the underlying error
func (e *Error) Unwrap() error {
	return e.Err
}
