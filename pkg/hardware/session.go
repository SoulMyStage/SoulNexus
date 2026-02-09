package hardware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/llm"
	"github.com/code-100-precent/LingEcho/pkg/media"
	"github.com/code-100-precent/LingEcho/pkg/media/encoder"
	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"github.com/code-100-precent/LingEcho/pkg/synthesizer"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	// DefaultTTSSpeedRatio 默认TTS语速倍率
	DefaultTTSSpeedRatio = 0.8 // 硬件端使用0.8（减速20%）
)

// Session hardware websocket session
type Session struct {
	config        *SessionConfig
	ctx           context.Context
	cancel        context.CancelFunc
	stateManager  *HardwareStateManager
	errorHandler  *ErrHandler
	asrService    *ASRService
	ttsService    *TTSService
	llmService    *LLMService
	messageWriter *Writer
	processor     *Processor
	audioManager  *AudioManager
	vadDetector   *VADDetector // VAD detect barge-in
	mu            sync.RWMutex
	db            *gorm.DB
	active        bool
	audioFormat   string            // opus, pcm, etc. // get from hardware by websocket
	sampleRate    int               // 8000, 16000, etc.
	channels      int               // 1, 2
	opusDecoder   media.EncoderFunc // OPUS -> PCM (for ASR)
	opusEncoder   media.EncoderFunc // PCM -> OPUS (for TTS)
}

// NewSession create new hardware session
func NewSession(config *SessionConfig) (*Session, error) {
	if config == nil {
		return nil, NewRecoverableError("Session", "config can not be empty", nil)
	}

	if config.Conn == nil {
		return nil, NewRecoverableError("Session", "websocket connection can not be empty", nil)
	}

	if config.Logger == nil {
		config.Logger = zap.L()
	}

	if config.Context == nil {
		config.Context = context.Background()
	}

	ctx, cancel := context.WithCancel(config.Context)
	stateManager := NewHardwareStateManager()
	errorHandler := NewErrHandler(config.Logger)
	messageWriter := NewWriter(config.Conn, config.Logger)
	// 创建ASR服务（使用默认配置，hello消息后会重新初始化）
	transcriber, err := CreateASR(recognizer.GetGlobalFactory(), config.Credential, config.Language, 0, 0)
	if err != nil {
		cancel()
		return nil, NewRecoverableError("Session", "创建ASR服务失败", err)
	}

	asrService := NewASRService(
		ctx,
		config.Credential,
		config.Language,
		transcriber,
		errorHandler,
		config.Logger,
	)

	// 创建TTS服务（使用默认配置，hello消息后会重新初始化）
	synthesizer, err := CreateTTS(config.Credential, config.Speaker, 0, 0)
	if err != nil {
		cancel()
		return nil, NewRecoverableError("Session", "创建TTS服务失败", err)
	}

	ttsService := NewTTSService(
		ctx,
		config.Credential,
		config.Speaker,
		synthesizer,
		errorHandler,
		config.Logger,
	)

	// 创建LLM服务
	llmProvider, err := CreateLLM(ctx, config.Credential, config.SystemPrompt)
	if err != nil {
		cancel()
		return nil, NewRecoverableError("Session", "创建LLM服务失败", err)
	}

	llmService := NewLLMService(
		ctx,
		config.Credential,
		config.SystemPrompt,
		config.LLMModel,
		config.Temperature,
		config.MaxTokens,
		llmProvider,
		errorHandler,
		config.Logger,
	)

	// 创建过滤词管理器
	filterManager, err := NewFilterManager(config.Logger)
	if err != nil {
		config.Logger.Warn("创建过滤词管理器失败，将不使用过滤功能", zap.Error(err))
		filterManager = nil
	}
	// 使用默认采样率，hello消息后会更新
	audioManager := NewAudioManager(16000, 1, config.Logger)

	// 创建 VAD 检测器，使用配置中的参数
	vadDetector := NewVADDetector()
	vadDetector.SetLogger(config.Logger) // 设置日志记录器
	if config.EnableVAD {
		vadDetector.SetEnabled(true)
		vadDetector.SetThreshold(config.VADThreshold)
		// 如果阈值很低（<200），自动降低连续帧数要求以提高灵敏度
		consecutiveFrames := config.VADConsecutiveFrames
		if config.VADThreshold < 200 && consecutiveFrames > 1 {
			consecutiveFrames = 1
			config.Logger.Info("VAD阈值较低，自动降低连续帧数要求以提高灵敏度",
				zap.Float64("threshold", config.VADThreshold),
				zap.Int("originalFrames", config.VADConsecutiveFrames),
				zap.Int("adjustedFrames", consecutiveFrames),
			)
		}
		vadDetector.SetConsecutiveFrames(consecutiveFrames)
	} else {
		vadDetector.SetEnabled(false)
	}

	// 保存数据库连接（用于设备状态更新）
	var db *gorm.DB
	if config.DB != nil {
		db = config.DB
	}

	// 创建消息处理器
	processor := NewProcessor(
		stateManager,
		llmService,
		ttsService,
		messageWriter,
		errorHandler,
		config.Logger,
		synthesizer,
		filterManager,
		audioManager,
		config.Credential,
	)

	session := &Session{
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		stateManager:  stateManager,
		errorHandler:  errorHandler,
		asrService:    asrService,
		ttsService:    ttsService,
		llmService:    llmService,
		messageWriter: messageWriter,
		processor:     processor,
		audioManager:  audioManager,
		vadDetector:   vadDetector,
		db:            db,
		audioFormat:   "opus",
		sampleRate:    16000,
		channels:      1,
		active:        false,
	}

	// 设置默认音频配置（hello消息后会更新）
	processor.SetAudioConfig("opus", 16000, 1, nil)

	// 设置ASR回调
	asrService.SetCallbacks(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			// 处理ASR结果
			incremental := stateManager.UpdateASRText(text, isLast)
			if incremental != "" {
				processor.ProcessASRResult(ctx, incremental)
			}

			// 处理最终结果
			if isLast && text != "" {
				processor.ProcessUserInput(text)
			}
		},
		func(err error) {
			classified := errorHandler.HandleError(err, "ASR")
			if classifiedErr, ok := classified.(*Error); ok && classifiedErr.Type == ErrorTypeFatal {
				stateManager.SetFatalError(true)
				messageWriter.SendError("ASR错误: "+err.Error(), true)
			}
		},
	)

	// 设置processor的session引用（用于goodbye功能）
	processor.SetSession(session)

	return session, nil
}

