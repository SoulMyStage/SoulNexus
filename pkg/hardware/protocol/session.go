package protocol

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/hardware/constants"
	"github.com/code-100-precent/LingEcho/pkg/hardware/sessions"
	"github.com/code-100-precent/LingEcho/pkg/hardware/stream"
	"github.com/code-100-precent/LingEcho/pkg/hardware/tools"
	"github.com/code-100-precent/LingEcho/pkg/logger"
	"github.com/code-100-precent/LingEcho/pkg/synthesizer"
	"github.com/code-100-precent/LingEcho/pkg/voiceprint"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// HardwareSessionOption hardware session options
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

// HardwareSession hardware session
type HardwareSession struct {
	sessionID         string
	config            *HardwareSessionOption
	conn              *websocket.Conn
	cancel            context.CancelFunc
	writer            *HardwareWriter
	asrPipeline       *sessions.ASRPipeline
	stateManager      *sessions.ASRStateManager
	llmService        *tools.LLMService
	ttsPipeline       *stream.TTSPipeline
	speakerManager    *tools.SpeakerManager
	voiceprintTool    *tools.VoiceprintIdentifyTool // voice print tools
	ctx               context.Context
	logger            *zap.Logger
	mu                sync.RWMutex
	active            bool
	goodbyePending    bool
	llmProcessing     bool
	currentSpeakerID  string                                  // current speaker ID, for switch speaker
	ttsServiceCache   map[string]synthesizer.SynthesisService // TTS service cache：speakerID -> service
	db                *gorm.DB
	voiceprintService *voiceprint.Service
	callRecording     *models.CallRecording
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
		SystemPrompt: fmt.Sprintf(constants.LLMSystemPromptTemplate, hardwareConfig.SystemPrompt, hardwareConfig.MaxToken),
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
	var sessionRef *HardwareSession
	pipeline, err := stream.NewTTSPipeline(&stream.TTSPipelineConfig{
		TTSService: stream.NewTTSServiceAdapter(ttsService),
		SendCallback: func(data []byte) error {
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
	sessionID := fmt.Sprintf("%s%d_%d", constants.HARDWARE_SESSION_PREFIX, hardwareConfig.AssistantID, time.Now().UnixNano())
	session = &HardwareSession{
		sessionID:         sessionID,
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
		ttsServiceCache:   make(map[string]synthesizer.SynthesisService),
		db:                hardwareConfig.DB,
		voiceprintService: hardwareConfig.VoiceprintService,
		callRecording: &models.CallRecording{
			UserID:      hardwareConfig.Credential.UserID,
			AssistantID: hardwareConfig.AssistantID,
			SessionID:   sessionID,
			CallType:    "voice",
			CallStatus:  "ongoing",
			StartTime:   time.Now(),
			LLMModel:    hardwareConfig.LLMModel,
			TTSProvider: ttsProvider,
		},
	}
	if hardwareConfig.Speaker != "" {
		session.callRecording.AddSpeaker(hardwareConfig.Speaker)
	}
	if hardwareConfig.DB != nil {
		if err := models.CreateCallRecording(hardwareConfig.DB, session.callRecording); err != nil {
			hardwareConfig.Logger.Error("[Session] 初始化时保存通话记录失败", zap.Error(err))
		} else {
			hardwareConfig.Logger.Info(fmt.Sprintf("[Session] 通话记录已创建 SessionID:%s, RecordingID:%d", sessionID, uint64(session.callRecording.ID)))
		}
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
	if hardwareConfig.DB != nil && hardwareConfig.VoiceprintService != nil {
		assistantIDStr := fmt.Sprintf("%d", hardwareConfig.AssistantID)
		session.voiceprintTool = tools.NewVoiceprintIdentifyTool(
			hardwareConfig.DB,
			hardwareConfig.VoiceprintService,
			assistantIDStr,
			hardwareConfig.Logger,
		)
	}
	sessionRef = session
	go session.preloadCommonSpeakers(ttsConfig)
	return session
}

// preloadCommonSpeakers 预加载常用发音人的 TTS 服务
func (s *HardwareSession) preloadCommonSpeakers(baseConfig synthesizer.TTSCredentialConfig) {
	speakersToPreload := []struct {
		id       string
		name     string
		priority string
	}{
		{"101040", "智瑜（四川话）", "high"},
		{"101019", "智瑜（粤语）", "high"},
		{"101004", "智云（男声，成熟）", "medium"},
		{"101007", "智娜（女声，知性）", "medium"},
	}
	preloadedCount := 0
	for _, speaker := range speakersToPreload {
		currentSpeaker := s.config.Speaker
		var currentVoiceType int64
		fmt.Sscanf(currentSpeaker, "%d", &currentVoiceType)
		if fmt.Sprintf("%d", currentVoiceType) == speaker.id {
			s.logger.Debug("[Session] 跳过当前使用的发音人",
				zap.String("speaker_id", speaker.id),
				zap.String("name", speaker.name))
			continue
		}
		newConfig := make(synthesizer.TTSCredentialConfig)
		for k, v := range baseConfig {
			newConfig[k] = v
		}
		var voiceType int64
		fmt.Sscanf(speaker.id, "%d", &voiceType)
		newConfig["voiceType"] = voiceType
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
}

func (s *HardwareSession) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return nil
	}
	s.active = true
	go s.messageLoop()
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
			if err := s.handleAudio(message); err != nil {
				s.logger.Warn("[Session] 处理音频消息失败", zap.Error(err))
			}
		case websocket.TextMessage:
			if err := s.handleText(message); err != nil {
				s.logger.Warn("[Session] 处理文本消息失败", zap.Error(err))
			}
		}
	}
}

// Stop stop hardware session
func (s *HardwareSession) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return nil
	}
	if s.ttsPipeline != nil {
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
	s.saveCallRecording()
	return nil
}

