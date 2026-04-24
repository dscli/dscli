package dsc

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// Chat 发送聊天请求
func (c *Deepseek) Chat(ctx context.Context, messages []toolcall.Message, tools []toolcall.Tool) (*ChatResponse, error) {
	// 非工具调用的 assistant 消息，清空 reasoning_content（API 会忽略但保留更安全）
	for i, message := range messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 0 && message.ReasoningContent != "" {
			message.ReasoningContent = ""
			messages[i] = message
		}
	}
	model := context.ContextValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
	insideShellExec := context.ContextValue(ctx, context.InsideShellExecKey, false)
	stream := context.ContextValue(ctx, context.StreamKey, false)

	// 如果是streaming请求，即使InsideShellExec为true也测试streaming逻辑
	if insideShellExec && !stream {
		return &ChatResponse{
			ID: "id",
			Choices: []Choice{
				{
					Message: toolcall.Message{Role: "assistant", Content: "yes, here I heard"},
				},
			},
		}, nil
	}

	// 如果是streaming请求，使用streaming处理
	if stream {
		maxTokens := 8192 * 48 // 384K
		return c.chatStream(ctx, ChatRequest{
			Model:     model,
			Messages:  messages,
			Tools:     tools,
			Stream:    true,
			MaxTokens: maxTokens,
			Thinking: Thinking{
				Type: "enabled",
			},
			ReasoningEffort: "max",
		})
	}

	// 非streaming请求：单次请求（无重试），足够大的 maxTokens 防止截断
	maxTokens := 8192 * 48 // 384K
	req := ChatRequest{
		Model:           model,
		Messages:        messages,
		Tools:           tools,
		MaxTokens:       maxTokens,
		Stream:          false,
		Thinking:        Thinking{Type: "enabled"},
		ReasoningEffort: "max",
	}

	var resp ChatResponse
	err := c.doRequest("POST", "/chat/completions", req, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// 如果返回 length 截断，仍返回部分响应（比报错好）
	return &resp, nil
}