// Start 启动会话
func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.active {
		return nil
	}

	// 连接ASR服务
	if err := s.asrService.Connect(); err != nil {
		return NewRecoverableError("Session", "连接ASR服务失败", err)
	}

	// 等待ASR连接建立（动态检查，最多等待1秒）
	s.waitForASRConnection(1 * time.Second)

	// 录音功能已禁用

	// 更新设备在线状态（如果有MAC地址和数据库连接）
	if s.db != nil && s.config.MacAddress != "" {
		err := models.UpdateDeviceOnlineStatus(s.db, s.config.MacAddress, true)
		if err != nil {
			s.config.Logger.Warn("更新设备在线状态失败",
				zap.Error(err),
				zap.String("mac_address", s.config.MacAddress))
		} else {
			s.config.Logger.Info("设备已上线",
				zap.String("mac_address", s.config.MacAddress))
		}
	}

	// 发送连接成功消息
	if err := s.messageWriter.SendConnected(); err != nil {
		s.config.Logger.Error("发送连接成功消息失败", zap.Error(err))
	}

	s.active = true

	// 启动消息处理循环
	go s.messageLoop()

	return nil
}

// waitForASRConnection 等待ASR连接建立（动态检查）
func (s *Session) waitForASRConnection(maxWait time.Duration) {
	checkInterval := 50 * time.Millisecond
	elapsed := time.Duration(0)
	startTime := time.Now()

	for elapsed < maxWait {
		if s.asrService.IsConnected() {
			s.config.Logger.Info("ASR连接已建立",
				zap.Duration("elapsed", time.Since(startTime)),
			)
			return
		}
		time.Sleep(checkInterval)
		elapsed += checkInterval
	}

	// 超时后记录警告，但继续执行（ASR有自动重连机制）
	s.config.Logger.Warn("ASR连接未在预期时间内建立，继续执行（将自动重连）",
		zap.Duration("maxWait", maxWait),
	)
}

