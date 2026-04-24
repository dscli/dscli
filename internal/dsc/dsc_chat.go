package dsc

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
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
		return c.chatStream(ctx, ChatRequest{
			Model:    model,
			Messages: messages,
			Tools:    tools,
			Stream:   true,
			Thinking: Thinking{
				Type: "enabled",
			},
			ReasoningEffort: "max",
		})
	}

	// 非streaming请求
	maxTokens := 8192 * 48 // 384K
	maxAttempts := 1       // max attempts 1 means no retry
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req := ChatRequest{
			Model:     model,
			Messages:  messages,
			Tools:     tools,
			MaxTokens: maxTokens,
			Stream:    false,
			Thinking: Thinking{
				Type: "enabled",
			},
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

		choice := resp.Choices[0]
		if choice.FinishReason != "length" {
			if attempt > 1 {
				outfmt.Printf("%s已完成更正并返回完整响应。\n", model)
			}
			return &resp, nil
		}

		if attempt < maxAttempts {
			message := choice.Message
			outfmt.PrintContent(ctx, message.ReasoningContent, message.Content)
			if len(message.ToolCalls) == 0 {
				message.ReasoningContent = ""
			}
			messages = append(messages, message)
			tcs := message.ToolCalls
			for _, tc := range tcs {
				outfmt.Printf("消息(length=%d)因超过 max_tokens=%d 截断，正通知%s...\n", len(tc.Function.Arguments), maxTokens, model)
				messages = append(messages, toolcall.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content: fmt.Sprintf(`消息因超过 max_tokens=%d 而截断。
请严格遵循 write_file 工具 content 字段限制，分几部分用创建，追加的方式重写文件。
首次创建 append=false 写入不超过8192字符（大约500行文本）内容。
之后 append=true 追加内容不要超过8192字符（大约500行文本）。
尽量不要超过 max_tokens=%d 限制。
超过 max_tokens=%d 内容会被截断，截断的消息只能丢弃。`, maxTokens, maxTokens, maxTokens),
				})
			}
			continue
		}
		// 最后一次尝试仍 length，返回响应（即使被截断）
		// 而不是返回错误，这样用户至少能看到部分响应
		return &resp, nil
	}
	return nil, fmt.Errorf("unexpected loop exit")
}
