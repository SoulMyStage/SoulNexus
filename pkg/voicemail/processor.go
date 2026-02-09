package voicemail

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/LingByte/lingstorage-sdk-go"
	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/config"
	"github.com/code-100-precent/LingEcho/pkg/llm"
	"github.com/code-100-precent/LingEcho/pkg/recognizer"
	"github.com/code-100-precent/LingEcho/pkg/sip/codec"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// VoicemailProcessor 留言处理器
type VoicemailProcessor struct {
	db *gorm.DB
}

// NewVoicemailProcessor 创建留言处理器
func NewVoicemailProcessor(db *gorm.DB) *VoicemailProcessor {
	return &VoicemailProcessor{
		db: db,
	}
}

// ProcessVoicemail 处理留言（上传音频、转录、生成摘要）
func (p *VoicemailProcessor) ProcessVoicemail(
	ctx context.Context,
	voicemailID uint,
	pcmuAudio []byte,
	asrConfig map[string]interface{},
	llmProvider llm.LLMProvider,
) error {
	// 1. 获取留言记录
	voicemail, err := models.GetVoicemailByID(p.db, voicemailID)
	if err != nil {
		return fmt.Errorf("获取留言记录失败: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"voicemail_id": voicemailID,
		"audio_size":   len(pcmuAudio),
	}).Info("🎙️ 开始处理留言")

	// 2. 上传音频到存储
	audioURL, err := p.uploadAudio(ctx, voicemailID, pcmuAudio)
	if err != nil {
		logrus.WithError(err).Error("上传留言音频失败")
		// 不返回错误，继续处理
	} else {
		voicemail.AudioURL = audioURL
		logrus.WithField("audio_url", audioURL).Info("✅ 留言音频上传成功")
	}

	// 3. 语音转文字
	transcribedText, err := p.transcribeAudio(ctx, pcmuAudio, asrConfig)
	if err != nil {
		logrus.WithError(err).Error("留言转录失败")
		voicemail.TranscribeStatus = "failed"
		voicemail.TranscribeError = err.Error()
	} else {
		voicemail.TranscribedText = transcribedText
		voicemail.TranscribeStatus = "completed"
		now := time.Now()
		voicemail.TranscribedAt = &now
		logrus.WithField("text", transcribedText).Info("✅ 留言转录成功")
	}

	// 4. 生成摘要和关键词
	if transcribedText != "" && llmProvider != nil {
		summary, keywords, err := p.generateSummary(ctx, transcribedText, llmProvider)
		if err != nil {
			logrus.WithError(err).Warn("生成留言摘要失败")
		} else {
			voicemail.Summary = summary
			voicemail.Keywords = keywords
			logrus.WithFields(logrus.Fields{
				"summary":  summary,
				"keywords": keywords,
			}).Info("✅ 留言摘要生成成功")
		}
	}

	// 5. 更新留言记录
	if err := models.UpdateVoicemail(p.db, voicemail); err != nil {
		return fmt.Errorf("更新留言记录失败: %w", err)
	}

	logrus.WithField("voicemail_id", voicemailID).Info("✅ 留言处理完成")
	return nil
}

// uploadAudio 上传音频到存储
func (p *VoicemailProcessor) uploadAudio(ctx context.Context, voicemailID uint, pcmuAudio []byte) (string, error) {
	// 1. 转换 PCMU -> PCM16
	pcm16Audio := codec.PCMUToPCM16(pcmuAudio)

	// 2. 生成存储路径
	timestamp := time.Now().Unix()
	storageKey := fmt.Sprintf("voicemails/%d_%d.wav", voicemailID, timestamp)

	// 3. 上传到 LingStorage
	result, err := config.GlobalStore.UploadBytes(&lingstorage.UploadBytesRequest{
		Bucket:   config.GlobalConfig.Services.Storage.Bucket,
		Data:     pcm16Audio,
		Filename: storageKey,
	})

	if err != nil {
		return "", fmt.Errorf("上传音频失败: %w", err)
	}

	return result.URL, nil
}