// Stop 停止会话
func (s *Session) Stop() error {
	return s.stopWithReason("user_disconnect")
}

// StopWithTimeout 因超时停止会话
func (s *Session) StopWithTimeout() error {
	return s.stopWithReason("timeout")
}

// StopWithError 因错误停止会话
func (s *Session) StopWithError(err error) error {
	// 记录错误到设备错误日志
	if s.db != nil && s.config.MacAddress != "" {
		logErr := models.LogDeviceError(s.db, s.config.MacAddress, s.config.MacAddress,
			"session_error", "ERROR", "SESSION_STOP", err.Error(), "", "")
		if logErr != nil {
			s.config.Logger.Warn("记录设备错误失败", zap.Error(logErr))
		}
	}

	return s.stopWithReason("error")
}

// stopWithReason 带原因停止会话
func (s *Session) stopWithReason(reason string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.active {
		return nil
	}

	// 录音功能已禁用

	// 更新设备离线状态和性能数据（如果有MAC地址和数据库连接）
	if s.db != nil && s.config.MacAddress != "" {
		// 记录最后的性能数据
		if s.config.UserID > 0 {
			perfErr := models.LogDevicePerformance(s.db, s.config.MacAddress, s.config.MacAddress,
				0.0, 0.0, 0.0, 0) // 停止时的性能数据为0
			if perfErr != nil {
				s.config.Logger.Warn("记录设备性能数据失败", zap.Error(perfErr))
			}
		}

		// 更新设备离线状态
		err := models.UpdateDeviceOnlineStatus(s.db, s.config.MacAddress, false)
		if err != nil {
			s.config.Logger.Warn("更新设备离线状态失败",
				zap.Error(err),
				zap.String("mac_address", s.config.MacAddress))
		} else {
			s.config.Logger.Info("设备已离线",
				zap.String("mac_address", s.config.MacAddress),
				zap.String("stop_reason", reason))
		}
	}

	// 先取消所有TTS操作，避免在关闭过程中产生panic
	if s.stateManager != nil {
		s.stateManager.SetTTSPlaying(false)
		s.stateManager.CancelTTS()
	}

	// 取消session context，这会停止所有相关的goroutine
	s.cancel()

	// 断开ASR服务
	if s.asrService != nil {
		s.asrService.Disconnect()
	}

	// 关闭TTS服务
	if s.ttsService != nil {
		s.ttsService.Close()
	}

	// 关闭LLM服务
	if s.llmService != nil {
		s.llmService.Close()
	}

	// 关闭消息写入器
	if s.messageWriter != nil {
		s.messageWriter.Close()
	}

	// 显式关闭WebSocket连接（参考xiaozhi-server）
	if s.config.Conn != nil {
		// 发送正常关闭消息
		closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		if err := s.config.Conn.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(time.Second)); err != nil {
			s.config.Logger.Debug("发送WebSocket关闭消息失败", zap.Error(err))
		}

		// 关闭连接
		if err := s.config.Conn.Close(); err != nil {
			s.config.Logger.Debug("关闭WebSocket连接时出错", zap.Error(err))
		} else {
			s.config.Logger.Debug("WebSocket连接已关闭")
		}
	}

	// 清空状态
	if s.stateManager != nil {
		s.stateManager.Clear()
	}

	// 清空音频管理器
	if s.audioManager != nil {
		s.audioManager.Clear()
	}

	s.active = false

	return nil
}

