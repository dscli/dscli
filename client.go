package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Deepseek struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	maxRetries int           // 最大重试次数
	retryDelay time.Duration // 重试延迟（指数退避的初始延迟）
}

type Client interface {
	Models() (*ModelsResponse, error)
	Balance() (*BalanceResponse, error)
	FIM(ctx context.Context, prompt, suffix string, maxTokens int, temperature float64) (*FIMResponse, error)
	Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error)
}

// httpClient 单例HTTP客户端，避免创建多个连接池
// 超时设置为10分钟，适应DeepSeek API长处理时间的特性
var httpClient = &http.Client{
	Timeout: 600 * time.Second,
}

func NewClient(apiKey, baseURL string) Client {
	// 默认重试配置
	maxRetries := 600
	retryDelay := 10 * time.Second

	return &Deepseek{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: httpClient,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

// isRetryableError 判断错误是否可重试
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 网络连接错误（可重试）
	if strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "dial tcp") ||
		strings.Contains(errStr, "network is unreachable") ||
		strings.Contains(errStr, "no such host") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "i/o timeout") ||
		strings.Contains(errStr, "TLS handshake timeout") ||
		strings.Contains(errStr, "EOF") ||
		strings.Contains(errStr, "connection timed out") {
		return true
	}

	// HTTP状态码错误（部分可重试）
	if strings.Contains(errStr, "API 返回错误状态码") {
		// 5xx错误可重试，4xx错误不可重试
		if strings.Contains(errStr, "500") ||
			strings.Contains(errStr, "502") ||
			strings.Contains(errStr, "503") ||
			strings.Contains(errStr, "504") ||
			strings.Contains(errStr, "429") { // 429 Too Many Requests 也可重试
			return true
		}
	}

	return false
}

// doRequestSingle 单次请求（不带重试）
func (c *Deepseek) doRequestSingle(method, path string, body any, result any) (err error) {
	url := c.baseURL + path

	var reqBody io.Reader
	var data []byte
	if body != nil {
		data, err = JSONMarshal(body)
		if err != nil {
			err = fmt.Errorf("序列化请求失败: %w", err)
			return
		}

		defer DebugBytes("json", data)

		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		err = fmt.Errorf("创建请求失败: %w", err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Since error found
		SetVerbose(true)
		// 检查是否是网络错误
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			err = fmt.Errorf("网络请求超时: %w", err)
		} else {
			err = fmt.Errorf("网络请求失败: %w", err)
		}
		return
	}

	SetVerbose(false)

	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		SetVerbose(true)
		err = fmt.Errorf("读取响应失败: %w", err)
		return
	}
	SetVerbose(false)

	defer DebugBytes("", respBody)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		SetVerbose(true)
		err = fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
		return
	}
	SetVerbose(false)

	if result != nil {
		if err = json.Unmarshal(respBody, result); err != nil {
			err = fmt.Errorf("解析响应失败: %w", err)
			SetVerbose(true)
			return
		}
		SetVerbose(false)
	}
	return
}

// doRequest 请求方法（自动重试）
func (c *Deepseek) doRequest(method, path string, body any, result any) (err error) {
	defer StartWaiting(time.Second * 3)()
	var lastErr error
	attempt := 0
	for attempt = 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			// 计算重试延迟（指数退避）
			delay := min(time.Duration(1<<(attempt-1))*c.retryDelay,
				300*time.Second)
			// 简洁通知用户（不超过20字）
			Notice("网络异常，%d秒后重试...", int(delay.Seconds()))
			time.Sleep(delay)
		}

		err = c.doRequestSingle(method, path, body, result)
		lastErr = err

		if lastErr == nil {
			// 成功，返回
			if attempt > 0 {
				Success("重试成功！")
			}
			return nil
		}

		// 检查错误是否可重试
		if !isRetryableError(lastErr) || attempt == c.maxRetries {
			// 不可重试错误或已达到最大重试次数
			break
		}

		// 可重试错误，继续循环
	}

	// 所有重试都失败
	if attempt > 0 {
		return fmt.Errorf("经过%d次重试后仍然失败，最后错误: %w", attempt, lastErr)
	}
	return lastErr
}

func (c *Deepseek) Models() (*ModelsResponse, error) {
	var resp ModelsResponse
	err := c.doRequest("GET", "/models", nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, err
}

// Balance 获取余额
func (c *Deepseek) Balance() (*BalanceResponse, error) {
	var resp BalanceResponse
	err := c.doRequest("GET", "/user/balance", nil, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, err
}

// Chat 发送聊天请求
func (c *Deepseek) Chat(ctx context.Context, messages []Message, tools []Tool) (*ChatResponse, error) {
	model := ContextValue(ctx, CurrentModelName, ModelDeepseekChat)
	insideShellExec := ContextValue(ctx, InsideShellExec, false)
	stream := ContextValue(ctx, StreamKey, false)

	// 如果是streaming请求，即使InsideShellExec为true也测试streaming逻辑
	if insideShellExec && !stream {
		return &ChatResponse{
			ID: "id",
			Choices: []Choice{
				{
					Message: Message{Role: "assistant", Content: "yes, here I heard"},
				},
			},
		}, nil
	}

	// reset reasoning
	req := ChatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   stream,
	}

	// 如果是streaming请求，使用不同的处理方式
	if stream {
		return c.chatStream(ctx, req)
	}

	var resp ChatResponse
	err := c.doRequest("POST", "/chat/completions", req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, err
}

// chatStream 处理streaming聊天请求
func (c *Deepseek) chatStream(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	url := c.baseURL + "/chat/completions"

	data, err := JSONMarshal(req)
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

	resp, err := c.httpClient.Do(httpReq)
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
				Message: Message{
					Role:    "assistant",
					Content: fullContent.String(),
				},
			},
		},
	}, nil
}

// FIM 实现填充中间代码功能
func (c *Deepseek) FIM(ctx context.Context, prompt, suffix string, maxTokens int, temperature float64) (*FIMResponse, error) {
	// TODO: 实现FIM功能
	return nil, fmt.Errorf("FIM功能暂未实现")
}