// transcribeAudio 语音转文字
func (p *VoicemailProcessor) transcribeAudio(ctx context.Context, pcmuAudio []byte, asrConfig map[string]interface{}) (string, error) {
	// 1. 转换 PCMU 8kHz -> PCM16 16kHz（ASR通常需要16kHz）
	pcm8k := codec.PCMUToPCM16(pcmuAudio)
	pcm16k := codec.ResampleAudio(pcm8k, 8000, 16000)

	// 2. 从配置创建ASR服务
	provider := "qcloud" // 默认使用腾讯云
	if p, ok := asrConfig["provider"].(string); ok && p != "" {
		provider = p
	}

	language := "zh" // 默认中文
	if l, ok := asrConfig["language"].(string); ok && l != "" {
		language = l
	}

	// 创建ASR配置
	asrConfigObj, err := recognizer.NewTranscriberConfigFromMap(provider, asrConfig, language)
	if err != nil {
		return "", fmt.Errorf("创建ASR配置失败: %w", err)
	}

	// 创建ASR服务
	factory := recognizer.NewTranscriberFactory()
	asrService, err := factory.CreateTranscriber(asrConfigObj)
	if err != nil {
		return "", fmt.Errorf("创建ASR服务失败: %w", err)
	}

	// 3. 执行转录
	var transcribedText string
	var asrErr error
	done := make(chan bool, 1)

	asrService.Init(
		func(text string, isLast bool, duration time.Duration, uuid string) {
			if text != "" {
				transcribedText = text
			}
			if isLast || text != "" {
				select {
				case done <- true:
				default:
				}
			}
		},
		func(err error, isFatal bool) {
			asrErr = err
			select {
			case done <- true:
			default:
			}
		},
	)

	// 连接并发送音频
	if err := asrService.ConnAndReceive("voicemail_transcribe"); err != nil {
		return "", fmt.Errorf("ASR连接失败: %w", err)
	}

	if err := asrService.SendAudioBytes(pcm16k); err != nil {
		return "", fmt.Errorf("发送音频失败: %w", err)
	}

	if err := asrService.SendEnd(); err != nil {
		return "", fmt.Errorf("发送结束标记失败: %w", err)
	}

	// 等待转录结果（带超时）
	select {
	case <-done:
		if asrErr != nil {
			return "", fmt.Errorf("ASR转录失败: %w", asrErr)
		}
	case <-time.After(30 * time.Second):
		return "", fmt.Errorf("ASR转录超时")
	case <-ctx.Done():
		return "", fmt.Errorf("转录被取消")
	}

	if transcribedText == "" {
		return "", fmt.Errorf("未识别到内容")
	}

	return transcribedText, nil
}

// generateSummary 生成摘要和关键词
func (p *VoicemailProcessor) generateSummary(ctx context.Context, text string, llmProvider llm.LLMProvider) (string, string, error) {
	// 构建提示词
	prompt := fmt.Sprintf(`请分析以下留言内容，生成简洁的摘要和关键词。

留言内容：
%s

请按以下格式返回（不要包含其他内容）：
摘要：[一句话总结留言的主要内容]
关键词：[提取3-5个关键词，用逗号分隔]`, text)

	// 调用LLM
	response, err := llmProvider.Query(prompt, "")
	if err != nil {
		return "", "", fmt.Errorf("LLM调用失败: %w", err)
	}

	// 解析响应
	summary, keywords := parseSummaryResponse(response)

	return summary, keywords, nil
}

// parseSummaryResponse 解析摘要响应
func parseSummaryResponse(response string) (summary string, keywords string) {
	// 简单的解析逻辑
	lines := splitLines(response)

	for _, line := range lines {
		if len(line) > 3 {
			if line[:3] == "摘要：" || line[:3] == "摘要:" {
				summary = line[3:]
			} else if line[:4] == "关键词：" || line[:4] == "关键词:" {
				keywords = line[4:]
			}
		}
	}

	// 如果没有找到格式化的内容，使用整个响应作为摘要
	if summary == "" {
		summary = response
	}

	return
}

// splitLines 分割行
func splitLines(text string) []string {
	var lines []string
	var currentLine string

	for _, char := range text {
		if char == '\n' || char == '\r' {
			if currentLine != "" {
				lines = append(lines, currentLine)
				currentLine = ""
			}
		} else {
			currentLine += string(char)
		}
	}

	if currentLine != "" {
		lines = append(lines, currentLine)
	}

	return lines
}

// SaveVoicemailAudio 保存留言音频到本地文件
func (p *VoicemailProcessor) SaveVoicemailAudio(voicemailID uint, pcmuAudio []byte) (string, error) {
	// 创建存储目录
	storageDir := filepath.Join(".", "uploads", "voicemails")

	// 转换 PCMU -> PCM16
	_ = codec.PCMUToPCM16(pcmuAudio) // 转换但暂不使用

	// 生成文件名
	timestamp := time.Now().Unix()
	filename := fmt.Sprintf("voicemail_%d_%d.wav", voicemailID, timestamp)
	filepath := filepath.Join(storageDir, filename)

	// 保存文件（这里简化处理，实际应该写入WAV格式）
	// 注意：这里需要添加WAV文件头

	return filepath, nil
}