// switchSpeaker 切换发音人
func (s *HardwareSession) switchSpeaker(speakerID string, ttsProvider string, baseConfig synthesizer.TTSCredentialConfig) error {
	s.logger.Info("[Session] 开始切换发音人",
		zap.String("speaker_id", speakerID),
		zap.String("provider", ttsProvider))
	if s.callRecording != nil {
		s.callRecording.AddSpeaker(speakerID)
		if s.db != nil {
			if err := s.db.Model(s.callRecording).Update("speakers", s.callRecording.Speakers).Error; err != nil {
				s.logger.Warn("[Session] 更新发音人列表失败", zap.Error(err), zap.String("speaker_id", speakerID))
			}
		}
	}

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
	if s.asrPipeline != nil {
		s.asrPipeline.SetTTSPlaying(false)
	}
	if err := s.writer.SendTTSEnd(); err != nil {
		s.logger.Error("[TTS Pipeline] 发送 TTS 结束消息失败", zap.Error(err))
	}
	s.mu.RLock()
	goodbyePending := s.goodbyePending
	s.mu.RUnlock()
	if goodbyePending {
		s.logger.Info("[Session] 检测到待机请求，准备断开连接")
		go func() {
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

// saveCallRecording 保存通话记录
func (s *HardwareSession) saveCallRecording() {
	if s.callRecording == nil || s.db == nil {
		return
	}
	s.callRecording.EndTime = time.Now()
	s.callRecording.Duration = int(s.callRecording.EndTime.Sub(s.callRecording.StartTime).Seconds())
	s.callRecording.CallStatus = "completed"
	if err := s.db.Model(s.callRecording).Updates(map[string]interface{}{
		"end_time":    s.callRecording.EndTime,
		"duration":    s.callRecording.Duration,
		"call_status": "completed",
		"speakers":    s.callRecording.Speakers,
	}).Error; err != nil {
		s.logger.Error("[Session] 更新通话记录失败", zap.Error(err))
		return
	}
	s.logger.Info("[Session] 通话记录已更新",
		zap.Uint64("recordingID", uint64(s.callRecording.ID)),
		zap.String("sessionID", s.callRecording.SessionID),
		zap.Int("duration", s.callRecording.Duration),
		zap.String("speakers", s.callRecording.Speakers))
}
