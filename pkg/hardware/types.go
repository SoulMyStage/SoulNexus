package hardware

import (
	"context"
	"fmt"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/media"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// SessionConfig 会话配置
type SessionConfig struct {
	Conn         *websocket.Conn
	Credential   *models.UserCredential
	AssistantID  uint // 改为uint类型
	Language     string
	Speaker      string
	Temperature  float64
	MaxTokens    int
	SystemPrompt string
	KnowledgeKey string
	LLMModel     string
	DB           *gorm.DB
	Logger       *zap.Logger
	Context      context.Context

	// 录音相关配置
	UserID        uint    // 用户ID
	DeviceID      *string // 设备ID（MAC地址）
	MacAddress    string  // MAC地址
	RecordingPath string  // 录音文件存储路径

	// VAD 配置
	EnableVAD            bool    // 是否启用VAD
	VADThreshold         float64 // VAD阈值
	VADConsecutiveFrames int     // 需要连续超过阈值的帧数
}

// SessionInterface 语音会话接口
type SessionInterface interface {
	// Start 启动会话
	Start() error

	// Stop 停止会话
	Stop() error

	// HandleAudio 处理音频数据
	HandleAudio(data []byte) error

	// HandleText 处理文本消息
	HandleText(data []byte) error

	// IsActive 检查会话是否活跃
	IsActive() bool
}

// MessageWriter 消息写入器接口
type MessageWriter interface {
	// SendASRResult 发送ASR识别结果
	SendASRResult(text string) error

	// SendTTSAudio 发送TTS音频数据
	SendTTSAudio(data []byte) error

	// SendError 发送错误消息
	SendError(message string, fatal bool) error

	// SendConnected 发送连接成功消息
	SendConnected() error

	// SendLLMResponse 发送LLM响应
	SendLLMResponse(text string) error

	// SendTTSStart 发送TTS开始消息
	SendTTSStart(format media.StreamFormat) error

	// SendTTSEnd 发送TTS结束消息
	SendTTSEnd() error

	// SendWelcome 发送Welcome消息（xiaozhi协议）
	SendWelcome(audioFormat string, sampleRate, channels int, features map[string]interface{}) (string, error)

	// Close 关闭写入器
	Close() error
}

// ErrorHandler 错误处理器接口
type ErrorHandler interface {
	// HandleError 处理错误
	HandleError(err error, service string) error

	// IsFatal 判断是否是致命错误
	IsFatal(err error) bool
}

// AudioManager 音频管理器接口（解决TTS冲突）
type AudioManager interface {
	// ProcessInputAudio 处理输入音频（智能过滤TTS回音）
	ProcessInputAudio(data []byte, ttsPlaying bool) ([]byte, bool)

	// RecordTTSOutput 记录TTS输出音频（用于回声消除）
	RecordTTSOutput(data []byte)

	// Clear 清空状态
	Clear()
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
