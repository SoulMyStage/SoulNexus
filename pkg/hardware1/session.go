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
	initialized   bool              // 标记服务是否已初始化（收到hello消息后）
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
	llmProvider, err := CreateLLM(ctx, config.LLMProvider, config.LLMApiKey, config.LLMApiURL, config.SystemPrompt)
	if err != nil {
		cancel()
		return nil, NewRecoverableError("Session", "创建LLM服务失败", err)
	}
	llmService := NewLLMService(
		ctx,
		config.SystemPrompt,
		config.LLMModel,
		config.Temperature,
		config.MaxTokens,
		llmProvider,
		errorHandler,
		config.Logger,
	)
	filterManager, err := NewFilterManager(config.Logger)
	if err != nil {
		config.Logger.Warn("create filter manager failed, will not use filter manager", zap.Error(err))
		filterManager = nil
	}
	vadDetector := NewVADDetector()
	vadDetector.SetLogger(config.Logger)
	if config.EnableVAD {
		vadDetector.SetEnabled(true)
		vadDetector.SetThreshold(config.VADThreshold)
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
	processor := NewProcessor(
		stateManager,
		llmService,
		nil, // ttsService 稍后设置
		messageWriter,
		errorHandler,
		config.Logger,
		nil, // synthesizer 稍后设置
		filterManager,
		nil, // audioManager 稍后设置
		config.TtsConfig,
	)
	session := &Session{
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		stateManager:  stateManager,
		errorHandler:  errorHandler,
		asrService:    nil, //  delay initialization
		ttsService:    nil, //  delay initialization
		llmService:    llmService,
		messageWriter: messageWriter,
		processor:     processor,
		audioManager:  nil, //  delay initialization
		vadDetector:   vadDetector,
		db:            config.DB,
		audioFormat:   "", // 等待 hello 消息
		sampleRate:    0,  // 等待 hello 消息
		channels:      0,  // 等待 hello 消息
		active:        false,
		initialized:   false, //  delay initialization
	}
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
	if s.db != nil && s.config.MacAddress != "" {
		err := models.UpdateDeviceOnlineStatus(s.db, s.config.MacAddress, true)
		if err != nil {
			s.config.Logger.Warn(fmt.Sprintf("[Session] --- 更新设备在线状态失败 mac_address: %s, err: %v", s.config.MacAddress, err))
		} else {
			s.config.Logger.Info(fmt.Sprintf("[Session] --- 设备已上线 mac_address: %s", s.config.MacAddress))
		}
	}
	if err := s.messageWriter.SendConnected(); err != nil {
		s.config.Logger.Error("[Session] --- 发送连接成功消息失败", zap.Error(err))
	}
	s.active = true
	go s.messageLoop()
	return nil
}

// messageLoop message handle loop
func (s *Session) messageLoop() {
	defer func() {
		s.config.Logger.Info("[Session] --- 消息循环退出，触发会话关闭")
		if err := s.Stop(); err != nil {
			s.config.Logger.Error("[Session] --- 会话关闭失败", zap.Error(err))
		}
	}()
	for {
		select {
		case <-s.ctx.Done():
			s.config.Logger.Info("[Session] --- 消息循环退出")
			return
		default:
		}
		messageType, message, err := s.config.Conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				s.config.Logger.Debug("[Session] --- WebSocket连接正常关闭", zap.Error(err))
			} else {
				s.config.Logger.Debug("[Session] --- 读取WebSocket消息失败", zap.Error(err))
			}
			return
		}
		switch messageType {
		case websocket.BinaryMessage:
			if err := s.HandleAudio(message); err != nil {
				if !s.shouldSuppressError(err) {
					s.config.Logger.Warn("[Session] --- 处理音频消息失败", zap.Error(err))
				}
			}
		case websocket.TextMessage:
			if err := s.HandleText(message); err != nil {
				s.config.Logger.Warn("处理文本消息失败", zap.Error(err))
			}
		}
	}
}

// HandleText handler text message
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
		processor.Clear()
		s.stateManager.Clear()
		s.audioManager.Clear()
		s.config.Logger.Info("[Session] --- 新会话开始")
	case MessageTypePing:
		if err := s.messageWriter.SendPong(); err != nil {
			s.config.Logger.Warn("[Session] --- 发送pong响应失败", zap.Error(err))
		} else {
			s.config.Logger.Debug("[Session] --- 收到ping，已发送pong响应")
		}
	case MessageTypeAbort:
		s.handleAbortMessage()
	case MessageTypeHello:
		s.handleHelloMessage(msg)
	default:
		processor.HandleTextMessage(ctx, data)
	}
	return nil
}

