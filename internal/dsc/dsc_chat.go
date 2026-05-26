package dsc

import (
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// Chat 发送聊天请求
func (c *Deepseek) Chat(ctx context.Context, messages []prompt.Message, tools []toolcall.Tool) (*ChatResponse, error) {
	// 非工具调用的 assistant 消息，清空 reasoning_content（API 会忽略但保留更安全）
	for i, message := range messages {
		if message.Role == "assistant" && len(message.ToolCalls) == 0 && message.ReasoningContent != "" {
			message.ReasoningContent = ""
			messages[i] = message
		}
	}
	model := context.ContextValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
	stream := context.ContextValue(ctx, context.StreamKey, false)

	// 构建请求（stream / non-stream 共用）
	buildReq := func(stream bool) ChatRequest {
		req := ChatRequest{
			Model:     model,
			Messages:  messages,
			Tools:     tools,
			Stream:    stream,
			MaxTokens: DefaultMaxTokens,
			Thinking:  Thinking{Type: "enabled"},
		}
		if V4Enabled {
			req.ReasoningEffort = "max"
		}
		return req
	}

	// 如果是streaming请求，使用streaming处理（带重试）
	if stream {
		var resp *ChatResponse
		var lastErr error
		streamReq := buildReq(true)
		for attempt := 0; attempt <= c.maxRetries; attempt++ {
			if attempt > 0 {
				delay := min(time.Duration(1<<(attempt-1))*c.retryDelay,
					300*time.Second)
				if delay.Seconds() < 1 {
					outfmt.Notice("流中断，立即重试...")
				} else {
					outfmt.Notice("流中断，%d秒后重试...", int(delay.Seconds()))
				}
				time.Sleep(delay)
			}
			resp, lastErr = c.chatStream(ctx, streamReq)
			if lastErr == nil {
				if attempt > 0 {
					outfmt.Notice("重试成功")
				}
				return resp, nil
			}
			if !isRetryableError(lastErr) {
				return nil, lastErr
			}
		}
		return nil, fmt.Errorf("经过%d次重试后仍然失败: %w", c.maxRetries, lastErr)
	}

	// 非streaming请求
	req := buildReq(false)
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
