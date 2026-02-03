package hardware

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/code-100-precent/LingEcho/internal/models"
	"github.com/code-100-precent/LingEcho/pkg/llm"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// AnalysisService ai analysis service
type AnalysisService struct {
	db     *gorm.DB
	logger *zap.Logger
}

// NewAnalysisService create ai service
func NewAnalysisService(db *gorm.DB, logger *zap.Logger) *AnalysisService {
	if logger == nil {
		logger = zap.L()
	}
	return &AnalysisService{
		db:     db,
		logger: logger,
	}
}

// AnalysisResult analysis result
type AnalysisResult struct {
	Summary           string   `json:"summary"`           // recording summary
	Keywords          []string `json:"keywords"`          // keywords
	Tags              []string `json:"tags"`              // tags
	Category          string   `json:"category"`          // category
	IsImportant       bool     `json:"isImportant"`       // is important
	SentimentScore    float64  `json:"sentimentScore"`    // SentimentScore (-1-1)
	SatisfactionScore float64  `json:"satisfactionScore"` // Satisfy (0-1)
	ActionItems       []string `json:"actionItems"`       // action items
	Issues            []string `json:"issues"`            // issues
	Insights          string   `json:"insights"`          // insights
}

// AnalyzeCallRecording analysis call recording
func (s *AnalysisService) AnalyzeCallRecording(ctx context.Context, recordingID uint, forceAnalyze bool) error {
	var recording models.CallRecording
	if err := s.db.First(&recording, recordingID).Error; err != nil {
		return fmt.Errorf("获取录音记录失败: %w", err)
	}
	if !forceAnalyze && recording.AnalysisStatus == "completed" {
		s.logger.Info(fmt.Sprintf("%d 录音已分析完成，跳过", recording))
		return nil
	}
	if err := s.updateAnalysisStatus(recordingID, "analyzing", ""); err != nil {
		return fmt.Errorf("更新分析状态失败: %w", err)
	}
	var assistant models.Assistant
	if err := s.db.First(&assistant, recording.AssistantID).Error; err != nil {
		return s.updateAnalysisStatus(recordingID, "failed", "获取助手信息失败")
	}
	if assistant.ApiKey == "" || assistant.ApiSecret == "" {

		return s.updateAnalysisStatus(recordingID, "failed", "助手未配置API凭证")
	}
	provider, err := s.createLLMProvider(&assistant)
	if err != nil {
		return s.updateAnalysisStatus(recordingID, "failed", fmt.Sprintf("创建LLM提供者失败: %v", err))
	}
	defer provider.Hangup()
	result, err := s.performAnalysis(provider, &recording, &assistant)
	if err != nil {
		return s.updateAnalysisStatus(recordingID, "failed", fmt.Sprintf("AI分析失败: %v", err))
	}
	if err := s.saveAnalysisResult(recordingID, result); err != nil {
		return s.updateAnalysisStatus(recordingID, "failed", fmt.Sprintf("保存分析结果失败: %v", err))
	}
	s.logger.Info(fmt.Sprintf("%d 通话录音分析完成，种类: %s 是否重要 %v", recordingID, result.Category, recording.IsImportant))
	return nil
}

// createLLMProvider 创建LLM提供者
func (s *AnalysisService) createLLMProvider(assistant *models.Assistant) (llm.LLMProvider, error) {
	credential, err := models.GetUserCredentialByApiSecretAndApiKey(s.db, assistant.ApiKey, assistant.ApiSecret)
	if err != nil {
		return nil, fmt.Errorf("获取凭证失败: %w", err)
	}
	provider, err := llm.NewLLMProvider(context.Background(), credential, "")
	if err != nil {
		return nil, fmt.Errorf("创建LLM提供者失败: %w", err)
	}
	return provider, nil
}

// performAnalysis 执行AI分析
func (s *AnalysisService) performAnalysis(provider llm.LLMProvider, recording *models.CallRecording, assistant *models.Assistant) (*AnalysisResult, error) {
	prompt := s.buildAnalysisPrompt(recording)
	systemPrompt := s.buildSystemPrompt()
	provider.SetSystemPrompt(systemPrompt)
	options := llm.QueryOptions{
		Model:       assistant.LLMModel,
		MaxTokens:   intPtr(2000),
		Temperature: float32Ptr(0.3),
		Stream:      false,
	}
	response, err := provider.QueryWithOptions(prompt, options)
	if err != nil {
		return nil, fmt.Errorf("LLM查询失败: %w", err)
	}
	result, err := s.parseAnalysisResponse(response)
	if err != nil {
		return nil, fmt.Errorf("解析分析结果失败: %w", err)
	}
	return result, nil
}

// buildSystemPrompt 构建系统提示词
func (s *AnalysisService) buildSystemPrompt() string {
	return `你是一个专业的通话录音分析助手。你的任务是分析用户与AI助手的对话内容，提供深入的洞察和有价值的分析。
请按照以下JSON格式返回分析结果：

{
  "summary": "对话的简洁摘要（50-100字）",
  "keywords": ["关键词1", "关键词2", "关键词3"],
  "tags": ["标签1", "标签2"],
  "category": "对话分类（如：咨询、投诉、闲聊、技术支持、商务洽谈等）",
  "isImportant": true/false,
  "sentimentScore": 0.5,
  "satisfactionScore": 0.8,
  "actionItems": ["需要跟进的行动项"],
  "issues": ["发现的问题点"],
  "insights": "深度洞察分析（100-200字）"
}

分析要点：
1. summary: 提取对话核心内容，突出重点
2. keywords: 提取3-5个最重要的关键词
3. tags: 根据内容打标签（情感、主题、类型等）
4. category: 准确分类对话类型
5. isImportant: 判断是否为重要对话（涉及投诉、重要决策、紧急事项等）
6. sentimentScore: 情感分数，-1（负面）到1（正面）
7. satisfactionScore: 用户满意度，0（不满意）到1（非常满意）
8. actionItems: 需要后续跟进的具体行动
9. issues: 对话中暴露的问题或改进点
10. insights: 深度分析，包括用户需求、AI表现、改进建议等

请确保返回的是有效的JSON格式，不要包含其他文本。`
}

