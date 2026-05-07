package dsc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// StreamChunk represents an incremental piece of a streaming chat response.
type StreamChunk struct {
	Content          string // delta text content (may be empty)
	ReasoningContent string // delta reasoning content (may be empty)
	Done             bool   // true when the stream has ended
}

// ChatStreamChan sends a streaming chat request and writes incremental chunks
// to the provided channel.  It returns the full accumulated ChatResponse
// (suitable for DB persistence) when the stream completes, or an error.
//
// The caller should read from ch in a separate goroutine to avoid blocking
// the HTTP stream.
func (c *Deepseek) ChatStreamChan(ctx context.Context, messages []prompt.Message, tools []toolcall.Tool, ch chan<- StreamChunk) (*ChatResponse, error) {
	model := context.ContextValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
	maxTokens := 8192 * 48 // 384K

	req := ChatRequest{
		Model:           model,
		Messages:        messages,
		Tools:           tools,
		Stream:          true,
		MaxTokens:       maxTokens,
		Thinking:        Thinking{Type: "enabled"},
		ReasoningEffort: "max",
	}

	url := c.baseURL + "/chat/completions"

	data, err := outfmt.JSONMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		return nil, fmt.Errorf("非streaming响应，Content-Type: %s", contentType)
	}

	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	var fullReasoning strings.Builder
	var finishReason string

	// Accumulate tool calls from streaming deltas
	type toolCallAcc struct {
		Index    int
		ID       string
		Type     string
		Name     string
		Arguments strings.Builder
	}
	toolCallsMap := make(map[int]*toolCallAcc)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("读取streaming响应失败: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "data: ") {
			dataStr := line[6:]

			if dataStr == "[DONE]" {
				break
			}

			var chunk struct {
				Choices []struct {
					Delta struct {
						Content          string `json:"content"`
						ReasoningContent string `json:"reasoning_content"`
						ToolCalls []struct {
							Index    int    `json:"index"`
							ID       string `json:"id"`
							Type     string `json:"type"`
							Function struct {
								Name      string `json:"name"`
								Arguments string `json:"arguments"`
							} `json:"function"`
						} `json:"tool_calls"`
					} `json:"delta"`
					FinishReason string `json:"finish_reason"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
				continue
			}

			var deltaContent, deltaReasoning string
			var done bool

			if len(chunk.Choices) > 0 {
				choice := chunk.Choices[0]
				deltaContent = choice.Delta.Content
				deltaReasoning = choice.Delta.ReasoningContent
				if choice.FinishReason != "" {
					finishReason = choice.FinishReason
				}

				// Accumulate tool calls
				for _, tc := range choice.Delta.ToolCalls {
					acc, ok := toolCallsMap[tc.Index]
					if !ok {
						acc = &toolCallAcc{Index: tc.Index}
						toolCallsMap[tc.Index] = acc
					}
					if tc.ID != "" {
						acc.ID = tc.ID
					}
					if tc.Type != "" {
						acc.Type = tc.Type
					}
					if tc.Function.Name != "" {
						acc.Name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						acc.Arguments.WriteString(tc.Function.Arguments)
					}
				}
			}

			if deltaContent != "" {
				fullContent.WriteString(deltaContent)
			}
			if deltaReasoning != "" {
				fullReasoning.WriteString(deltaReasoning)
			}

			// Send incremental chunk to the channel
			if deltaContent != "" || deltaReasoning != "" || done {
				select {
				case ch <- StreamChunk{
					Content:          deltaContent,
					ReasoningContent: deltaReasoning,
					Done:             done,
				}:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
		}
	}

	// Build tool calls from accumulated deltas
	var toolCalls []prompt.ToolCall
	for i := 0; i < len(toolCallsMap); i++ {
		acc, ok := toolCallsMap[i]
		if ok {
			toolCalls = append(toolCalls, prompt.ToolCall{
				ID:   acc.ID,
				Type: acc.Type,
				Function: prompt.ToolCallFunction{
					Name:      acc.Name,
					Arguments: acc.Arguments.String(),
				},
			})
		}
	}

	// Signal stream completion
	select {
	case ch <- StreamChunk{Done: true}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	return &ChatResponse{
		ID:      "streaming-" + fmt.Sprint(len(fullContent.String())),
		Choices: []Choice{
			{
				Message: prompt.Message{
					Role:             "assistant",
					Content:          fullContent.String(),
					ReasoningContent: fullReasoning.String(),
					ToolCalls:        toolCalls,
				},
				FinishReason: finishReason,
			},
		},
	}, nil
}