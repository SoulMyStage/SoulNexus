package hardware

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"go.uber.org/zap"
)

// ASRService hardware asr service
type ASRService struct {
	ctx          context.Context
	cancel       context.CancelFunc
	transcriber  recognizer.TranscribeService
	errorHandler *ErrHandler
	reconnectMgr *ReconnectManager
	logger       *zap.Logger
	mu           sync.RWMutex
	wg           sync.WaitGroup // 用于等待协程退出
	connected    atomic.Bool    // 使用 atomic 提升性能
	onResult     func(text string, isLast bool, duration time.Duration, uuid string)
	onError      func(err error)
}

// NewASRService create asr service
func NewASRService(
	ctx context.Context,
	transcriber recognizer.TranscribeService,
	errorHandler *ErrHandler,
	logger *zap.Logger,
) *ASRService {
	ctx, cancel := context.WithCancel(ctx)
	service := &ASRService{
		ctx:          ctx,
		cancel:       cancel,
		transcriber:  transcriber,
		errorHandler: errorHandler,
		logger:       logger,
	}
	service.connected.Store(false)

	strategy := NewExponentialBackoffStrategy()
	reconnectMgr := NewManager(ctx, logger, strategy)
	reconnectMgr.SetReconnectCallback(service.reconnect)
	reconnectMgr.SetDisconnectCallback(service.onDisconnect)
	service.reconnectMgr = reconnectMgr
	return service
}

// SetCallbacks set callback
func (s *ASRService) SetCallbacks(
	onResult func(text string, isLast bool, duration time.Duration, uuid string),
	onError func(err error),
) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onResult = onResult
	s.onError = onError
}

// Connect connect asr service
func (s *ASRService) Connect() error {
	if s.connected.Load() {
		return nil
	}
	s.mu.Lock()
	if s.connected.Load() {
		s.mu.Unlock()
		return nil
	}
	onResult := s.onResult
	onError := s.onError
	s.transcriber.Init(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			if onResult != nil {
				onResult(text, isLast, duration, uuid)
			}
		},
		func(err error, isFatal bool) {
			if onError != nil {
				onError(err)
			}
			if err != nil {
				classified := s.errorHandler.Classify(err, "ASR")
				if classified.Type == ErrorTypeFatal {
					s.connected.Store(false)
				} else if classified.Type == ErrorTypeTransient {
					s.reconnectMgr.NotifyDisconnect(err)
				}
			}
		},
	)
	s.wg.Add(1)
	s.mu.Unlock()
	go s.receiveLoop()
	return nil
}

// Disconnect disconnect asr service
func (s *ASRService) Disconnect() error {
	if !s.connected.Load() {
		return nil
	}
	s.mu.Lock()
	if !s.connected.Load() {
		s.mu.Unlock()
		return nil
	}
	s.mu.Unlock()
	s.logger.Info("[ASRService] --- 开始断开 ASR 服务")
	s.cancel()
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		s.logger.Info("[ASRService] --- 协程已优雅退出")
	case <-time.After(5 * time.Second):
		s.logger.Warn("[ASRService] --- 协程退出超时，强制继续")
	}
	s.mu.Lock()
	if s.transcriber != nil {
		s.transcriber.StopConn()
	}
	s.mu.Unlock()
	s.connected.Store(false)
	s.logger.Info("[ASRService] --- 服务已断开")
	return nil
}

// SendAudio send audio data
func (s *ASRService) SendAudio(data []byte) error {
	connected := s.connected.Load()
	isReconnecting := s.reconnectMgr.IsReconnecting()
	if isReconnecting {
		return nil
	}
	if !connected {
		s.triggerReconnectOnce()
		return nil
	}
	s.mu.RLock()
	transcriber := s.transcriber
	s.mu.RUnlock()
	if transcriber == nil {
		s.triggerReconnectOnce()
		return nil
	}
	if !transcriber.Activity() {
		s.logger.Debug("[ASRService] --- transcriber 不活跃，触发重连")
		s.connected.Store(false)
		s.reconnectMgr.NotifyDisconnect(NewTransientError("ASR", "transcriber不活跃", nil))
		return nil
	}
	if err := transcriber.SendAudioBytes(data); err != nil {
		// 检查特定错误
		if strings.Contains(err.Error(), "not running") {
			s.logger.Debug("[ASRService] --- 检测到 recognizer not running 错误，触发重连")
			s.connected.Store(false)
			s.reconnectMgr.NotifyDisconnect(err)
			return nil
		}
		if !transcriber.Activity() {
			s.connected.Store(false)
			s.reconnectMgr.NotifyDisconnect(err)
		}
		return NewTransientError("ASR", "发送音频失败", err)
	}

	return nil
}

