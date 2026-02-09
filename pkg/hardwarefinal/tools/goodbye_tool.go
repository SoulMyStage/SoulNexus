package tools

import (
	"fmt"
)

// GoodbyeCallback 待机回调函数类型
type GoodbyeCallback func() error

// RegisterGoodbyeTool 注册待机（goodbye）工具到 LLM
func RegisterGoodbyeTool(service *LLMService, callback GoodbyeCallback) {
	service.RegisterTool(
		"goodbye",
		"当用户表达告别、再见、结束对话等意图时调用此函数。例如：'再见'、'拜拜'、'我要走了'、'结束对话'、'挂断'、'睡觉了'等。调用此函数后，系统会在播放完告别语后自动进入待机状态。",
		map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "告别的原因（可选）",
				},
			},
			"required": []string{},
		},
		func(args map[string]interface{}) (string, error) {
			return executeGoodbye(args, callback)
		},
	)
	
	service.logger.Info("已注册待机（goodbye）工具")
}

// executeGoodbye 执行待机操作
func executeGoodbye(args map[string]interface{}, callback GoodbyeCallback) (string, error) {
	reason, _ := args["reason"].(string)
	
	// 执行回调
	if callback != nil {
		if err := callback(); err != nil {
			return "", fmt.Errorf("设置待机状态失败: %w", err)
		}
	}
	
	// 返回成功消息（这个消息会被 TTS 播放）
	message := "好的，再见"
	if reason != "" {
		message = fmt.Sprintf("好的，%s，再见", reason)
	}
	
	return message, nil
}