// HandleAudio 处理音频数据（解决TTS冲突问题）
func (s *Session) HandleAudio(data []byte) error {
	s.mu.RLock()
	active := s.active
	audioManager := s.audioManager
	vadDetector := s.vadDetector
	ttsPlaying := s.stateManager.IsTTSPlaying()
	s.mu.RUnlock()

	if !active {
		return NewRecoverableError("Session", "会话未激活", nil)
	}

	// 检查状态
	if s.stateManager.IsFatalError() {
		return nil
	}

	// 处理音频数据：如果是OPUS格式，需要先解码为PCM
	var pcmData []byte
	if s.audioFormat == "opus" && s.opusDecoder != nil {
		audioPacket := &media.AudioPacket{Payload: data}
		frames, err := s.opusDecoder(audioPacket)
		if err != nil {
			s.config.Logger.Warn("OPUS解码失败", zap.Error(err), zap.Int("dataSize", len(data)))
			return nil
		}
		if len(frames) > 0 {
			if af, ok := frames[0].(*media.AudioPacket); ok {
				pcmData = af.Payload
			}
		}
	} else {
		// 已经是PCM格式，直接使用
		pcmData = data
	}

	// 录音功能已禁用

	if len(pcmData) == 0 {
		return nil
	}

	// 关键改进：TTS播放期间完全阻止ASR处理
	if ttsPlaying {
		// 1. 首先使用音频管理器进行严格的回音过滤
		filteredData, shouldProcess := audioManager.ProcessInputAudio(pcmData, true)
		if !shouldProcess {
			// 被音频管理器过滤掉，直接返回
			return nil
		}

		// 2. 即使通过了音频管理器过滤，在TTS播放期间也要非常谨慎
		// 使用更严格的VAD检测，只有非常明确的用户语音才会中断TTS
		if vadDetector.CheckBargeIn(filteredData, true) {
			s.config.Logger.Info("检测到强烈的用户说话信号，中断 TTS")

			// 优雅地取消 TTS 播放：先设置状态，再取消context
			s.stateManager.SetTTSPlaying(false)
			s.stateManager.CancelTTS()

			// 通知音频管理器TTS被中断
			if audioManager != nil {
				audioManager.NotifyTTSEnd()
			}

			// 发送TTS结束消息给前端
			if err := s.messageWriter.SendTTSEnd(); err != nil {
				s.config.Logger.Warn("发送TTS结束消息失败", zap.Error(err))
			}

			// 等待一小段时间让TTS完全停止，避免残留音频干扰
			time.Sleep(200 * time.Millisecond)

			// 继续处理音频（用户开始说话了）
			return s.asrService.SendAudio(filteredData)
		}

		// TTS 播放中且未检测到明确的用户说话，完全不发送到ASR
		s.config.Logger.Debug("TTS播放中，音频被过滤",
			zap.Int("dataSize", len(pcmData)),
			zap.Int("filteredSize", len(filteredData)))
		return nil
	}

	// TTS 未播放，正常处理音频
	// 使用音频管理器智能处理输入音频（解决TTS冲突）
	processedData, shouldProcess := audioManager.ProcessInputAudio(pcmData, false)
	if !shouldProcess {
		// 被过滤（可能是TTS回音或无效音频）
		s.config.Logger.Debug("音频被过滤",
			zap.Int("dataSize", len(pcmData)),
			zap.String("reason", "audio_manager_filter"))
		return nil
	}

	// 发送到ASR服务
	return s.asrService.SendAudio(processedData)
}

// shouldSuppressError 判断是否应该抑制错误日志
func (s *Session) shouldSuppressError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()
	// 抑制ASR连接相关的错误，这些错误会由重连机制处理
	suppressKeywords := []string{
		"服务未连接",
		"recognizer not running",
		"发送音频失败: recognizer not running",
	}

	for _, keyword := range suppressKeywords {
		if strings.Contains(errMsg, keyword) {
			return true
		}
	}

	return false
}

