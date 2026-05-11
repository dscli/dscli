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
)

// FIM 实现填充中间代码（Fill-In-the-Middle）功能。
//
// 通过 context.StreamKey 控制流式/非流式：
//   - false（默认）：一次性返回完整补全结果
//   - true：通过 SSE 流式输出，实时打印补全内容
//
// 参考 API 文档: https://api.deepseek.com/beta/completions
func (c *Deepseek) FIM(ctx context.Context, req FIMRequest) (*FIMResponse, error) {
	// 默认模型：deepseek-v4-pro（FIM API 唯一支持的模型）
	if req.Model == "" {
		req.Model = context.ContextValue(ctx, context.CurrentModelNameKey, "deepseek-v4-pro")
	}

	// 默认 max_tokens
	if req.MaxTokens <= 0 {
		req.MaxTokens = DefaultMaxTokens
	}

	// 从 context 读取 stream 设置
	if !req.Stream {
		req.Stream = context.ContextValue(ctx, context.StreamKey, false)
	}

	// 流式请求
	if req.Stream {
		return c.fimStream(ctx, req)
	}

	// 非流式请求
	var resp FIMResponse
	err := c.doRequest("POST", "/beta/completions", req, &resp)
	if err != nil {
		return nil, err
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in FIM response")
	}

	return &resp, nil
}

// fimStream 处理 FIM 流式请求（SSE）。
func (c *Deepseek) fimStream(ctx context.Context, req FIMRequest) (*FIMResponse, error) {
	url := c.baseURL + "/beta/completions"

	data, err := outfmt.JSONMarshal(req)
	if err != nil {
		return nil, fmt.Errorf("序列化FIM请求失败: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("创建FIM请求失败: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("FIM网络请求失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("FIM API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 检查 Content-Type
	contentType := resp.Header.Get("Content-Type")
	if !strings.Contains(contentType, "text/event-stream") {
		return nil, fmt.Errorf("非streaming响应，Content-Type: %s", contentType)
	}

	// 处理 SSE 流
	reader := bufio.NewReader(resp.Body)
	var fullText strings.Builder
	var lastChunk *FIMStreamChunk

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("读取FIM streaming响应失败: %w", err)
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 解析 SSE 格式: data: {...}
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		dataStr := line[6:] // 去掉 "data: " 前缀

		if dataStr == "[DONE]" {
			break
		}

		var chunk FIMStreamChunk
		if err := json.Unmarshal([]byte(dataStr), &chunk); err != nil {
			// 忽略解析错误，继续处理下一个数据块
			continue
		}
		lastChunk = &chunk

		// 实时输出文本
		if len(chunk.Choices) > 0 && chunk.Choices[0].Text != "" {
			text := chunk.Choices[0].Text
			fmt.Print(text)
			fullText.WriteString(text)
		}
	}

	// 构建最终响应
	result := &FIMResponse{
		ID: "fim-streaming-" + time.Now().Format("20060102150405"),
		Choices: []FIMChoice{
			{
				Text:  fullText.String(),
				Index: 0,
			},
		},
	}

	// 从最后一个 chunk 获取 metadata
	if lastChunk != nil {
		result.ID = lastChunk.ID
		if lastChunk.Usage != nil {
			result.Usage = *lastChunk.Usage
		}
	}

	return result, nil
}
