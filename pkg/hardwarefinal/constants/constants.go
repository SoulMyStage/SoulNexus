package constants

const (
	COMPONENT_OPUS_DECODE = "opus_decode"
	COMPONENT_ECHO_FILTER = "echo_filter"
	COMPONENT_FILTER_WORD = "filter_word"
	COMPONENT_VAD         = "vad"
)

const BUILD_SYSTEMPROMPT = `
## 重要规则
1. **禁止使用 Markdown 格式**：不要使用任何 Markdown 语法（如 **加粗**、*斜体*、# 标题、- 列表、代码块 + 等）
2. **禁止使用 Emoji 表情**：不要在回复中使用任何 emoji 符号（如 😊、👍、🎉 等）
3. **输出纯文本**：只输出纯文本内容，适合语音播报
4. **口语化表达**：使用自然、口语化的表达方式，避免书面语
5. **避免特殊符号**：不要使用星号、井号、下划线等特殊符号`
