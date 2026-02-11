package tools

import (
	"context"
	"fmt"
	"sync"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/voiceprint"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// VoiceprintIdentifyTool 声纹识别工具
type VoiceprintIdentifyTool struct {
	db                *gorm.DB
	voiceprintService *voiceprint.Service
	logger            *zap.Logger
	audioBuffer       [][]byte
	audioBufferMutex  sync.Mutex
	assistantID       string
	identifiedSpeaker *IdentifiedSpeaker
	identifiedMutex   sync.RWMutex
}

// IdentifiedSpeaker 已识别的说话人信息
type IdentifiedSpeaker struct {
	SpeakerID   string
	SpeakerName string
	Description string
	Confidence  float64
}

// NewVoiceprintIdentifyTool 创建声纹识别工具
func NewVoiceprintIdentifyTool(
	db *gorm.DB,
	voiceprintService *voiceprint.Service,
	assistantID string,
	logger *zap.Logger,
) *VoiceprintIdentifyTool {
	return &VoiceprintIdentifyTool{
		db:                db,
		voiceprintService: voiceprintService,
		logger:            logger,
		audioBuffer:       make([][]byte, 0),
		assistantID:       assistantID,
	}
}

// AddAudioFrame 添加音频帧到缓冲区
func (t *VoiceprintIdentifyTool) AddAudioFrame(data []byte) {
	t.audioBufferMutex.Lock()
	defer t.audioBufferMutex.Unlock()
	frameCopy := make([]byte, len(data))
	copy(frameCopy, data)
	t.audioBuffer = append(t.audioBuffer, frameCopy)
	maxFrames := 84 // 5000ms / 60ms ≈ 84 帧
	if len(t.audioBuffer) > maxFrames {
		t.audioBuffer = t.audioBuffer[len(t.audioBuffer)-maxFrames:]
	}
}

// GetIdentifiedSpeaker 获取已识别的说话人
func (t *VoiceprintIdentifyTool) GetIdentifiedSpeaker() *IdentifiedSpeaker {
	t.identifiedMutex.RLock()
	defer t.identifiedMutex.RUnlock()
	return t.identifiedSpeaker
}

// Identify 执行声纹识别
func (t *VoiceprintIdentifyTool) Identify(ctx context.Context) (*IdentifiedSpeaker, error) {
	t.audioBufferMutex.Lock()
	if len(t.audioBuffer) == 0 {
		t.audioBufferMutex.Unlock()
		return nil, fmt.Errorf("no audio data to identify")
	}

	// 合并所有音频帧
	totalLen := 0
	for _, frame := range t.audioBuffer {
		totalLen += len(frame)
	}

	audioData := make([]byte, 0, totalLen)
	for _, frame := range t.audioBuffer {
		audioData = append(audioData, frame...)
	}
	t.audioBuffer = t.audioBuffer[:0] // 清空缓冲区
	t.audioBufferMutex.Unlock()

	// 获取该助手下的所有声纹
	var voiceprints []models.Voiceprint
	if err := t.db.Where("assistant_id = ?", t.assistantID).Find(&voiceprints).Error; err != nil {
		return nil, fmt.Errorf("failed to get voiceprints: %w", err)
	}

	if len(voiceprints) == 0 {
		return nil, fmt.Errorf("no voiceprints found for assistant %s", t.assistantID)
	}

	// 构建候选 ID 列表
	candidateIDs := make([]string, len(voiceprints))
	voiceprintMap := make(map[string]*models.Voiceprint)
	for i, vp := range voiceprints {
		candidateIDs[i] = vp.SpeakerID
		voiceprintMap[vp.SpeakerID] = &vp
	}

	// 调用声纹识别服务
	result, err := t.voiceprintService.IdentifyVoiceprint(ctx, &voiceprint.IdentifyRequest{
		CandidateIDs: candidateIDs,
		AssistantID:  t.assistantID,
		AudioData:    audioData,
	})
	if err != nil {
		return nil, fmt.Errorf("voiceprint identification failed: %w", err)
	}

	// 检查是否匹配
	if !result.IsMatch {
		return nil, fmt.Errorf("no matching voiceprint found (score: %.2f)", result.Score)
	}

	// 获取对应的声纹记录
	vp := voiceprintMap[result.SpeakerID]
	if vp == nil {
		return nil, fmt.Errorf("voiceprint record not found for speaker %s", result.SpeakerID)
	}

	// 构建识别结果
	identified := &IdentifiedSpeaker{
		SpeakerID:   result.SpeakerID,
		SpeakerName: vp.SpeakerName,
		Description: vp.Description,
		Confidence:  result.Score,
	}

	// 保存识别结果
	t.identifiedMutex.Lock()
	t.identifiedSpeaker = identified
	t.identifiedMutex.Unlock()

	t.logger.Info("[VoiceprintIdentify] 识别成功",
		zap.String("speaker_id", identified.SpeakerID),
		zap.String("speaker_name", identified.SpeakerName),
		zap.Float64("confidence", identified.Confidence))

	return identified, nil
}

// ClearIdentification 清除识别结果
func (t *VoiceprintIdentifyTool) ClearIdentification() {
	t.identifiedMutex.Lock()
	defer t.identifiedMutex.Unlock()
	t.identifiedSpeaker = nil
}

// GetGreetingMessage 获取问候消息
func (t *VoiceprintIdentifyTool) GetGreetingMessage() string {
	t.identifiedMutex.RLock()
	defer t.identifiedMutex.RUnlock()

	if t.identifiedSpeaker == nil {
		return ""
	}

	// 构建问候消息
	greeting := fmt.Sprintf("听您的声音，您是%s吗？", t.identifiedSpeaker.SpeakerName)
	return greeting
}

// GetUserDescription 获取用户描述（用于 LLM System Prompt）
func (t *VoiceprintIdentifyTool) GetUserDescription() string {
	t.identifiedMutex.RLock()
	defer t.identifiedMutex.RUnlock()

	if t.identifiedSpeaker == nil || t.identifiedSpeaker.Description == "" {
		return ""
	}

	return fmt.Sprintf("关于用户 %s 的信息：%s", t.identifiedSpeaker.SpeakerName, t.identifiedSpeaker.Description)
}
