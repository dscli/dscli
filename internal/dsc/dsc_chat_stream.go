package dsc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/nanjj/clog"
)

// chatStreamChunk 是 SSE 流中单个 data 块的 JSON 结构（OpenAI 兼容格式）。
// 提取为具名类型以便测试复用；目前仅消费 content 与 id 字段。
type chatStreamChunk struct {
	ID      string `json:"id"`
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// chatStream 处理streaming聊天请求
func (c *Deepseek) chatStream(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "chatStream")
	defer span.Finish()

	url := c.baseURL + "/chat/completions"

	data, err := outfmt.JSONMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
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

	// 检查Content-Type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		return nil, fmt.Errorf("非streaming响应，Content-Type: %s", contentType)
	}

	// 处理SSE流
	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder
	// 流式 chunk 自带响应 ID（与最终 ChatResponse.ID 一致），取第一个非空值，
	// 使流式路径也能获得真实的会话 ID（而非时间戳占位）。
	respID := ""

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

		// 解析SSE格式: data: {...}（"data:" 后可选空白）
		if strings.HasPrefix(line, "data:") {
			dataStr := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if dataStr == "" {
				continue
			}

			if dataStr == "[DONE]" {
				break
			}

			// 解析JSON数据
			var chunk chatStreamChunk
			if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
				outfmt.Debug("跳过无法解析的 SSE chunk: %s (%v)\n", dataStr, err)
				continue
			}
			if respID == "" && chunk.ID != "" {
				respID = chunk.ID
			}

			// 输出内容
			if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
				content := chunk.Choices[0].Delta.Content
				fmt.Print(content)
				fullContent.WriteString(content)
			}
		}
	}

	// 返回一个包含完整内容的响应，用于保存到数据库
	if respID == "" {
		respID = "streaming-response-" + time.Now().Format("20060102150405")
	}
	return &ChatResponse{
		ID: respID,
		Choices: []Choice{
			{
				Message: prompt.Message{
					Role:    "assistant",
					Content: fullContent.String(),
				},
			},
		},
	}, nil
}
