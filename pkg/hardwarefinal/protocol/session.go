package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/constants"
	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/sessions"
	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/stream"
	"github.com/code-100-precent/LingEcho/pkg/hardwarefinal/tools"
	"github.com/code-100-precent/LingEcho/pkg/llm"
	"github.com/code-100-precent/LingEcho/pkg/logger"
	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"github.com/code-100-precent/LingEcho/pkg/synthesizer"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

const (
	MessageTypeHello  = "hello"
	MessageTypeListen = "listen"
	MessageTypeAbort  = "abort"
	MessageTypePing   = "ping"
)

type HardwareSessionOption struct {
	Conn         *websocket.Conn
	Logger       *zap.Logger
	AssistantID  uint
	Credential   *models.UserCredential
	LLMModel     string
	SystemPrompt string
	MaxToken     int
	Speaker      string
}

type HardwareSession struct {
	config         *HardwareSessionOption
	conn           *websocket.Conn
	cancel         context.CancelFunc
	writer         *HardwareWriter
	asrPipeline    *sessions.ASRPipeline
	stateManager   *sessions.ASRStateManager
	llmService     *tools.LLMService
	ttsPipeline    *stream.TTSPipeline
	speakerManager *tools.SpeakerManager
	ctx            context.Context
	logger         *zap.Logger
	mu             sync.RWMutex
	active         bool
	goodbyePending bool // 待机待处理标志
}

func NewHardwareSession(ctx context.Context, hardwareConfig *HardwareSessionOption) *HardwareSession {
	if hardwareConfig.Logger == nil {
		hardwareConfig.Logger = zap.L()
	}
	sessionCtx, cancel := context.WithCancel(ctx)
	writer := NewHardwareWriter(sessionCtx, hardwareConfig.Conn, hardwareConfig.Logger)

	llmService, err := tools.NewLLMService(&tools.LLMConfig{
		APIKey:       hardwareConfig.Credential.LLMApiKey,
		BaseURL:      hardwareConfig.Credential.LLMApiURL,
		Provider:     hardwareConfig.Credential.LLMProvider,
		SystemPrompt: fmt.Sprintf(hardwareConfig.SystemPrompt + constants.BUILD_SYSTEMPROMPT),
		MaxTokens:    hardwareConfig.MaxToken,
	}, hardwareConfig.Logger)
	if err != nil {

		logger.Fatal("创建 LLM 服务失败", zap.Error(err))
	}
	tools.RegisterBuiltinTools(llmService)

	// 创建发音人管理器
	speakerManager := tools.NewSpeakerManager(hardwareConfig.Logger)

	// 注册发音人切换工具
	tools.RegisterSpeakerTool(llmService, speakerManager)

	ttsProvider := hardwareConfig.Credential.GetTTSProvider()
	ttsConfig := make(synthesizer.TTSCredentialConfig)
	ttsConfig["provider"] = ttsProvider
	if hardwareConfig.Credential.TtsConfig != nil {
		for key, value := range hardwareConfig.Credential.TtsConfig {
			ttsConfig[key] = value
		}
	}
	if _, exists := ttsConfig["voiceType"]; !exists {
		ttsConfig["voiceType"] = hardwareConfig.Speaker
	}
	ttsService, err := synthesizer.NewSynthesisServiceFromCredential(ttsConfig)
	if err != nil {
		logger.Fatal("创建 TTS 服务失败", zap.Error(err))
	}
	pipeline, err := stream.NewTTSPipeline(&stream.TTSPipelineConfig{
		TTSService:          stream.NewTTSServiceAdapter(ttsService),
		SendCallback:        func(data []byte) error { return writer.SendTTSAudioWithFlowControl(data, 60, 60) },
		GetPendingCountFunc: func() int { return writer.GetBinaryChannelLength() },
		TTSWorkers:          2,
		TargetSampleRate:    16000,
		FrameDuration:       60 * time.Millisecond,
		Logger:              hardwareConfig.Logger,
	})
	if err != nil {
		logger.Fatal("创建管道失败", zap.Error(err))
	}
	if err := pipeline.Start(sessionCtx); err != nil {
		logger.Fatal("启动 TTS Pipeline 失败", zap.Error(err))
	}
	session := &HardwareSession{
		config:         hardwareConfig,
		logger:         hardwareConfig.Logger,
		active:         false,
		writer:         writer,
		ctx:            sessionCtx,
		cancel:         cancel,
		conn:           hardwareConfig.Conn,
		stateManager:   sessions.NewASRStateManager(),
		llmService:     llmService,
		ttsPipeline:    pipeline,
		speakerManager: speakerManager,
	}
	speakerManager.SetSwitchCallback(func(speakerID string) error {
		return session.switchSpeaker(speakerID, ttsProvider, ttsConfig)
	})
	pipeline.SetOnComplete(func() {
		logger.Info("[TTS Pipeline] 播放完成，发送 TTS 结束消息")
		if err := writer.SendTTSEnd(); err != nil {
			logger.Error("[TTS Pipeline] 发送 TTS 结束消息失败", zap.Error(err))
		}
	})
	return session
}