// waitForASRConnection 等待ASR连接建立（动态检查）
func (s *Session) waitForASRConnection(maxWait time.Duration) {
	checkInterval := 50 * time.Millisecond
	elapsed := time.Duration(0)
	startTime := time.Now()
	for elapsed < maxWait {
		if s.asrService.IsConnected() {
			s.config.Logger.Info(fmt.Sprintf("[Session] --- 等待ASR连接建立中... %s", time.Since(startTime)))
			return
		}
		time.Sleep(checkInterval)
		elapsed += checkInterval
	}
}

// Stop 停止会话
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return nil
	}
	if s.db != nil && s.config.MacAddress != "" {
		if s.config.UserID > 0 {
			perfErr := models.LogDevicePerformance(s.db, s.config.MacAddress, s.config.MacAddress,
				0.0, 0.0, 0.0, 0)
			if perfErr != nil {
				s.config.Logger.Warn("记录设备性能数据失败", zap.Error(perfErr))
			}
		}
		err := models.UpdateDeviceOnlineStatus(s.db, s.config.MacAddress, false)
		if err != nil {
			s.config.Logger.Warn(fmt.Sprintf("[Session] --- 更新设备离线状态失败 mac_address: %s, err: %v", s.config.MacAddress, err))
		} else {
			s.config.Logger.Info(fmt.Sprintf("[Session] --- 设备已离线 mac_address: %s, reason: disconnect", s.config.MacAddress))
		}
	}
	if s.stateManager != nil {
		s.stateManager.SetTTSPlaying(false)
		s.stateManager.CancelTTS()
	}
	s.cancel()
	if s.asrService != nil {
		s.asrService.Disconnect()
	}
	if s.ttsService != nil {
		s.ttsService.Close()
	}
	if s.llmService != nil {
		s.llmService.Close()
	}
	if s.messageWriter != nil {
		s.messageWriter.Close()
	}
	if s.config.Conn != nil {
		closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		if err := s.config.Conn.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(time.Second)); err != nil {
			s.config.Logger.Error(fmt.Sprintf("[Session] --- 发送WebSocket关闭消息失败 %v", err))
		}
		if err := s.config.Conn.Close(); err != nil {
			s.config.Logger.Error(fmt.Sprintf("[Session] --- 关闭WebSocket连接时出错 %v", err))
		} else {
			s.config.Logger.Error(fmt.Sprintf("[Session] --- WebSocket连接已关闭 %v", err))
		}
	}
	if s.stateManager != nil {
		s.stateManager.Clear()
	}
	if s.audioManager != nil {
		s.audioManager.Clear()
	}
	s.active = false
	return nil
}

// HandleAudio 处理音频数据
func (s *Session) HandleAudio(data []byte) error {
	s.mu.RLock()
	active := s.active
	initialized := s.initialized
	audioManager := s.audioManager
	vadDetector := s.vadDetector
	ttsPlaying := s.stateManager.IsTTSPlaying()
	s.mu.RUnlock()
	if !active {
		return NewRecoverableError("Session", "会话未激活", nil)
	}
	if !initialized {
		s.config.Logger.Debug("[Session] --- 收到音频数据但ASR/TTS服务尚未初始化，丢弃音频数据")
		return nil
	}
	if s.stateManager.IsFatalError() {
		return nil
	}
	var pcmData []byte
	if s.audioFormat == "opus" && s.opusDecoder != nil {
		audioPacket := &media.AudioPacket{Payload: data}
		frames, err := s.opusDecoder(audioPacket)
		if err != nil {
			s.config.Logger.Warn(fmt.Sprintf("[Session] --- OPUS解码失败，数据大小：%d", len(data)))
			return nil
		}
		if len(frames) > 0 {
			if af, ok := frames[0].(*media.AudioPacket); ok {
				pcmData = af.Payload
			}
		}
	} else {
		pcmData = data
	}
	if len(pcmData) == 0 {
		return nil
	}
	processedData, shouldProcess := audioManager.ProcessInputAudio(pcmData, ttsPlaying)
	if !shouldProcess {
		return nil
	}
	if ttsPlaying {
		if vadDetector.CheckBargeIn(processedData, true) {
			s.config.Logger.Info("检测到强烈的用户说话信号，中断 TTS")
			s.stateManager.SetTTSPlaying(false)
			s.stateManager.CancelTTS()
			if audioManager != nil {
				audioManager.NotifyTTSEnd()
			}
			if err := s.messageWriter.SendTTSEnd(); err != nil {
				s.config.Logger.Warn("发送TTS结束消息失败", zap.Error(err))
			}
			time.Sleep(200 * time.Millisecond)
			return s.asrService.SendAudio(processedData)
		}
		return nil
	}
	return s.asrService.SendAudio(processedData)
}

