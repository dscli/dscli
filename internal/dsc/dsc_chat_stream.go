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

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
)

// chatStream 处理streaming聊天请求
func (c *Deepseek) chatStream(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
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

	// 检查Content-Type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		return nil, fmt.Errorf("非streaming响应，Content-Type: %s", contentType)
	}

	// 处理SSE流
	reader := bufio.NewReader(resp.Body)
	var fullContent strings.Builder

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

		// 解析SSE格式: data: {...}
		if strings.HasPrefix(line, "data: ") {
			dataStr := line[6:] // 去掉"data: "前缀

			if dataStr == "[DONE]" {
				break
			}

			// 解析JSON数据
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}

			if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
				// 忽略解析错误，继续处理下一个数据块
				continue
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
	return &ChatResponse{
		ID: "streaming-response-" + time.Now().Format("20060102150405"),
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