// HandleText 处理文本消息
func (s *Session) HandleText(data []byte) error {
	s.mu.RLock()
	active := s.active
	processor := s.processor
	ctx := s.ctx
	s.mu.RUnlock()

	if !active {
		return NewRecoverableError("Session", "会话未激活", nil)
	}

	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		s.config.Logger.Warn("解析文本消息失败", zap.Error(err))
		return nil
	}

	msgType, ok := msg["type"].(string)
	if !ok {
		return nil
	}

	switch msgType {
	case MessageTypeNewSession:
		// 新会话，清空消息历史
		processor.Clear()
		s.stateManager.Clear()
		s.audioManager.Clear()
		s.config.Logger.Info("新会话开始")

	case MessageTypePing:
		// 心跳消息，发送pong响应
		if err := s.messageWriter.SendPong(); err != nil {
			s.config.Logger.Warn("发送pong响应失败", zap.Error(err))
		} else {
			s.config.Logger.Debug("收到ping，已发送pong响应")
		}

	case MessageTypeAbort:
		// 中断当前操作
		s.handleAbortMessage()

	case "hello":
		// xiaozhi协议hello消息处理
		s.handleHelloMessage(msg)

	default:
		// 其他消息由processor处理
		processor.HandleTextMessage(ctx, data)
	}

	return nil
}

// handleAbortMessage 处理abort消息 - 中断当前所有操作
func (s *Session) handleAbortMessage() {
	s.config.Logger.Info("收到abort消息，中断当前操作")

	// 1. 中断TTS播放
	if s.stateManager != nil {
		s.stateManager.SetTTSPlaying(false)
		s.stateManager.CancelTTS()
		s.config.Logger.Debug("已中断TTS播放")
	}

	// 2. 清空所有状态
	if s.stateManager != nil {
		s.stateManager.Clear()
		s.config.Logger.Debug("已清空状态管理器")
	}

	// 3. 清空音频管理器
	if s.audioManager != nil {
		s.audioManager.Clear()
		s.config.Logger.Debug("已清空音频管理器")
	}

	// 4. 重置VAD检测器
	if s.vadDetector != nil {
		// VAD检测器没有Clear方法，但会在下次使用时自动重置
		s.config.Logger.Debug("VAD检测器将在下次使用时重置")
	}

	// 5. 清空处理器状态
	if s.processor != nil {
		s.processor.Clear()
		s.config.Logger.Debug("已清空处理器状态")
	}

	// 6. 发送TTS停止消息给客户端
	if s.messageWriter != nil {
		if err := s.messageWriter.SendTTSEnd(); err != nil {
			s.config.Logger.Warn("发送TTS停止消息失败", zap.Error(err))
		} else {
			s.config.Logger.Debug("已发送TTS停止消息")
		}
	}

	// 7. 断开ASR连接（如果需要的话）
	if s.asrService != nil {
		// 注意：这里不完全断开ASR，只是重置状态，保持连接以便后续使用
		s.config.Logger.Debug("ASR服务保持连接，状态已重置")
	}

	s.config.Logger.Info("abort消息处理完成，所有操作已中断")
}

