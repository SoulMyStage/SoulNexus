package sip

import (
	"context"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/llm"
	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"github.com/code-100-precent/LingEcho/pkg/sip/codec"
	"github.com/code-100-precent/LingEcho/pkg/voice/factory"
	"github.com/emiago/sipgo/sip"
	"github.com/pion/rtp"
	"github.com/sirupsen/logrus"
	"go.uber.org/zap"
)

// checkAIAutoAnswer 检查是否需要启动 AI 代接
// 返回值：(是否启动AI代接, SipUser, Assistant, error)
func (as *SipServer) checkAIAutoAnswer(req *sip.Request) (bool, *models.SipUser, *models.Assistant, error) {
	logrus.Debug("🔍 开始检查 AI 自动接听")

	if as.db == nil {
		logrus.Warn("❌ 数据库连接为空，无法检查 AI 自动接听")
		return false, nil, nil, nil
	}

	// 获取被叫号码（To 头中的用户名）
	to := req.To()
	if to == nil {
		logrus.Debug("❌ To 头为空")
		return false, nil, nil, nil
	}

	toUsername := to.Address.User
	if toUsername == "" {
		logrus.Debug("❌ To 用户名为空")
		return false, nil, nil, nil
	}

	logrus.WithField("to_username", toUsername).Info("🔍 检查被叫号码是否有代接方案")

	// 查询 SipUser（同时匹配 username 和 bound_phone_number）
	// 必须同时满足：enabled = true（启用）AND is_active = true（激活）
	var sipUser models.SipUser
	query := as.db.Where("(username = ? OR bound_phone_number = ?) AND enabled = ? AND is_active = ?", toUsername, toUsername, true, true)

	// 先检查是否有匹配的记录（不管是否启用/激活）
	var count int64
	as.db.Model(&models.SipUser{}).Where("username = ? OR bound_phone_number = ?", toUsername, toUsername).Count(&count)
	logrus.WithFields(logrus.Fields{
		"to_username":   toUsername,
		"total_matches": count,
	}).Info("📊 数据库查询统计")

	err := query.First(&sipUser).Error
	if err != nil {
		// 用户不存在或未启用，不启动 AI 代接
		logrus.WithFields(logrus.Fields{
			"to_username": toUsername,
			"error":       err.Error(),
		}).Warn("❌ 未找到匹配的代接方案")
		return false, nil, nil, nil
	}

	logrus.WithFields(logrus.Fields{
		"to_username":  toUsername,
		"sip_user_id":  sipUser.ID,
		"scheme_name":  sipUser.SchemeName,
		"auto_answer":  sipUser.AutoAnswer,
		"assistant_id": sipUser.AssistantID,
		"enabled":      sipUser.Enabled,
		"is_active":    sipUser.IsActive,
	}).Info("✅ 找到匹配的激活代接方案")

	// 检查是否绑定了 AI 助手
	if sipUser.AssistantID == nil || *sipUser.AssistantID == 0 {
		return false, &sipUser, nil, nil
	}

	// 检查是否启用了自动接听
	if !sipUser.AutoAnswer {
		return false, &sipUser, nil, nil
	}

	// 查询 Assistant
	var assistant models.Assistant
	err = as.db.First(&assistant, *sipUser.AssistantID).Error
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"sip_user":     toUsername,
			"assistant_id": *sipUser.AssistantID,
			"error":        err,
		}).Warn("Failed to load assistant for AI auto-answer")
		return false, &sipUser, nil, err
	}

	logrus.WithFields(logrus.Fields{
		"sip_user":     toUsername,
		"assistant_id": assistant.ID,
		"assistant":    assistant.Name,
	}).Info("✅ AI 代接条件满足")

	return true, &sipUser, &assistant, nil
}