func (s *HardwareSession) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		s.logger.Warn("[Session] --- 会话已经启动")
		return nil
	}
	s.logger.Info("[Session] --- 启动会话...")
	s.active = true
	go s.messageLoop()
	s.logger.Info("[Session] --- 会话启动成功，等待 hello 消息")
	return nil
}

// messageLoop message handler loop
func (s *HardwareSession) messageLoop() {
	defer func() {
		s.logger.Info("[Session] --- 消息循环退出，触发会话关闭")
		if err := s.Stop(); err != nil {
			s.logger.Error("[Session] --- 会话关闭失败", zap.Error(err))
		}
	}()

	for {
		select {
		case <-s.ctx.Done():
			s.logger.Info("[Session] --- Context 已取消，退出消息循环")
			return
		default:
		}
		messageType, message, err := s.conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseNoStatusReceived) {
				s.logger.Info("[Session] WebSocket 连接正常关闭", zap.Error(err))
			} else {
				s.logger.Error("[Session] 读取 WebSocket 消息失败", zap.Error(err))
			}
			return
		}
		switch messageType {
		case websocket.BinaryMessage:
			if err := s.HandleAudio(message); err != nil {
				s.logger.Warn("[Session] 处理音频消息失败", zap.Error(err))
			}
		case websocket.TextMessage:
			if err := s.handleText(message); err != nil {
				s.logger.Warn("[Session] 处理文本消息失败", zap.Error(err))
			}
		}
	}
}