// handleHelloMessage 处理xiaozhi协议的hello消息
func (s *Session) handleHelloMessage(msg map[string]interface{}) {
	s.config.Logger.Info("收到hello消息", zap.Any("message", msg))

	// 提取audio_params
	audioFormat := "opus"
	sampleRate := 16000
	channels := 1
	frameDuration := "60ms"

	if audioParams, ok := msg["audio_params"].(map[string]interface{}); ok {
		if format, ok := audioParams["format"].(string); ok {
			audioFormat = format
		}
		if rate, ok := audioParams["sample_rate"].(float64); ok {
			sampleRate = int(rate)
		}
		if ch, ok := audioParams["channels"].(float64); ok {
			channels = int(ch)
		}
		if frameDur, ok := audioParams["frame_duration"].(float64); ok {
			frameDuration = fmt.Sprintf("%dms", int(frameDur))
		}
	}

	// 更新会话音频配置
	s.mu.Lock()
	s.audioFormat = audioFormat
	s.sampleRate = sampleRate
	s.channels = channels
	s.mu.Unlock()

	// 如果是OPUS格式，初始化编解码器
	if audioFormat == "opus" {
		if err := s.initializeOpusCodecs(sampleRate, channels, frameDuration); err != nil {
			s.config.Logger.Error("初始化OPUS编解码器失败", zap.Error(err))
			s.messageWriter.SendError(fmt.Sprintf("初始化OPUS编解码器失败: %v", err), true)
			return
		}

		// 更新processor的音频配置
		s.processor.SetAudioConfig(audioFormat, sampleRate, channels, s.opusEncoder)
	} else {
		// PCM格式
		s.processor.SetAudioConfig(audioFormat, sampleRate, channels, nil)
	}

	// 重新初始化ASR和TTS服务（使用正确的采样率）
	if err := s.reinitializeServices(sampleRate, channels); err != nil {
		s.config.Logger.Error("重新初始化服务失败", zap.Error(err))
		s.messageWriter.SendError(fmt.Sprintf("重新初始化服务失败: %v", err), true)
		return
	}

	// 更新音频管理器配置
	s.audioManager = NewAudioManager(sampleRate, channels, s.config.Logger)

	// 提取features
	var features map[string]interface{}
	if feat, ok := msg["features"].(map[string]interface{}); ok {
		features = feat
	}

	// 发送Welcome响应
	sessionID, err := s.messageWriter.SendWelcome(audioFormat, sampleRate, channels, features)
	if err != nil {
		s.config.Logger.Error("发送Welcome响应失败", zap.Error(err))
	} else {
		s.config.Logger.Info("已发送Welcome响应",
			zap.String("audioFormat", audioFormat),
			zap.Int("sampleRate", sampleRate),
			zap.Int("channels", channels),
			zap.String("sessionID", sessionID),
		)
	}
}

// initializeOpusCodecs 初始化OPUS编解码器
func (s *Session) initializeOpusCodecs(sampleRate, channels int, frameDuration string) error {
	// 创建OPUS解码器（OPUS -> PCM，用于ASR）
	opusDecoder, err := encoder.CreateDecode(
		media.CodecConfig{
			Codec:         "opus",
			SampleRate:    sampleRate,
			Channels:      channels,
			BitDepth:      16,
			FrameDuration: frameDuration,
		},
		media.CodecConfig{
			Codec:         "pcm",
			SampleRate:    sampleRate,
			Channels:      channels,
			BitDepth:      16,
			FrameDuration: frameDuration,
		},
	)
	if err != nil {
		return fmt.Errorf("创建OPUS解码器失败: %w", err)
	}
	s.opusDecoder = opusDecoder

	// 创建OPUS编码器（PCM -> OPUS，用于TTS）
	opusEncoder, err := encoder.CreateEncode(
		media.CodecConfig{
			Codec:         "opus",
			SampleRate:    sampleRate,
			Channels:      channels,
			BitDepth:      16,
			FrameDuration: frameDuration,
		},
		media.CodecConfig{
			Codec:         "pcm",
			SampleRate:    sampleRate,
			Channels:      channels,
			BitDepth:      16,
			FrameDuration: frameDuration,
		},
	)
	if err != nil {
		return fmt.Errorf("创建OPUS编码器失败: %w", err)
	}
	s.opusEncoder = opusEncoder

	return nil
}

