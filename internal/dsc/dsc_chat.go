package dsc

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// Chat 发送聊天请求
func (c *Deepseek) Chat(ctx context.Context, messages []toolcall.Message, tools []toolcall.Tool) (*ChatResponse, error) {
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
		return c.chatStream(ctx, ChatRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools,
			Stream:   true,
		})
	}

	// 非streaming请求
	maxTokens := 4096
	maxAttempts := 2
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req := ChatRequest{
			Model:     model,
			Messages:  messages,
			Tools:     tools,
			MaxTokens: maxTokens,
			Stream:    false,
		}

		var resp ChatResponse
		err := c.doRequest("POST", "/chat/completions", req, &resp)
		if err != nil {
			return nil, err
		}

		if len(resp.Choices) == 0 {
			return nil, fmt.Errorf("no choices in response")
		}

		choice := resp.Choices[0]
		if choice.FinishReason != "length" {
			return &resp, nil
		}
		// 如果是 length，且还有尝试次数，则增加 maxTokens 继续
		if attempt < maxAttempts {
			maxTokens = 8192
			// 注意：此时不应将本次截断的响应加入 messages，所以 messages 保持不变
			continue
		}
		// 最后一次尝试仍 length，返回响应（即使被截断）
		// 而不是返回错误，这样用户至少能看到部分响应
		return &resp, nil
	}
	return nil, fmt.Errorf("unexpected loop exit")
}