// handleText handler text message
func (s *HardwareSession) handleText(data []byte) error {
	s.mu.RLock()
	active := s.active
	s.mu.RUnlock()
	if !active {
		return fmt.Errorf("[Session] --- 会话未激活")
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(data, &msg); err != nil {
		s.logger.Warn("解析文本消息失败", zap.Error(err))
		return nil
	}
	s.logger.Info(fmt.Sprintf("[Session] --- 收到文本消息：%s", string(data)))
	msgType, ok := msg["type"].(string)
	if !ok {
		return nil
	}
	switch msgType {
	case MessageTypeHello:
		s.handleHelloMessage(msg)
	case MessageTypeListen:
		s.handleListenMessage(msg)
	case MessageTypeAbort:
		s.handleAbortMessage()
	case MessageTypePing:
		s.handlePingMessage()
	default:
		s.logger.Warn("[Session] --- 未处理的文本消息类型", zap.String("type", msgType))
	}
	return nil
}

// handlePingMessage 处理 ping 消息
func (s *HardwareSession) handlePingMessage() {
	if err := s.writer.SendPong(); err != nil {
		s.logger.Error("[Session] --- 发送 pong 消息失败", zap.Error(err))
	}
}

// handleListenMessage 处理拾音模式消息
// 消息格式：{"type":"listen","state":"start|stop|detect","mode":"auto|manual"}
func (s *HardwareSession) handleListenMessage(msg map[string]interface{}) {
	state, _ := msg["state"].(string)
	mode, _ := msg["mode"].(string)

	s.logger.Info("[Session] 处理 listen 消息",
		zap.String("state", state),
		zap.String("mode", mode))
}

// handleAbortMessage 处理中断消息
// 消息格式：{"type":"abort"}
func (s *HardwareSession) handleAbortMessage() {
	s.logger.Info("[Session] 收到中断请求，停止 LLM 和 TTS")

	// 1. 中断 LLM 生成
	if s.llmService != nil {
		s.logger.Info("[Session] 中断 LLM 生成")
		s.llmService.GetProvider().Interrupt()
	}

	// 2. 中断 TTS 播放
	if s.ttsPipeline != nil {
		s.logger.Info("[Session] 中断 TTS 播放")
		s.ttsPipeline.Interrupt()
	}

	// 3. 重置 TTS 流控
	if s.writer != nil {
		s.logger.Info("[Session] 重置 TTS 流控")
		s.writer.ResetTTSFlowControl()
	}

	// 4. 发送中断确认消息
	if s.writer != nil {
		if err := s.writer.SendAbortConfirmation(); err != nil {
			s.logger.Error("[Session] 发送中断确认消息失败", zap.Error(err))
		} else {
			s.logger.Info("[Session] 已发送中断确认消息")
		}
	}

	s.logger.Info("[Session] 中断处理完成")
}

// handleHelloMessage 处理hello消息
// {\"type\":\"hello\",\"version\":1,\"features\":{\"aec\":true,\"mcp\":true},\"transport\":\"websocket\",\"audio_params\":{\"format\":\"opus\",\"sample_rate\":16000,\"channels\":1,\"frame_duration\":60}}"}
func (s *HardwareSession) handleHelloMessage(msg map[string]interface{}) {
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
	asrProvider := s.config.Credential.GetASRProvider()
	factory := recognizer.GetGlobalFactory()
	asrConfig := make(map[string]interface{})
	asrConfig["provider"] = asrProvider
	asrConfig["language"] = "zh"
	for key, value := range s.config.Credential.AsrConfig {
		asrConfig[key] = value
	}
	config, err := recognizer.NewTranscriberConfigFromMap(asrProvider, asrConfig, "zh")
	if err != nil {
		s.mu.Unlock()
		s.logger.Error("创建 ASR 配置失败", zap.Error(err))
		return
	}
	asrService, err := factory.CreateTranscriber(config)
	if err != nil {
		s.mu.Unlock()
		s.logger.Error("创建 ASR 服务失败", zap.Error(err))
		return
	}

	pipeline, err := sessions.NewASRPipeline(&sessions.ASRPipelineOption{
		Asr:           asrService,
		SampleRate:    sampleRate,
		Channels:      channels,
		FrameDuration: frameDuration,
	}, s.logger)
	if err != nil {
		s.mu.Unlock()
		s.logger.Error("创建ASRPipeline失败", zap.Error(err))
		return
	}
	pipeline.SetOutputCallback(func(text string, isFinal bool) {
		incrementalText := s.stateManager.UpdateASRText(text, isFinal)
		if incrementalText == "" {
			return
		}
		s.logger.Info("[Session] 处理增量文本",
			zap.String("incrementalText", incrementalText),
			zap.Bool("isFinal", isFinal))
		pipeline.PrintMetrics()
		if err := s.writer.SendASRResult(incrementalText); err != nil {
			s.logger.Error("[Session] 发送 ASR 结果失败", zap.Error(err))
		}
		if err := s.writer.SendTTSStart(); err != nil {
			s.logger.Error("[Session] 发送 TTS 开始消息失败", zap.Error(err))
		}
		s.writer.ResetTTSFlowControl()
		token := 70

		// 异步调用 LLM，避免阻塞 ASR 处理
		go func(text string) {
			err := s.llmService.QueryStream(text, func(segment string, isComplete bool) error {
				s.logger.Info("========== LLM 回调 ==========",
					zap.String("segment", segment),
					zap.Bool("is_complete", isComplete))
				if !isComplete {
					s.ttsPipeline.OnLLMToken(segment)
				} else {
					s.logger.Info("========== LLM 完成，调用 OnLLMComplete ==========")
					s.ttsPipeline.OnLLMComplete()
					s.logger.Info("========== OnLLMComplete 调用完毕 ==========")
				}
				return nil
			}, llm.QueryOptions{
				MaxTokens: &token,
				Model:     "qwen-plus",
				Stream:    true,
			})
			if err != nil {
				s.logger.Error("[Session] LLM 查询失败", zap.Error(err))
				// 发送错误消息给客户端
				if sendErr := s.writer.SendError(fmt.Sprintf("LLM 查询失败: %v", err), false); sendErr != nil {
					s.logger.Error("[Session] 发送错误消息失败", zap.Error(sendErr))
				}
			}
		}(incrementalText)
	})
	s.asrPipeline = pipeline
	dialogID := fmt.Sprintf("dialog_%s", s.writer.sessionID)
	if err := asrService.ConnAndReceive(dialogID); err != nil {
		s.mu.Unlock()
		s.logger.Error("[Session] 连接 ASR 服务失败", zap.Error(err))
		return
	}
	s.mu.Unlock()
	var features map[string]interface{}
	if feat, ok := msg["features"].(map[string]interface{}); ok {
		features = feat
	}
	sessionID, err := s.writer.SendWelcome(audioFormat, sampleRate, channels, features)
	if err != nil {
		s.logger.Error("发送Welcome响应失败", zap.Error(err))
	} else {
		s.logger.Info(fmt.Sprintf("[Session] --- 已发送Welcome响应 audioFormat:%s, sampleRate:%d, channel:%d, sessionId:%s", audioFormat, sampleRate, channels, sessionID))
	}
}

// HandleAudio 处理音频数据
func (s *HardwareSession) HandleAudio(data []byte) error {
	s.mu.RLock()
	active := s.active
	pipeline := s.asrPipeline
	s.mu.RUnlock()

	if !active {
		return fmt.Errorf("[Session] 会话未激活")
	}

	if pipeline == nil {
		return fmt.Errorf("[Session] ASR Pipeline 未初始化")
	}
	err := pipeline.ProcessInput(s.ctx, data)
	if err != nil {
		s.logger.Error("[Session] ASR 处理输入失败", zap.Error(err))
		return err
	}

	return nil
}

// Stop stop hardware session
func (s *HardwareSession) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return nil
	}
	if s.ttsPipeline != nil {
		s.logger.Info("[Session] --- 停止 TTS Pipeline")
		if err := s.ttsPipeline.Stop(); err != nil {
			s.logger.Error("[Session] --- 停止 TTS Pipeline 失败", zap.Error(err))
		}
	}
	s.cancel()
	if s.writer != nil {
		s.writer.Close()
	}
	if s.conn != nil {
		closeMessage := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "")
		if err := s.conn.WriteControl(websocket.CloseMessage, closeMessage, time.Now().Add(time.Second)); err != nil {
			s.logger.Debug("[Session] --- 发送WebSocket关闭消息失败", zap.Error(err))
		}
		if err := s.conn.Close(); err != nil {
			s.logger.Debug("[Session] --- 关闭WebSocket连接时出错", zap.Error(err))
		} else {
			s.logger.Debug("[Session] --- WebSocket连接已关闭")
		}
	}
	s.active = false
	return nil
}

// switchSpeaker 切换发音人
func (s *HardwareSession) switchSpeaker(speakerID string, ttsProvider string, baseConfig synthesizer.TTSCredentialConfig) error {
	s.logger.Info("[Session] 开始切换发音人",
		zap.String("speaker_id", speakerID),
		zap.String("provider", ttsProvider))

	// 创建新的 TTS 配置
	newConfig := make(synthesizer.TTSCredentialConfig)
	for k, v := range baseConfig {
		newConfig[k] = v
	}

	// 更新 voiceType
	var voiceType int64
	fmt.Sscanf(speakerID, "%d", &voiceType)
	newConfig["voiceType"] = voiceType

	// 创建新的 TTS 服务
	newTTSService, err := synthesizer.NewSynthesisServiceFromCredential(newConfig)
	if err != nil {
		return fmt.Errorf("创建新 TTS 服务失败: %w", err)
	}

	// 动态更新 TTS 服务（不重建 Pipeline，速度更快）
	s.ttsPipeline.UpdateTTSService(stream.NewTTSServiceAdapter(newTTSService))

	s.logger.Info("[Session] 发音人切换成功",
		zap.String("speaker_id", speakerID))

	return nil
}
