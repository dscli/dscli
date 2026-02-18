package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

type Client struct {
	apiKey  string
	baseURL string
	debug   bool
	http    *http.Client
}

func NewClient(apiKey, baseURL string, debug bool) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		debug:   debug,
		http:    &http.Client{},
	}
}

func (c *Client) doRequest(method, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("序列化请求失败: %w", err)
		}
		reqBody = bytes.NewReader(data)
		if c.debug {
			fmt.Fprintf(os.Stderr, "请求体: %s\n", string(data))
		}
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	if c.debug {
		fmt.Fprintf(os.Stderr, "请求: %s %s\n", method, url)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应失败: %w", err)
	}
	if c.debug {
		fmt.Fprintf(os.Stderr, "响应状态: %s\n", resp.Status)
		fmt.Fprintf(os.Stderr, "响应体: %s\n", string(respBody))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}
	return nil
}

// Models 获取模型列表
func (c *Client) Models() (*ModelsResponse, error) {
	var resp ModelsResponse
	err := c.doRequest("GET", "/models", nil, &resp)
	return &resp, err
}

// Balance 获取余额
func (c *Client) Balance() (*BalanceResponse, error) {
	var resp BalanceResponse
	err := c.doRequest("GET", "/user/balance", nil, &resp)
	return &resp, err
}

// Chat 发送聊天请求
func (c *Client) Chat(model string, messages []Message) (*ChatResponse, error) {
	req := ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	var resp ChatResponse
	err := c.doRequest("POST", "/chat/completions", req, &resp)
	return &resp, err
}

// FIM 发送 FIM 补全请求
func (c *Client) FIM(model, prompt, suffix string, maxTokens int, temperature float64) (*FIMResponse, error) {
	req := FIMRequest{
		Model:       model,
		Prompt:      prompt,
		Suffix:      suffix,
		MaxTokens:   maxTokens,
		Temperature: temperature,
	}
	var resp FIMResponse
	err := c.doRequest("POST", "/beta/completions", req, &resp)
	return &resp, err
}