// reinitializeServices 重新初始化ASR和TTS服务
func (s *Session) reinitializeServices(sampleRate, channels int) error {
	transcriberFactory := recognizer.GetGlobalFactory()

	// 停止旧的ASR服务
	s.config.Logger.Info("停止旧的ASR服务")
	if s.asrService != nil {
		s.asrService.Disconnect()
	}

	// 重新初始化ASR服务（使用硬件的采样率）
	s.config.Logger.Info("重新初始化ASR服务",
		zap.Int("sampleRate", sampleRate),
		zap.Int("channels", channels),
	)
	transcriber, err := CreateASR(transcriberFactory, s.config.Credential, s.config.Language, sampleRate, channels)
	if err != nil {
		return fmt.Errorf("重新初始化ASR服务失败: %w", err)
	}

	// 创建新的ASR服务
	newASRService := NewASRService(
		s.ctx,
		s.config.Credential,
		s.config.Language,
		transcriber,
		s.errorHandler,
		s.config.Logger,
	)

	// 设置ASR回调
	newASRService.SetCallbacks(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			incremental := s.stateManager.UpdateASRText(text, isLast)
			if incremental != "" {
				s.processor.ProcessASRResult(s.ctx, incremental)
			}

			// 处理最终结果
			if isLast && text != "" {
				s.processor.ProcessUserInput(text)
			}
		},
		func(err error) {
			classified := s.errorHandler.HandleError(err, "ASR")
			if classifiedErr, ok := classified.(*Error); ok && classifiedErr.Type == ErrorTypeFatal {
				s.stateManager.SetFatalError(true)
				s.messageWriter.SendError("ASR错误: "+err.Error(), true)
			}
		},
	)

	// 连接ASR服务
	if err := newASRService.Connect(); err != nil {
		return fmt.Errorf("连接ASR服务失败: %w", err)
	}

	s.asrService = newASRService
	s.config.Logger.Info("ASR服务已重新初始化")

	// 等待ASR连接建立（动态检查）
	s.waitForASRConnection(1 * time.Second)

	// 重新初始化TTS服务（使用硬件的采样率）
	s.config.Logger.Info("重新初始化TTS服务",
		zap.Int("sampleRate", sampleRate),
		zap.Int("channels", channels),
	)
	synthesizer, err := CreateTTS(s.config.Credential, s.config.Speaker, sampleRate, channels)
	if err != nil {
		return fmt.Errorf("重新初始化TTS服务失败: %w", err)
	}

	// 创建新的TTS服务
	newTTSService := NewTTSService(
		s.ctx,
		s.config.Credential,
		s.config.Speaker,
		synthesizer,
		s.errorHandler,
		s.config.Logger,
	)

	// 更新processor中的synthesizer引用
	s.processor.SetSynthesizer(synthesizer)

	s.ttsService = newTTSService
	s.config.Logger.Info("TTS服务已重新初始化")

	return nil
}

// IsActive 检查会话是否活跃
func (s *Session) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// RecordAIResponse 记录AI回复（录音功能已禁用）
func (s *Session) RecordAIResponse(text string) {
	// 录音功能已禁用
}

// messageLoop 消息处理循环
func (s *Session) messageLoop() {
	defer func() {
		// 当消息循环退出时，触发优雅关闭
		s.config.Logger.Info("消息循环退出，触发会话关闭")
		// 调用Stop方法确保所有资源正确清理
		if err := s.Stop(); err != nil {
			s.config.Logger.Error("会话关闭失败", zap.Error(err))
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			s.config.Logger.Info("消息循环退出")
			return
		default:
		}

		messageType, message, err := s.config.Conn.ReadMessage()
		if err != nil {
			// 检查是否是正常的关闭错误
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				s.config.Logger.Debug("WebSocket连接正常关闭", zap.Error(err))
			} else {
				s.config.Logger.Debug("读取WebSocket消息失败", zap.Error(err))
			}
			// 直接返回，defer中的Stop会处理清理
			return
		}

		switch messageType {
		case websocket.BinaryMessage:
			// 音频消息
			if err := s.HandleAudio(message); err != nil {
				// 只记录非ASR连接相关的错误，避免日志泛滥
				if !s.shouldSuppressError(err) {
					s.config.Logger.Warn("处理音频消息失败", zap.Error(err))
				}
			}
		case websocket.TextMessage:
			// 文本消息
			if err := s.HandleText(message); err != nil {
				s.config.Logger.Warn("处理文本消息失败", zap.Error(err))
			}
		}
	}
}

