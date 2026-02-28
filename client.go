package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type Deepseek struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

type Client interface {
	Models() (*ModelsResponse, error)
	Balance() (*BalanceResponse, error)
	FIM(model, prompt, suffix string, maxTokens int, temperature float64) (*FIMResponse, error)
	Chat(model string, messages []Message, tools []Tool) (*ChatResponse, error)
}

func NewClient(apiKey, baseURL string) Client {
	return &Deepseek{
		apiKey:     apiKey,
		baseURL:    baseURL,
		httpClient: &http.Client{},
	}
}

func (c *Deepseek) doRequest(method, path string, body any, result any) (err error) {
	url := c.baseURL + path

	var reqBody io.Reader
	var data []byte
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			err = fmt.Errorf("序列化请求失败: %w", err)
			slog.Error(err.Error(), "body", body)
			return
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		err = fmt.Errorf("创建请求失败: %w", err)
		slog.Error(err.Error(), "method", method, "data", string(data))
		return
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		err = fmt.Errorf("请求失败: %w", err)
		slog.Error(err.Error(), "req", req)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		err = fmt.Errorf("读取响应失败: %w", err)
		slog.Error(err.Error(), "data", string(data))
		return
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
		slog.Error(err.Error(), "data", string(data))
		return
	}

	if result != nil {
		if err = json.Unmarshal(respBody, result); err != nil {
			err = fmt.Errorf("解析响应失败: %w", err)
			slog.Error(err.Error(), "respBody", string(respBody))
			return
		}
	}
	return
}

// Models 获取模型列表
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
func (c *Deepseek) Chat(model string, messages []Message, tools []Tool) (*ChatResponse, error) {
	for i, m := range messages {
		if m.ReasoningContent != "" {
			m.ReasoningContent = ""
			messages[i] = m
		}
	}

	req := ChatRequest{
		Model:    model,
		Messages: messages,
		Tools:    tools,
		Stream:   false,
	}
	var resp ChatResponse
	err := c.doRequest("POST", "/chat/completions", req, &resp)
	if err != nil {
		slog.Error(err.Error(), "req", req)
		return nil, err
	}
	return &resp, err
}

// FIM 发送 FIM 补全请求
func (c *Deepseek) FIM(model, prompt, suffix string, maxTokens int, temperature float64) (*FIMResponse, error) {
	req := FIMRequest{
		Model:       model,
		Prompt:      prompt,
		Suffix:      suffix,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	var resp FIMResponse
	err := c.doRequest("POST", "/beta/completions", req, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}