// buildAnalysisPrompt 构建分析提示词
func (s *AnalysisService) buildAnalysisPrompt(recording *models.CallRecording) string {
	var prompt strings.Builder
	prompt.WriteString("请分析以下通话录音内容：\n\n")
	prompt.WriteString(fmt.Sprintf("通话时长: %d秒\n", recording.Duration))
	prompt.WriteString(fmt.Sprintf("通话类型: %s\n", recording.CallType))
	prompt.WriteString(fmt.Sprintf("通话状态: %s\n", recording.CallStatus))
	prompt.WriteString("\n")
	if recording.UserInput != "" {
		prompt.WriteString("用户输入:\n")
		prompt.WriteString(recording.UserInput)
		prompt.WriteString("\n\n")
	}
	if recording.AIResponse != "" {
		prompt.WriteString("AI回复:\n")
		prompt.WriteString(recording.AIResponse)
		prompt.WriteString("\n\n")
	}
	if recording.Summary != "" {
		prompt.WriteString("现有摘要:\n")
		prompt.WriteString(recording.Summary)
		prompt.WriteString("\n\n")
	}
	prompt.WriteString("请基于以上内容进行深度分析，返回JSON格式的分析结果。")
	return prompt.String()
}

// parseAnalysisResponse 解析分析响应
func (s *AnalysisService) parseAnalysisResponse(response string) (*AnalysisResult, error) {
	response = strings.TrimSpace(response)
	start := strings.Index(response, "{")
	end := strings.LastIndex(response, "}")
	if start == -1 || end == -1 || start >= end {
		return nil, fmt.Errorf("响应中未找到有效的JSON格式")
	}
	jsonStr := response[start : end+1]
	var result AnalysisResult
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		return nil, fmt.Errorf("JSON解析失败: %w", err)
	}
	if result.Summary == "" {
		result.Summary = "无法生成摘要"
	}
	if result.Category == "" {
		result.Category = "未分类"
	}
	if len(result.Keywords) == 0 {
		result.Keywords = []string{"通话"}
	}
	return &result, nil
}

// saveAnalysisResult 保存分析结果
func (s *AnalysisService) saveAnalysisResult(recordingID uint, result *AnalysisResult) error {
	analysisJSON, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化分析结果失败: %w", err)
	}
	keywordsJSON, _ := json.Marshal(result.Keywords)
	tagsJSON, _ := json.Marshal(result.Tags)
	now := time.Now()
	updates := map[string]interface{}{
		"ai_analysis":     string(analysisJSON),
		"analysis_status": "completed",
		"analysis_error":  "",
		"analyzed_at":     &now,
		"summary":         result.Summary,
		"keywords":        string(keywordsJSON),
		"tags":            string(tagsJSON),
		"category":        result.Category,
		"is_important":    result.IsImportant,
	}
	return s.db.Model(&models.CallRecording{}).Where("id = ?", recordingID).Updates(updates).Error
}

// updateAnalysisStatus 更新分析状态
func (s *AnalysisService) updateAnalysisStatus(recordingID uint, status, errorMsg string) error {
	updates := map[string]interface{}{
		"analysis_status": status,
		"analysis_error":  errorMsg,
	}

	if status == "analyzing" {
		updates["analyzed_at"] = nil
	} else if status == "completed" {
		now := time.Now()
		updates["analyzed_at"] = &now
	}

	return s.db.Model(&models.CallRecording{}).Where("id = ?", recordingID).Updates(updates).Error
}

// AutoAnalyzeRecording 自动分析录音（在录音创建后调用）
func (s *AnalysisService) AutoAnalyzeRecording(ctx context.Context, recordingID uint) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("自动分析录音发生panic",
					zap.Uint("recordingID", recordingID),
					zap.Any("panic", r),
				)
			}
		}()
		time.Sleep(2 * time.Second)
		if err := s.AnalyzeCallRecording(ctx, recordingID, false); err != nil {
			s.logger.Error("自动分析录音失败",
				zap.Uint("recordingID", recordingID),
				zap.Error(err),
			)
			s.updateAnalysisStatus(recordingID, "failed", fmt.Sprintf("自动分析失败: %v", err))
		} else {
			s.db.Model(&models.CallRecording{}).Where("id = ?", recordingID).Update("auto_analyzed", true)
		}
	}()
}

// BatchAnalyzeRecordings 批量分析录音
func (s *AnalysisService) BatchAnalyzeRecordings(ctx context.Context, userID uint, assistantID *uint, limit int) error {
	query := s.db.Where("user_id = ? AND analysis_status IN (?)", userID, []string{"pending", "failed"})

	if assistantID != nil {
		query = query.Where("assistant_id = ?", *assistantID)
	}

	var recordings []models.CallRecording
	if err := query.Limit(limit).Find(&recordings).Error; err != nil {
		return fmt.Errorf("查询待分析录音失败: %w", err)
	}

	s.logger.Info("开始批量分析录音",
		zap.Int("count", len(recordings)),
		zap.Uint("userID", userID),
	)

	for _, recording := range recordings {
		if err := s.AnalyzeCallRecording(ctx, recording.ID, false); err != nil {
			s.logger.Error("批量分析录音失败",
				zap.Uint("recordingID", recording.ID),
				zap.Error(err),
			)
			continue
		}
		time.Sleep(1 * time.Second)
	}

	return nil
}