// triggerReconnectOnce 确保重连只触发一次
func (s *ASRService) triggerReconnectOnce() {
	if s.reconnectMgr.IsReconnecting() {
		return
	}
	s.logger.Debug("[ASRService] --- 服务未连接，触发重连")
	s.reconnectMgr.NotifyDisconnect(NewTransientError("ASR", "服务未连接", nil))
}

// IsConnected 检查是否已连接
func (s *ASRService) IsConnected() bool {
	return s.connected.Load()
}

// Activity 检查服务是否活跃
func (s *ASRService) Activity() bool {
	if !s.connected.Load() {
		return false
	}

	s.mu.RLock()
	transcriber := s.transcriber
	s.mu.RUnlock()

	if transcriber == nil {
		return false
	}
	return transcriber.Activity()
}

// receiveLoop 接收循环
func (s *ASRService) receiveLoop() {
	defer s.wg.Done() // 协程退出时通知
	defer s.logger.Info("[ASRService] --- receiveLoop 协程已退出")
	s.logger.Info("[ASRService] --- receiveLoop 协程已启动")
	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("[ASRService] --- 接收循环收到退出信号")
			s.connected.Store(false)
			return
		default:
		}
		err := s.transcriber.ConnAndReceive("")
		if err != nil {
			s.connected.Store(false)
			select {
			case <-s.ctx.Done():
				s.logger.Info("[ASRService] --- 连接因 context 取消而中断")
				return
			default:
			}

			classified := s.errorHandler.Classify(err, "ASR")
			if classified.Type == ErrorTypeFatal {
				s.logger.Error("[ASRService] 连接致命错误", zap.Error(err))
				if s.onError != nil {
					s.onError(classified)
				}
				return
			}

			s.logger.Warn("[ASRService] 连接失败，等待后重连", zap.Error(err))
			select {
			case <-s.ctx.Done():
				return
			case <-time.After(3 * time.Second):
				s.reconnectMgr.NotifyDisconnect(err)
				select {
				case <-s.ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		} else {
			s.connected.Store(true)
			s.reconnectMgr.Reset()
			s.logger.Info("[ASRService] --- 连接成功")
			func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-s.ctx.Done():
						s.connected.Store(false)
						return
					case <-ticker.C:
						s.mu.RLock()
						transcriber := s.transcriber
						s.mu.RUnlock()
						if transcriber != nil && !transcriber.Activity() {
							s.logger.Info("[ASRService] --- 连接已断开（Activity 检查）")
							s.connected.Store(false)
							select {
							case <-s.ctx.Done():
								return
							case <-time.After(2 * time.Second):
								return // 返回到外层循环重连
							}
						}
					}
				}
			}()
		}
	}
}

// reconnect 重连
func (s *ASRService) reconnect() error {
	if s.connected.Load() {
		s.logger.Debug("[ASRService] --- 已连接，跳过重连")
		return nil
	}
	s.mu.Lock()
	if s.connected.Load() {
		s.mu.Unlock()
		s.logger.Debug("[ASRService] --- 已连接（双重检查），跳过重连")
		return nil
	}
	s.logger.Info("[ASRService] 开始重连")
	onResult := s.onResult
	onError := s.onError

	s.transcriber.Init(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			if onResult != nil {
				onResult(text, isLast, duration, uuid)
			}
		},
		func(err error, isFatal bool) {
			if onError != nil {
				onError(err)
			}
		},
	)
	s.wg.Add(1)
	s.mu.Unlock()
	go s.receiveLoop()
	s.logger.Info("[ASRService] --- 重连已启动")
	return nil
}

// onDisconnect 断开连接回调
func (s *ASRService) onDisconnect(err error) {
	s.logger.Warn("[ASRService] --- 连接断开", zap.Error(err))
}
