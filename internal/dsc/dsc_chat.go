package dsc

import (
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
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
	maxTokens := 8192
	maxAttempts := 16
	attempts := []toolcall.Message{}
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
			if attempt == 1 {
				return &resp, nil
			} else { // for attempt > 1
				message := choice.Message
				outfmt.PrintContent(ctx, message.ReasoningContent, message.Content)
				attempts = append(attempts, message)
				choice.Message = MergeAttempts(attempts)
				resp.Choices[0] = choice
				return &resp, nil
			}
		}
		// 如果是 length，且还有尝试次数，则增加 maxTokens 继续
		if attempt < maxAttempts {
			message := choice.Message
			outfmt.PrintContent(ctx, message.ReasoningContent, message.Content)
			message.ReasoningContent = ""
			messages = append(messages, message)
			attempts = append(attempts, message)
			tcs := message.ToolCalls
			tail := ""
			for _, tc := range tcs {
				tail = toolcall.TruncateTail(tc.Function.Arguments, 120)
				headtail := toolcall.TruncateHeadTail(tc.Function.Arguments, 120)
				outfmt.Printf("消息(%d)因max tokens截断: %q\n", len(tc.Function.Arguments), headtail)
				messages = append(messages, toolcall.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Content: fmt.Sprintf(`Tool call arguments truncated. Received so far (end shown): %s
Please continue generating the remaining part from where it stopped.
Output only the missing JSON fragment, do not repeat the part you see above.`, tail),
				})
			}
			messages = append(messages, toolcall.Message{
				Role: "user",
				Content: fmt.Sprintf(`Arguments truncated. Received so far (end shown): %s
Continue generating from this point. Output only the missing part, do not repeat.`, tail),
			})
			continue
		}
		// 最后一次尝试仍 length，返回响应（即使被截断）
		// 而不是返回错误，这样用户至少能看到部分响应
		return &resp, nil
	}
	return nil, fmt.Errorf("unexpected loop exit")
}

// MergeAttempts 合并多次 attempt 的 assistant 消息（因截断产生的多个
// 片段），返回一个完整的 assistant 消息，其中 content 按顺序拼接，
// tool_calls 按 ID 合并 arguments。
func MergeAttempts(attempts []toolcall.Message) (result toolcall.Message) {
	// 最终结果
	result = toolcall.Message{
		Role: "assistant",
	}

	// 收集 content 片段
	var contentBuilder strings.Builder

	// 用于合并 tool_calls，key 为 tool_call ID
	toolCallMap := make(map[string]*toolcall.ToolCall)
	var order []string
	for _, msg := range attempts {
		// 合并 content
		if msg.Content != "" {
			contentBuilder.WriteString(msg.Content)
		}

		// 合并 tool_calls
		for _, tc := range msg.ToolCalls {
			if existing, ok := toolCallMap[tc.ID]; ok {
				// 同一个 ID，拼接 arguments
				existing.Function.Arguments = fmt.Sprintf("%s%s", existing.Function.Arguments, tc.Function.Arguments)
				toolCallMap[tc.ID] = existing
			} else {
				// 新 ID，复制一份
				order = append(order, tc.ID)
				clone := toolcall.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: toolcall.ToolCallFunction{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				}
				toolCallMap[tc.ID] = &clone
			}
		}
	}

	// 设置合并后的 content
	if contentBuilder.Len() > 0 {
		content := contentBuilder.String()
		result.Content = content
	}

	// 设置合并后的 tool_calls
	if len(toolCallMap) > 0 {
		result.ToolCalls = make([]toolcall.ToolCall, 0, len(toolCallMap))
		for _, id := range order {
			result.ToolCalls = append(result.ToolCalls, *toolCallMap[id])
		}
	}

	return result
}
