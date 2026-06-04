package dsc

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/price"
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
		streamReq := buildReq(true)
		err := c.retryWithBackoff("流中断", func() error {
			var err error
			resp, err = c.chatStream(ctx, streamReq)
			return err
		})
		if err != nil {
			return nil, err
		}
		return resp, nil
	}

	// 非streaming请求
	req := buildReq(false)
	var resp ChatResponse
	err := c.doRequest("POST", "/chat/completions", req, &resp)
	if err != nil {
		return nil, err
	}

	// 累加 usage，用于后续成本计算
	if resp.Usage != nil {
		price.AddUsage(*resp.Usage)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// 如果返回 length 截断，仍返回部分响应（比报错好）
	return &resp, nil
}