// CreateASR 创建ASR服务
func CreateASR(transcriberFactory *recognizer.DefaultTranscriberFactory, credential *models.UserCredential, language string, sampleRate, channels int) (recognizer.TranscribeService, error) {
	asrProvider := credential.GetASRProvider()
	if asrProvider == "" {
		return nil, NewRecoverableError("Factory", "ASR provider未配置", nil)
	}
	// 构建配置
	asrConfig := make(map[string]interface{})
	asrConfig["provider"] = asrProvider
	asrConfig["language"] = language

	if credential.AsrConfig != nil {
		for key, value := range credential.AsrConfig {
			asrConfig[key] = value
		}
	}

	// 设置音频参数
	if sampleRate > 0 {
		asrConfig["sampleRate"] = sampleRate
		asrConfig["sample_rate"] = sampleRate
	}
	if channels > 0 {
		asrConfig["channels"] = channels
	}

	// 验证提供商支持
	vendor := recognizer.GetVendor(asrProvider)
	if transcriberFactory != nil && !transcriberFactory.IsVendorSupported(vendor) {
		supported := transcriberFactory.GetSupportedVendors()
		return nil, NewRecoverableError("Factory", fmt.Sprintf("不支持的ASR提供商: %s, 支持的提供商: %v", asrProvider, supported), nil)
	}

	// 解析配置
	config, err := recognizer.NewTranscriberConfigFromMap(asrProvider, asrConfig, language)
	if err != nil {
		return nil, NewRecoverableError("Factory", "解析ASR配置失败", err)
	}

	// 创建服务
	if transcriberFactory == nil {
		transcriberFactory = recognizer.GetGlobalFactory()
	}
	asrService, err := transcriberFactory.CreateTranscriber(config)
	if err != nil {
		return nil, NewRecoverableError("Factory", "创建ASR服务失败", err)
	}

	return asrService, nil
}

// CreateTTS 创建TTS服务
func CreateTTS(credential *models.UserCredential, speaker string, sampleRate, channels int) (synthesizer.SynthesisService, error) {
	ttsProvider := credential.GetTTSProvider()
	if ttsProvider == "" {
		return nil, NewRecoverableError("Factory", "TTS provider未配置", nil)
	}

	ttsConfig := make(synthesizer.TTSCredentialConfig)
	ttsConfig["provider"] = ttsProvider

	if credential.TtsConfig != nil {
		for key, value := range credential.TtsConfig {
			ttsConfig[key] = value
		}
	}

	if _, exists := ttsConfig["voiceType"]; !exists && speaker != "" {
		ttsConfig["voiceType"] = speaker
	}
	if _, exists := ttsConfig["voice_type"]; !exists && speaker != "" {
		ttsConfig["voice_type"] = speaker
	}

	// 设置音频参数
	if sampleRate > 0 {
		ttsConfig["sampleRate"] = sampleRate
		ttsConfig["sample_rate"] = sampleRate
	}
	if channels > 0 {
		ttsConfig["channels"] = channels
	}

	// 设置默认语速
	setDefaultTTSSpeed(ttsConfig, ttsProvider)

	ttsService, err := synthesizer.NewSynthesisServiceFromCredential(ttsConfig)
	if err != nil {
		return nil, NewRecoverableError("Factory", "创建TTS服务失败", err)
	}
	return ttsService, nil
}

// setDefaultTTSSpeed 设置默认TTS语速
func setDefaultTTSSpeed(ttsConfig synthesizer.TTSCredentialConfig, provider string) {
	// 检查是否已经设置了语速
	if _, exists := ttsConfig["speedRatio"]; exists {
		return
	}
	if _, exists := ttsConfig["speed_ratio"]; exists {
		return
	}
	if _, exists := ttsConfig["speed"]; exists {
		return
	}

	// 根据提供商设置默认语速
	switch provider {
	case "openai":
		ttsConfig["speed"] = DefaultTTSSpeedRatio
	default:
		// 大多数提供商使用 speedRatio
		ttsConfig["speedRatio"] = DefaultTTSSpeedRatio
	}
}

// CreateLLM 创建LLM服务
func CreateLLM(ctx context.Context, credential *models.UserCredential, systemPrompt string) (llm.LLMProvider, error) {
	provider, err := llm.NewLLMProvider(ctx, credential.LLMProvider, credential.LLMApiKey, credential.LLMApiURL, systemPrompt)
	if err != nil {
		return nil, NewRecoverableError("Factory", "创建LLM服务失败", err)
	}

	return provider, nil
}
