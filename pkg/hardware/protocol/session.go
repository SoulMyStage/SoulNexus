package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/hardware/sessions"
	"github.com/code-100-precent/LingEcho/pkg/hardware/stream"
	"github.com/code-100-precent/LingEcho/pkg/hardware/tools"
	"github.com/code-100-precent/LingEcho/pkg/llm"
	"github.com/code-100-precent/LingEcho/pkg/logger"
	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"github.com/code-100-precent/LingEcho/pkg/synthesizer"
	"github.com/code-100-precent/LingEcho/pkg/utils"
	"github.com/code-100-precent/LingEcho/pkg/voiceprint"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

const (
	MessageTypeHello  = "hello"
	MessageTypeListen = "listen"
	MessageTypeAbort  = "abort"
	MessageTypePing   = "ping"
)

type HardwareSessionOption struct {
	Conn                 *websocket.Conn
	Logger               *zap.Logger
	AssistantID          uint
	Credential           *models.UserCredential
	LLMModel             string
	SystemPrompt         string
	MaxToken             int
	Speaker              string
	EnableVAD            bool
	VADThreshold         float64
	VADConsecutiveFrames int
	DB                   *gorm.DB
	VoiceprintService    *voiceprint.Service
}

type HardwareSession struct {
	config            *HardwareSessionOption
	conn              *websocket.Conn
	cancel            context.CancelFunc
	writer            *HardwareWriter
	asrPipeline       *sessions.ASRPipeline
	stateManager      *sessions.ASRStateManager
	llmService        *tools.LLMService
	ttsPipeline       *stream.TTSPipeline
	speakerManager    *tools.SpeakerManager
	voiceprintTool    *tools.VoiceprintIdentifyTool // 声纹识别工具
	ctx               context.Context
	logger            *zap.Logger
	mu                sync.RWMutex
	active            bool
	goodbyePending    bool
	llmProcessing     bool
	currentSpeakerID  string                                  // 当前识别的说话人 ID，用于检测用户切换
	ttsServiceCache   map[string]synthesizer.SynthesisService // TTS 服务缓存：speakerID -> service
	db                *gorm.DB
	voiceprintService *voiceprint.Service
}