// shouldSuppressError 判断是否应该抑制错误日志
func (s *Session) shouldSuppressError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := err.Error()
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

// handleAbortMessage 处理abort消息 - 中断当前所有操作
func (s *Session) handleAbortMessage() {
	s.config.Logger.Info("[Session] --- 收到abort消息，中断当前操作")
	if s.stateManager != nil {
		s.stateManager.SetTTSPlaying(false)
		s.stateManager.CancelTTS()
	}
	if s.stateManager != nil {
		s.stateManager.Clear()
	}
	if s.audioManager != nil {
		s.audioManager.Clear()
	}
	if s.processor != nil {
		s.processor.Clear()
	}
	if s.messageWriter != nil {
		if err := s.messageWriter.SendTTSEnd(); err != nil {
			s.config.Logger.Warn("[Session] --- 发送TTS停止消息失败", zap.Error(err))
		} else {
			s.config.Logger.Info("[Session] --- 已发送TTS停止消息")
		}
	}
	if s.asrService != nil {
		s.config.Logger.Info("[Session] --- ASR服务保持连接，状态已重置")
	}
}

// handleHelloMessage 处理hello消息
func (s *Session) handleHelloMessage(msg map[string]interface{}) {
	s.config.Logger.Info(fmt.Sprintf("[Session] --- 收到来自硬件的hello消息: %s", msg))
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
	s.mu.Lock()
	if s.initialized {
		s.mu.Unlock()
		s.config.Logger.Warn("[Session] --- ASR/TTS服务已初始化，忽略重复的hello消息")
		var features map[string]interface{}
		if feat, ok := msg["features"].(map[string]interface{}); ok {
			features = feat
		}
		sessionID, err := s.messageWriter.SendWelcome(audioFormat, sampleRate, channels, features)
		if err != nil {
			s.config.Logger.Error("发送Welcome响应失败", zap.Error(err))
		} else {
			s.config.Logger.Info("[Session] --- 已发送Welcome响应（重复hello）",
				zap.String("sessionID", sessionID))
		}
		return
	}
	s.audioFormat = audioFormat
	s.sampleRate = sampleRate
	s.channels = channels
	s.mu.Unlock()
	if err := s.initializeServices(audioFormat, sampleRate, channels, frameDuration); err != nil {
		s.config.Logger.Error("初始化服务失败", zap.Error(err))
		s.messageWriter.SendError(fmt.Sprintf("初始化服务失败: %v", err), true)
		return
	}
	var features map[string]interface{}
	if feat, ok := msg["features"].(map[string]interface{}); ok {
		features = feat
	}
	sessionID, err := s.messageWriter.SendWelcome(audioFormat, sampleRate, channels, features)
	if err != nil {
		s.config.Logger.Error("发送Welcome响应失败", zap.Error(err))
	} else {
		s.config.Logger.Info(fmt.Sprintf("[Session] --- 已发送Welcome响应 audioFormat:%s, sampleRate:%d, channel:%d, sessionId:%s", audioFormat, sampleRate, channels, sessionID))
	}
}