// startAIVoiceSession 启动 AI 语音会话
func (as *SipServer) startAIVoiceSession(
	callID string,
	clientRTPAddr *net.UDPAddr,
	sipUser *models.SipUser,
	assistant *models.Assistant,
) error {
	logrus.WithFields(logrus.Fields{
		"call_id":     callID,
		"sip_user":    sipUser.Username,
		"assistant":   assistant.Name,
		"client_addr": clientRTPAddr.String(),
	}).Info("🤖 启动 AI 语音会话")

	// 获取用户凭证
	// 如果 Assistant 配置了 ApiKey 和 ApiSecret，通过它们查找对应的凭证
	// 否则使用 Assistant 用户的第一个凭证
	var credential *models.UserCredential

	// 方案1：如果 Assistant 有 ApiKey 和 ApiSecret，通过它们查找凭证
	if assistant.ApiKey != "" && assistant.ApiSecret != "" {
		var cred models.UserCredential
		if err := as.db.Where("api_key = ? AND api_secret = ?", assistant.ApiKey, assistant.ApiSecret).First(&cred).Error; err == nil {
			credential = &cred
			logrus.WithFields(logrus.Fields{
				"call_id":       callID,
				"api_key":       assistant.ApiKey,
				"credential_id": cred.ID,
				"user_id":       cred.UserID,
				"asr_provider":  cred.GetASRProvider(),
				"tts_provider":  cred.GetTTSProvider(),
			}).Info("✓ 通过 ApiKey/ApiSecret 找到凭证")
		} else {
			logrus.WithFields(logrus.Fields{
				"call_id": callID,
				"api_key": assistant.ApiKey,
				"error":   err,
			}).Warn("⚠️  未找到 ApiKey/ApiSecret 对应的凭证")
		}
	}

	// 方案2：尝试从 Assistant 的用户获取凭证
	if credential == nil && assistant.UserID > 0 {
		var cred models.UserCredential
		if err := as.db.Where("user_id = ?", assistant.UserID).First(&cred).Error; err == nil {
			credential = &cred
			logrus.WithFields(logrus.Fields{
				"call_id":        callID,
				"user_id":        assistant.UserID,
				"credential_id":  cred.ID,
				"asr_provider":   cred.GetASRProvider(),
				"tts_provider":   cred.GetTTSProvider(),
				"has_asr_config": cred.AsrConfig != nil && len(cred.AsrConfig) > 0,
				"has_tts_config": cred.TtsConfig != nil && len(cred.TtsConfig) > 0,
			}).Info("✓ 使用 Assistant 用户的凭证")
		} else {
			logrus.WithFields(logrus.Fields{
				"call_id": callID,
				"user_id": assistant.UserID,
				"error":   err,
			}).Warn("⚠️  未找到 Assistant 用户的凭证")
		}
	}

	// 方案3：如果还是没有，使用第一个可用凭证
	if credential == nil {
		var cred models.UserCredential
		if err := as.db.First(&cred).Error; err == nil {
			credential = &cred
			logrus.WithField("call_id", callID).Warn("⚠️  使用默认凭证（第一个可用凭证）")
		} else {
			return fmt.Errorf("no credential available for AI session")
		}
	}

	// 创建服务工厂
	transcriberFactory := recognizer.GetGlobalFactory()

	// 创建 zap logger（使用 nop logger 或从配置获取）
	zapLogger, _ := zap.NewProduction()
	if zapLogger == nil {
		zapLogger = zap.NewNop()
	}

	serviceFactory := factory.NewServiceFactory(transcriberFactory, zapLogger)

	// 创建 ASR 服务
	asrTranscriber, err := serviceFactory.CreateASR(credential, assistant.Language)
	if err != nil {
		return fmt.Errorf("failed to create ASR service: %w", err)
	}

	// 创建 TTS 服务
	ttsService, err := serviceFactory.CreateTTS(credential, assistant.Speaker)
	if err != nil {
		return fmt.Errorf("failed to create TTS service: %w", err)
	}

	// 创建 LLM Provider
	// 注意：需要将助手的模型配置传递给 LLM Provider
	llmProvider, err := serviceFactory.CreateLLM(
		context.Background(),
		credential,
		assistant.SystemPrompt,
	)
	if err != nil {
		return fmt.Errorf("failed to create LLM provider: %w", err)
	}

	// 如果助手配置了特定的模型，需要设置到 LLM Provider
	// 这里需要检查 LLM Provider 的类型并设置模型
	if assistant.LLMModel != "" {
		// 尝试设置模型（如果 LLM Provider 支持）
		if openaiProvider, ok := llmProvider.(*llm.OpenAIProvider); ok {
			openaiProvider.SetModel(assistant.LLMModel)
			logrus.WithFields(logrus.Fields{
				"call_id": callID,
				"model":   assistant.LLMModel,
			}).Info("🤖 设置 LLM 模型")
		}
	}

	// 创建 VoiceConversationHandler
	handler := NewVoiceConversationHandler(
		callID,
		clientRTPAddr,
		as.rtpConn,
		credential,
		asrTranscriber,
		ttsService,
		llmProvider,
		sipUser, // 传递 SipUser 配置
	)

	// 保存 handler
	as.voiceHandlersMu.Lock()
	as.voiceHandlers[callID] = handler
	as.voiceHandlersMu.Unlock()

	// 启动 handler
	handler.Start()

	// 启动 RTP 接收协程
	go as.receiveRTPForAI(callID, clientRTPAddr, handler)

	logrus.WithField("call_id", callID).Info("✅ AI 语音会话已启动")

	return nil
}