func NewHardwareSession(ctx context.Context, hardwareConfig *HardwareSessionOption) *HardwareSession {
	if hardwareConfig.Logger == nil {
		hardwareConfig.Logger = zap.L()
	}

	// 记录 VAD 配置
	hardwareConfig.Logger.Info("[Session] 创建会话，VAD 配置",
		zap.Bool("EnableVAD", hardwareConfig.EnableVAD),
		zap.Float64("VADThreshold", hardwareConfig.VADThreshold),
		zap.Int("VADConsecutiveFrames", hardwareConfig.VADConsecutiveFrames))

	sessionCtx, cancel := context.WithCancel(ctx)
	writer := NewHardwareWriter(sessionCtx, hardwareConfig.Conn, hardwareConfig.Logger)

	llmService, err := tools.NewLLMService(&tools.LLMConfig{
		APIKey:       hardwareConfig.Credential.LLMApiKey,
		BaseURL:      hardwareConfig.Credential.LLMApiURL,
		Provider:     hardwareConfig.Credential.LLMProvider,
		SystemPrompt: fmt.Sprintf("你是一个简洁的语音助手。助手信息：%s\n\n核心规则：\n1. 每次回复只说一两句话，简短有力\n2. 不超过%d个token\n3. 纯文本，无Markdown、emoji、特殊符号\n4. 口语化，适合语音播报\n5. 再见/拜拜时调用goodbye工具，切换声音时调用switch_speaker工具", hardwareConfig.SystemPrompt, hardwareConfig.MaxToken),
		MaxTokens:    hardwareConfig.MaxToken,
	}, hardwareConfig.Logger)
	if err != nil {

		logger.Fatal("创建 LLM 服务失败", zap.Error(err))
	}
	tools.RegisterBuiltinTools(llmService)
	speakerManager := tools.NewSpeakerManager(hardwareConfig.Logger)
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
	var session *HardwareSession

	// 创建一个临时变量来保存 session 引用
	var sessionRef *HardwareSession

	pipeline, err := stream.NewTTSPipeline(&stream.TTSPipelineConfig{
		TTSService: stream.NewTTSServiceAdapter(ttsService),
		SendCallback: func(data []byte) error {
			// 记录 TTS 输出音频到 AudioManager（用于回声消除）
			if sessionRef != nil && sessionRef.asrPipeline != nil {
				if audioMgr := sessionRef.asrPipeline.GetAudioManager(); audioMgr != nil {
					audioMgr.RecordTTSOutput(data)
				}
			}
			return writer.SendTTSAudioWithFlowControl(data, 60, 60)
		},
		GetPendingCountFunc: func() int { return writer.GetBinaryChannelLength() },
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
	session = &HardwareSession{
		config:            hardwareConfig,
		logger:            hardwareConfig.Logger,
		active:            false,
		writer:            writer,
		ctx:               sessionCtx,
		cancel:            cancel,
		conn:              hardwareConfig.Conn,
		stateManager:      sessions.NewASRStateManager(),
		llmService:        llmService,
		ttsPipeline:       pipeline,
		speakerManager:    speakerManager,
		ttsServiceCache:   make(map[string]synthesizer.SynthesisService), // 初始化缓存
		db:                hardwareConfig.DB,
		voiceprintService: hardwareConfig.VoiceprintService,
	}
	speakerManager.SetSwitchCallback(func(speakerID string) error {
		return session.switchSpeaker(speakerID, ttsProvider, ttsConfig)
	})
	tools.RegisterGoodbyeTool(llmService, func() error {
		return session.handleGoodbye()
	})
	pipeline.SetOnComplete(func() {
		session.onTTSComplete()
	})

	// 初始化声纹识别工具
	if hardwareConfig.DB != nil && hardwareConfig.VoiceprintService != nil {
		assistantIDStr := fmt.Sprintf("%d", hardwareConfig.AssistantID)
		session.voiceprintTool = tools.NewVoiceprintIdentifyTool(
			hardwareConfig.DB,
			hardwareConfig.VoiceprintService,
			assistantIDStr,
			hardwareConfig.Logger,
		)
		hardwareConfig.Logger.Info("[Session] 声纹识别工具已初始化",
			zap.String("assistantID", assistantIDStr))
	}

	// 设置 session 引用，使 TTS SendCallback 可以访问 AudioManager
	sessionRef = session

	// 预创建常用发音人的 TTS 服务（异步，不阻塞会话创建）
	go session.preloadCommonSpeakers(ttsProvider, ttsConfig)

	return session
}

// preloadCommonSpeakers 预加载常用发音人的 TTS 服务
func (s *HardwareSession) preloadCommonSpeakers(ttsProvider string, baseConfig synthesizer.TTSCredentialConfig) {
	// 预加载列表：优先加载方言和特色音色（这些切换频率高且创建慢）
	speakersToPreload := []struct {
		id       string
		name     string
		priority string // high: 方言/特色, medium: 常用, low: 通用
	}{
		{"101040", "智瑜（四川话）", "high"},
		{"101019", "智瑜（粤语）", "high"},
		{"101004", "智云（男声，成熟）", "medium"},
		{"101007", "智娜（女声，知性）", "medium"},
	}

	s.logger.Info("[Session] 开始预加载发音人 TTS 服务",
		zap.Int("total_count", len(speakersToPreload)))

	preloadedCount := 0
	for _, speaker := range speakersToPreload {
		// 跳过当前已使用的发音人（已经创建了）
		currentSpeaker := s.config.Speaker
		var currentVoiceType int64
		fmt.Sscanf(currentSpeaker, "%d", &currentVoiceType)
		if fmt.Sprintf("%d", currentVoiceType) == speaker.id {
			s.logger.Debug("[Session] 跳过当前使用的发音人",
				zap.String("speaker_id", speaker.id),
				zap.String("name", speaker.name))
			continue
		}

		// 创建配置
		newConfig := make(synthesizer.TTSCredentialConfig)
		for k, v := range baseConfig {
			newConfig[k] = v
		}

		var voiceType int64
		fmt.Sscanf(speaker.id, "%d", &voiceType)
		newConfig["voiceType"] = voiceType

		// 创建 TTS 服务
		startTime := time.Now()
		ttsService, err := synthesizer.NewSynthesisServiceFromCredential(newConfig)
		if err != nil {
			s.logger.Warn("[Session] 预加载发音人失败",
				zap.String("speaker_id", speaker.id),
				zap.String("name", speaker.name),
				zap.String("priority", speaker.priority),
				zap.Error(err))
			continue
		}

		// 保存到缓存
		s.mu.Lock()
		s.ttsServiceCache[speaker.id] = ttsService
		s.mu.Unlock()

		elapsed := time.Since(startTime)
		preloadedCount++
		s.logger.Info("[Session] 预加载发音人成功",
			zap.String("speaker_id", speaker.id),
			zap.String("name", speaker.name),
			zap.String("priority", speaker.priority),
			zap.Duration("耗时", elapsed))
	}

	s.logger.Info("[Session] 发音人预加载完成",
		zap.Int("preloaded_count", preloadedCount),
		zap.Int("cached_count", len(s.ttsServiceCache)))
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

	// 清除之前的声纹识别结果，准备进行新的识别
	if s.voiceprintTool != nil {
		s.voiceprintTool.ClearIdentification()
		s.logger.Info("[Session] 已清除之前的声纹识别结果")
	}

	// 重置当前说话人 ID，允许在新的对话中进行识别
	s.mu.Lock()
	s.currentSpeakerID = ""
	s.mu.Unlock()
	s.logger.Info("[Session] 已重置当前说话人，准备进行新的识别")
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

	// 3. 清空 TTS 音频队列
	if s.writer != nil {
		s.logger.Info("[Session] 清空 TTS 音频队列")
		s.writer.ClearTTSQueue()
	}

	// 4. 重置 TTS 流控
	if s.writer != nil {
		s.logger.Info("[Session] 重置 TTS 流控")
		s.writer.ResetTTSFlowControl()
	}

	// 5. 清除 TTS 播放状态
	if s.asrPipeline != nil {
		s.logger.Info("[Session] 清除 TTS 播放状态")
		s.asrPipeline.SetTTSPlaying(false)
	}

	// 6. 发送 TTS 结束消息
	if s.writer != nil {
		if err := s.writer.SendTTSEnd(); err != nil {
			s.logger.Error("[Session] 发送 TTS 结束消息失败", zap.Error(err))
		} else {
			s.logger.Info("[Session] 已发送 TTS 结束消息")
		}
	}

	// 7. 发送中断确认消息
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
		Asr:                  asrService,
		SampleRate:           sampleRate,
		Channels:             channels,
		FrameDuration:        frameDuration,
		EnableVAD:            s.config.EnableVAD,
		VADThreshold:         s.config.VADThreshold,
		VADConsecutiveFrames: s.config.VADConsecutiveFrames,
		VADServiceURL:        utils.GetEnv("VAD_SERVICE_URL"),
		VADSessionID:         fmt.Sprintf("user_%d", s.config.AssistantID),
	}, s.logger)
	if err != nil {
		s.mu.Unlock()
		s.logger.Error("创建ASRPipeline失败", zap.Error(err))
		return
	}

	s.logger.Info("[Session] ASR Pipeline 创建成功，VAD 配置",
		zap.Bool("EnableVAD", s.config.EnableVAD),
		zap.Float64("VADThreshold", s.config.VADThreshold),
		zap.Int("VADConsecutiveFrames", s.config.VADConsecutiveFrames))

	s.asrPipeline = pipeline

	// 设置 Barge-in 回调：当 VAD 检测到用户说话时，打断 TTS 和 LLM
	if s.config.EnableVAD {
		pipeline.SetBargeInCallback(func() {
			s.logger.Info("[Session] Barge-in 触发：用户说话，打断 TTS 和 LLM")

			// 1. 中断 LLM 生成
			if s.llmService != nil {
				s.llmService.GetProvider().Interrupt()
			}

			// 2. 中断 TTS 播放
			if s.ttsPipeline != nil {
				s.ttsPipeline.Interrupt()
			}

			// 3. 清空 TTS 音频队列
			if s.writer != nil {
				s.writer.ClearTTSQueue()
			}

			// 4. 重置 TTS 流控
			if s.writer != nil {
				s.writer.ResetTTSFlowControl()
			}

			// 5. 发送 TTS 结束消息
			if s.writer != nil {
				if err := s.writer.SendTTSEnd(); err != nil {
					s.logger.Error("[Session] Barge-in 发送 TTS 结束消息失败", zap.Error(err))
				} else {
					s.logger.Info("[Session] Barge-in 已发送 TTS 结束消息")
				}
			}

			// 6. 延迟清除 TTS 播放状态（给扬声器时间播放完缓冲区）
			// 不立即清除，避免 ASR 识别到扬声器残留的声音
			go func() {
				time.Sleep(500 * time.Millisecond) // 延迟 500ms
				if pipeline != nil {
					pipeline.ClearTTSState()
				}
			}()
		})
	}

	pipeline.SetOutputCallback(func(text string, isFinal bool) {
		incrementalText := s.stateManager.UpdateASRText(text, isFinal)
		if incrementalText == "" {
			return
		}
		s.logger.Info(fmt.Sprintf("[Session] --- 处理增量文本 [%s] 是否结束 [%v] ", incrementalText, isFinal))
		pipeline.PrintMetrics()
		if err := s.writer.SendASRResult(incrementalText); err != nil {
			s.logger.Error("[Session] 发送 ASR 结果失败", zap.Error(err))
		}
		s.mu.Lock()
		if s.llmProcessing {
			s.logger.Info("[Session] LLM 正在处理中，忽略新的 ASR 结果",
				zap.String("text", incrementalText))
			s.mu.Unlock()
			return
		}
		s.llmProcessing = true
		s.mu.Unlock()
		// 注意：不在这里设置 SetTTSPlaying(true)
		// 而是在 TTS 实际开始发送音频时设置
		if err := s.writer.SendTTSStart(); err != nil {
			s.logger.Error("[Session] 发送 TTS 开始消息失败", zap.Error(err))
		}
		go func(text string) {
			receivedContent := false
			defer func() {
				s.mu.Lock()
				s.llmProcessing = false
				s.mu.Unlock()
				s.logger.Info("[Session] LLM 处理完成，清除处理标志")
			}()

			// 如果是最终结果，进行声纹识别（仅在用户切换时更新 Prompt）
			var userContext string
			if isFinal && s.voiceprintTool != nil {
				identified, err := s.voiceprintTool.Identify(s.ctx)
				if err != nil {
					s.logger.Warn("[Session] 声纹识别失败",
						zap.Error(err))
				} else {
					s.logger.Info("[Session] 声纹识别成功",
						zap.String("speaker_id", identified.SpeakerID),
						zap.String("speaker_name", identified.SpeakerName),
						zap.Float64("confidence", identified.Confidence))

					// 检查是否是新用户（用户切换）
					s.mu.RLock()
					previousSpeakerID := s.currentSpeakerID
					s.mu.RUnlock()

					if identified.SpeakerID != previousSpeakerID {
						// 用户切换，更新 System Prompt
						s.logger.Info("[Session] 检测到用户切换",
							zap.String("previous_speaker", previousSpeakerID),
							zap.String("current_speaker", identified.SpeakerID))

						// 获取用户描述用于 LLM 上下文
						userContext = s.voiceprintTool.GetUserDescription()
						if userContext != "" {
							s.logger.Info("[Session] 已获取用户描述",
								zap.String("context", userContext))

							// 更新 LLM System Prompt 以包含用户信息
							currentPrompt := s.llmService.GetConfig().SystemPrompt
							// 添加明确的上下文说明，让 LLM 知道后面的信息是什么
							updatedPrompt := fmt.Sprintf("%s\n\n【当前用户信息】\n目前跟你对话的人是：%s\n用户背景：%s",
								currentPrompt,
								identified.SpeakerName,
								userContext)
							s.llmService.SetSystemPrompt(updatedPrompt)
							s.logger.Info("[Session] 已更新 LLM System Prompt 以包含用户信息",
								zap.String("speaker_name", identified.SpeakerName))
						}

						// 更新当前说话人 ID
						s.mu.Lock()
						s.currentSpeakerID = identified.SpeakerID
						s.mu.Unlock()
					} else {
						s.logger.Info("[Session] 同一用户继续说话，跳过 Prompt 更新",
							zap.String("speaker_id", identified.SpeakerID))
					}
				}
			}

			err := s.llmService.QueryStream(text, func(segment string, isComplete bool) error {
				if !isComplete {
					if !receivedContent {
						receivedContent = true
						if s.asrPipeline != nil {
							s.asrPipeline.SetTTSPlaying(true)
							s.logger.Info("[Session] 已设置 TTS 播放状态（LLM 开始返回内容）")
						}
					}
					s.ttsPipeline.OnLLMToken(segment)
				} else {
					s.ttsPipeline.OnLLMComplete()
				}
				return nil
			}, llm.QueryOptions{
				MaxTokens: &s.config.MaxToken,
				Model:     "qwen-flash",
				Stream:    true,
			})
			if err != nil {
				s.logger.Error("[Session] LLM 查询失败", zap.Error(err))
				if !receivedContent {
					s.logger.Info("[Session] LLM 未返回内容就失败，清除 TTS 状态并发送 TTS 结束消息")
					if s.asrPipeline != nil {
						s.asrPipeline.SetTTSPlaying(false)
					}
					if sendErr := s.writer.SendTTSEnd(); sendErr != nil {
						s.logger.Error("[Session] 发送 TTS 结束消息失败", zap.Error(sendErr))
					}
				}
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
	voiceprintTool := s.voiceprintTool
	s.mu.RUnlock()

	if !active {
		return fmt.Errorf("[Session] 会话未激活")
	}

	if pipeline == nil {
		return fmt.Errorf("[Session] ASR Pipeline 未初始化")
	}
	if voiceprintTool != nil {
		voiceprintTool.AddAudioFrame(data)
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

	// 检查缓存中是否已有该发音人的 TTS 服务
	s.mu.RLock()
	cachedService, exists := s.ttsServiceCache[speakerID]
	s.mu.RUnlock()

	if exists {
		// 使用缓存的服务，立即切换
		s.ttsPipeline.UpdateTTSService(stream.NewTTSServiceAdapter(cachedService))
		s.logger.Info("[Session] 使用缓存的 TTS 服务，发音人切换成功",
			zap.String("speaker_id", speakerID))
		return nil
	}

	// 缓存中没有，异步创建新的 TTS 服务
	go func() {
		startTime := time.Now()

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
			s.logger.Error("[Session] 创建新 TTS 服务失败",
				zap.Error(err),
				zap.String("speaker_id", speakerID))
			return
		}

		// 保存到缓存
		s.mu.Lock()
		s.ttsServiceCache[speakerID] = newTTSService
		s.mu.Unlock()

		// 动态更新 TTS 服务
		s.ttsPipeline.UpdateTTSService(stream.NewTTSServiceAdapter(newTTSService))

		elapsed := time.Since(startTime)
		s.logger.Info("[Session] 发音人切换成功（新建服务）",
			zap.String("speaker_id", speakerID),
			zap.Duration("耗时", elapsed))
	}()

	// 立即返回，不等待 TTS 服务创建完成
	s.logger.Info("[Session] 发音人切换请求已提交，正在后台处理")
	return nil
}

// handleGoodbye 处理待机请求
func (s *HardwareSession) handleGoodbye() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.logger.Info("[Session] 收到待机请求，将在 TTS 播放完成后进入待机状态")
	s.goodbyePending = true
	return nil
}

// onTTSComplete TTS 播放完成回调
func (s *HardwareSession) onTTSComplete() {
	s.logger.Info("[TTS Pipeline] 播放完成，发送 TTS 结束消息")

	// 清除TTS播放状态，允许ASR继续处理（带1秒宽限期）
	if s.asrPipeline != nil {
		s.asrPipeline.SetTTSPlaying(false)
	}

	// 注意：不要清除 StateManager！
	// ASR 服务会持续累积文本，StateManager 需要保持状态才能正确提取增量
	// 只在会话结束或用户明确重启时才清除

	if err := s.writer.SendTTSEnd(); err != nil {
		s.logger.Error("[TTS Pipeline] 发送 TTS 结束消息失败", zap.Error(err))
	}
	s.mu.RLock()
	goodbyePending := s.goodbyePending
	s.mu.RUnlock()

	if goodbyePending {
		s.logger.Info("[Session] 检测到待机请求，准备断开连接")
		go func() {
			// 等待一小段时间，确保 TTS 结束消息已发送
			time.Sleep(100 * time.Millisecond)
			s.logger.Info("[Session] 执行待机断开连接")
			if err := s.Stop(); err != nil {
				s.logger.Error("[Session] 待机断开连接失败", zap.Error(err))
			} else {
				s.logger.Info("[Session] 待机断开连接成功，硬件将进入待机状态")
			}
		}()
	}
}
