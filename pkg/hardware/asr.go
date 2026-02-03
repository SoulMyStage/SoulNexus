package hardware

import (
	"context"
	"sync"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/hardware/reconnect"
	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"go.uber.org/zap"
)

// ASRService ASR服务实现
type ASRService struct {
	ctx          context.Context
	cancel       context.CancelFunc
	credential   *models.UserCredential
	language     string
	transcriber  recognizer.TranscribeService
	errorHandler *ErrHandler
	reconnectMgr *reconnect.Manager
	logger       *zap.Logger
	mu           sync.RWMutex
	connected    bool
	onResult     func(text string, isLast bool, duration time.Duration, uuid string)
	onError      func(err error)
}

// NewASRService 创建ASR服务
func NewASRService(
	ctx context.Context,
	credential *models.UserCredential,
	language string,
	transcriber recognizer.TranscribeService,
	errorHandler *ErrHandler,
	logger *zap.Logger,
) *ASRService {
	ctx, cancel := context.WithCancel(ctx)

	service := &ASRService{
		ctx:          ctx,
		cancel:       cancel,
		credential:   credential,
		language:     language,
		transcriber:  transcriber,
		errorHandler: errorHandler,
		logger:       logger,
	}
	// 创建重连管理器
	strategy := reconnect.NewExponentialBackoffStrategy()
	reconnectMgr := reconnect.NewManager(ctx, logger, strategy)
	reconnectMgr.SetReconnectCallback(service.reconnect)
	reconnectMgr.SetDisconnectCallback(service.onDisconnect)
	service.reconnectMgr = reconnectMgr

	return service
}

// SetCallbacks 设置回调函数
func (s *ASRService) SetCallbacks(
	onResult func(text string, isLast bool, duration time.Duration, uuid string),
	onError func(err error),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onResult = onResult
	s.onError = onError
}

// Connect 建立连接
func (s *ASRService) Connect() error {
	s.mu.Lock()
	if s.connected {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// 初始化ASR服务
	s.transcriber.Init(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			if s.onResult != nil {
				s.onResult(text, isLast, duration, uuid)
			}
		},
		func(err error, isFatal bool) {
			if s.onError != nil {
				s.onError(err)
			}
			if err != nil {
				classified := s.errorHandler.Classify(err, "ASR")
				if classified.Type == ErrorTypeFatal {
					s.mu.Lock()
					s.connected = false
					s.mu.Unlock()
				} else if classified.Type == ErrorTypeTransient {
					s.reconnectMgr.NotifyDisconnect(err)
				}
			}
		},
	)

	// 启动连接和接收循环
	go s.receiveLoop()

	return nil
}

// Disconnect 断开连接
func (s *ASRService) Disconnect() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.connected {
		return nil
	}

	s.cancel()
	if s.transcriber != nil {
		s.transcriber.StopConn()
	}

	s.connected = false
	return nil
}

// SendAudio 发送音频数据
func (s *ASRService) SendAudio(data []byte) error {
	s.mu.RLock()
	connected := s.connected
	transcriber := s.transcriber
	s.mu.RUnlock()

	if !connected || transcriber == nil {
		return NewTransientError("ASR", "服务未连接", nil)
	}

	if err := transcriber.SendAudioBytes(data); err != nil {
		if !transcriber.Activity() {
			s.mu.Lock()
			s.connected = false
			s.mu.Unlock()
			s.reconnectMgr.NotifyDisconnect(err)
		}
		return NewTransientError("ASR", "发送音频失败", err)
	}

	return nil
}

// IsConnected 检查是否已连接
func (s *ASRService) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connected
}

// Activity 检查服务是否活跃
func (s *ASRService) Activity() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.connected || s.transcriber == nil {
		return false
	}
	return s.transcriber.Activity()
}

// receiveLoop 接收循环
func (s *ASRService) receiveLoop() {
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("ASR接收循环退出")
			s.mu.Lock()
			s.connected = false
			s.mu.Unlock()
			return
		default:
		}

		// 尝试连接
		err := s.transcriber.ConnAndReceive("")
		if err != nil {
			s.mu.Lock()
			s.connected = false
			s.mu.Unlock()

			classified := s.errorHandler.Classify(err, "ASR")

			if classified.Type == ErrorTypeFatal {
				s.logger.Error("ASR连接致命错误", zap.Error(err))
				if s.onError != nil {
					s.onError(classified)
				}
				return
			}

			// 其他错误，通知重连管理器
			s.logger.Warn("ASR连接失败，等待后重连", zap.Error(err))
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(3 * time.Second):
				s.reconnectMgr.NotifyDisconnect(err)
				select {
				case <-s.ctx.Done():
					return
				case <-time.After(2 * time.Second):
					// 继续循环
				}
			}
		} else {
			// 连接成功
			s.mu.Lock()
			s.connected = true
			s.mu.Unlock()
			s.reconnectMgr.Reset()
			s.logger.Info("ASR连接成功")

			// 保持连接，等待音频数据或连接断开
			ticker := time.NewTicker(1 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-s.ctx.Done():
					s.mu.Lock()
					s.connected = false
					s.mu.Unlock()
					return
				case <-ticker.C:
					// 定期检查连接是否还活跃
					if !s.transcriber.Activity() {
						s.logger.Info("ASR连接已断开（Activity检查）")
						s.mu.Lock()
						s.connected = false
						s.mu.Unlock()

						select {
						case <-s.ctx.Done():
							return
						case <-time.After(2 * time.Second):
							goto reconnect
						}
					}
				}
			}

		reconnect:
			// 继续循环，尝试重新连接
		}
	}
}

// reconnect 重连
func (s *ASRService) reconnect() error {
	s.mu.Lock()
	if s.connected {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()

	// 重新初始化
	s.transcriber.Init(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			if s.onResult != nil {
				s.onResult(text, isLast, duration, uuid)
			}
		},
		func(err error, isFatal bool) {
			if s.onError != nil {
				s.onError(err)
			}
		},
	)

	// 启动新的接收循环
	go s.receiveLoop()

	return nil
}

// onDisconnect 断开连接回调
func (s *ASRService) onDisconnect(err error) {
	s.logger.Warn("ASR连接断开", zap.Error(err))
}