// initializeServices 初始化所有音频相关服务
func (s *Session) initializeServices(audioFormat string, sampleRate, channels int, frameDuration string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.initialized {
		return nil
	}
	s.config.Logger.Info("[Session] --- 初始化音频编解码器")
	if err := s.initializeOpusCodecs(sampleRate, channels, frameDuration); err != nil {
		return fmt.Errorf("初始化OPUS编解码器失败: %w", err)
	}
	s.processor.SetAudioConfig(audioFormat, sampleRate, channels, s.opusEncoder)
	s.config.Logger.Info(fmt.Sprintf("[Session] --- 初始化ASR服务 sample:%d channels:%d", sampleRate, channels))
	transcriber, err := CreateASR(s.config.AsrConfig, s.config.Language, sampleRate, channels)
	if err != nil {
		return fmt.Errorf("创建ASR服务失败: %w", err)
	}
	asrService := NewASRService(
		s.ctx,
		transcriber,
		s.errorHandler,
		s.config.Logger,
	)
	asrService.SetCallbacks(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			incremental := s.stateManager.UpdateASRText(text, isLast)
			if incremental != "" {
				s.processor.ProcessASRResult(s.ctx, incremental)
			}
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
	if err := asrService.Connect(); err != nil {
		return fmt.Errorf("连接ASR服务失败: %w", err)
	}
	s.asrService = asrService
	go s.waitForASRConnection(1 * time.Second)
	s.config.Logger.Info(fmt.Sprintf("[Session] --- 初始化TTS服务 sample:%d channels:%d", sampleRate, channels))
	synthesizer, err := CreateTTS(s.config.TtsConfig, s.config.Speaker, sampleRate, channels)
	if err != nil {
		return fmt.Errorf("创建TTS服务失败: %w", err)
	}
	ttsService := NewTTSService(
		s.ctx,
		s.config.Speaker,
		synthesizer,
		s.errorHandler,
		s.config.Logger,
	)
	s.ttsService = ttsService
	s.processor.SetSynthesizer(synthesizer)
	s.audioManager = NewAudioManager(sampleRate, channels, s.config.Logger)
	s.initialized = true
	s.config.Logger.Info("[Session] --- 硬件通话ASR/TTS/LLM服务初始化完成")
	return nil
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

// IsActive 检查会话是否活跃
func (s *Session) IsActive() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// CreateASR 创建ASR服务
func CreateASR(asrConfig models.ProviderConfig, language string, sampleRate, channels int) (recognizer.TranscribeService, error) {
	asrProvider := asrConfig["provider"].(string)
	if asrProvider == "" {
		return nil, NewRecoverableError("Factory", "ASR provider未配置", nil)
	}
	if sampleRate > 0 {
		asrConfig["sampleRate"] = sampleRate
		asrConfig["sample_rate"] = sampleRate
	}
	if channels > 0 {
		asrConfig["channels"] = channels
	}
	vendor := recognizer.GetVendor(asrProvider)
	transcriberFactory := recognizer.GetGlobalFactory()
	if !transcriberFactory.IsVendorSupported(vendor) {
		return nil, NewRecoverableError(
			"Factory",
			fmt.Sprintf("不支持的ASR提供商: %s, 支持的提供商: %v", asrProvider, transcriberFactory.GetSupportedVendors()),
			nil)
	}
	config, err := recognizer.NewTranscriberConfigFromMap(asrProvider, asrConfig, language)
	if err != nil {
		return nil, NewRecoverableError("Factory", "Parse Config Error", err)
	}
	asrService, err := transcriberFactory.CreateTranscriber(config)
	if err != nil {
		return nil, NewRecoverableError("Factory", "创建ASR服务失败", err)
	}
	return asrService, nil
}

// CreateTTS 创建TTS服务
func CreateTTS(ttsConfig models.ProviderConfig, speaker string, sampleRate, channels int) (synthesizer.SynthesisService, error) {
	ttsProvider := ttsConfig["provider"].(string)
	if ttsProvider == "" {
		return nil, NewRecoverableError("Factory", "TTS provider未配置", nil)
	}
	ttsServiceConfig := make(synthesizer.TTSCredentialConfig)
	ttsServiceConfig["provider"] = ttsProvider
	for key, value := range ttsConfig {
		ttsServiceConfig[key] = value
	}

	if _, exists := ttsConfig["voiceType"]; !exists && speaker != "" {
		ttsServiceConfig["voiceType"] = speaker
	}
	if _, exists := ttsConfig["voice_type"]; !exists && speaker != "" {
		ttsServiceConfig["voice_type"] = speaker
	}
	if sampleRate > 0 {
		ttsServiceConfig["sampleRate"] = sampleRate
		ttsServiceConfig["sample_rate"] = sampleRate
	}
	if channels > 0 {
		ttsServiceConfig["channels"] = channels
	}
	ttsServiceConfig["speedRatio"] = 0.8
	ttsService, err := synthesizer.NewSynthesisServiceFromCredential(ttsServiceConfig)
	if err != nil {
		return nil, NewRecoverableError("Factory", "create tts service", err)
	}
	return ttsService, nil
}

// CreateLLM 创建LLM服务
func CreateLLM(ctx context.Context, llmProvider, apiKey, apiUrl, systemPrompt string) (llm.LLMProvider, error) {
	provider, err := llm.NewLLMProvider(ctx, llmProvider, apiKey, apiUrl, systemPrompt)
	if err != nil {
		return nil, NewRecoverableError("Factory", "创建LLM服务失败", err)
	}

	return provider, nil
}