// receiveRTPForAI 接收 RTP 包并转发给 AI handler
func (as *SipServer) receiveRTPForAI(callID string, clientAddr *net.UDPAddr, handler *VoiceConversationHandler) {
	buffer := make([]byte, 1500)

	logrus.WithFields(logrus.Fields{
		"call_id":     callID,
		"client_addr": clientAddr.String(),
	}).Info("📡 开始接收 RTP 包")

	for {
		// 检查 handler 是否还在运行
		select {
		case <-handler.ctx.Done():
			logrus.WithField("call_id", callID).Info("AI handler 已停止，退出 RTP 接收")
			return
		default:
		}

		// 设置读取超时
		// as.rtpConn.SetReadDeadline(time.Now().Add(1 * time.Second))

		n, receivedAddr, err := as.rtpConn.ReadFromUDP(buffer)
		if err != nil {
			// 超时是正常的，继续循环
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			logrus.WithFields(logrus.Fields{
				"call_id": callID,
				"error":   err,
			}).Error("Failed to read RTP data")
			continue
		}

		// 检查是否来自目标客户端
		if !receivedAddr.IP.Equal(clientAddr.IP) {
			continue
		}

		// 解析 RTP 包
		packet := &rtp.Packet{}
		if err := packet.Unmarshal(buffer[:n]); err != nil {
			continue
		}

		// 只处理 PCMU (payload type 0)
		if packet.PayloadType != 0 {
			continue
		}

		// 转发给 handler
		handler.ProcessAudioPacket(packet.Payload)
	}
}

// stopAIVoiceSession 停止 AI 语音会话
func (as *SipServer) stopAIVoiceSession(callID string) {
	as.voiceHandlersMu.Lock()
	handler, exists := as.voiceHandlers[callID]
	if exists {
		delete(as.voiceHandlers, callID)
	}
	as.voiceHandlersMu.Unlock()

	if exists {
		// 在停止前保存录音
		if handler.IsRecording() {
			recordingAudio := handler.GetRecordingAudio()
			if len(recordingAudio) > 0 {
				logrus.WithFields(logrus.Fields{
					"call_id":    callID,
					"audio_size": len(recordingAudio),
				}).Info("📼 保存AI通话录音")

				// 保存录音文件
				go as.saveAIRecording(callID, recordingAudio, handler)
			}
		}

		handler.Stop()
		logrus.WithField("call_id", callID).Info("✅ AI 语音会话已停止")
	}
}

// saveAIRecording 保存AI通话录音
func (as *SipServer) saveAIRecording(callID string, pcmuAudio []byte, handler *VoiceConversationHandler) {
	if as.db == nil {
		logrus.WithField("call_id", callID).Warn("数据库未初始化，无法保存录音")
		return
	}

	// 创建录音目录
	recordDir := "uploads/audio"
	if err := os.MkdirAll(recordDir, 0755); err != nil {
		logrus.WithError(err).Error("Failed to create audio directory")
		return
	}

	// 生成录音文件名
	recordingFile := fmt.Sprintf("%s/recorded_%s.wav", recordDir, callID)

	// 将PCMU转换为PCM16
	pcmBytes := codec.PCMUToPCM16(pcmuAudio)

	// 将[]byte转换为[]int16
	pcmSamples := make([]int16, len(pcmBytes)/2)
	for i := 0; i < len(pcmSamples); i++ {
		low := int16(pcmBytes[i*2])
		high := int16(pcmBytes[i*2+1])
		pcmSamples[i] = high<<8 | low
	}

	// 保存为WAV文件
	if err := saveWAV(recordingFile, pcmSamples, 8000); err != nil {
		logrus.WithFields(logrus.Fields{
			"call_id": callID,
			"error":   err,
		}).Error("保存录音文件失败")
		return
	}

	logrus.WithFields(logrus.Fields{
		"call_id": callID,
		"file":    recordingFile,
		"size":    len(pcmSamples),
	}).Info("✅ AI通话录音文件已保存")

	// 生成录音URL并保存到数据库
	time.Sleep(500 * time.Millisecond) // 等待文件写入完成
	as.saveRecordingURL(callID, recordingFile)

	// 如果handler有SipUser信息，也可以保存为留言
	if handler.sipUser != nil && handler.IsInMessageMode() {
		// 获取主叫号码
		var callerNumber string
		sipCall, err := models.GetSipCallByCallID(as.db, callID)
		if err == nil {
			callerNumber = sipCall.FromUsername
		}

		// 保存留言
		if voicemail, err := handler.SaveVoicemail(as.db, callerNumber, &sipCall.ID); err == nil {
			logrus.WithFields(logrus.Fields{
				"call_id":      callID,
				"voicemail_id": voicemail.ID,
			}).Info("✅ 留言已保存")
		}
	}
}
