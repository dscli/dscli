package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"gitcode.com/nanjunjie/dscli/internal/log"
)

type Client struct {
	apiKey  string
	baseURL string
	debug   bool
	http    *http.Client
}

func NewClient(apiKey, baseURL string, debug bool) *Client {
	// 设置日志级别
	log.SetDebugMode(debug)
	
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		debug:   debug,
		http:    &http.Client{},
	}
}

func (c *Client) doRequest(method, path string, body interface{}, result interface{}) error {
	url := c.baseURL + path
	
	// 记录API请求日志
	log.APIRequest(method, url, body)
	
	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			log.Error("序列化请求失败: %v", err)
			return fmt.Errorf("序列化请求失败: %w", err)
		}
		reqBody = bytes.NewReader(data)
		if c.debug {
			fmt.Fprintf(os.Stderr, "请求体: %s\n", string(data))
		}
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		log.Error("创建请求失败: %v", err)
		return fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	
	if c.debug {
		fmt.Fprintf(os.Stderr, "请求: %s %s\n", method, url)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		log.Error("请求失败: %v", err)
		return fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("读取响应失败: %v", err)
		return fmt.Errorf("读取响应失败: %w", err)
	}
	
	// 记录API响应日志
	log.APIResponse(resp.StatusCode, respBody)
	
	if c.debug {
		fmt.Fprintf(os.Stderr, "响应状态: %s\n", resp.Status)
		fmt.Fprintf(os.Stderr, "响应体: %s\n", string(respBody))
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Error("API返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
		return fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(respBody))
	}

	if result != nil {
		if err := json.Unmarshal(respBody, result); err != nil {
			log.Error("解析响应失败: %v", err)
			return fmt.Errorf("解析响应失败: %w", err)
		}
	}
	return nil
}

// Models 获取模型列表
func (c *Client) Models() (*ModelsResponse, error) {
	// 只在DEBUG模式下输出开始日志
	var resp ModelsResponse
	err := c.doRequest("GET", "/models", nil, &resp)
	if err != nil {
		log.Error("获取模型列表失败: %v", err)
	} else {
	}
	return &resp, err
}

// Balance 获取余额
func (c *Client) Balance() (*BalanceResponse, error) {
	// 只在DEBUG模式下输出开始日志
	var resp BalanceResponse
	err := c.doRequest("GET", "/user/balance", nil, &resp)
	if err != nil {
		log.Error("查询余额失败: %v", err)
	} else {
	}
	return &resp, err
}

// Chat 发送聊天请求（无工具）
func (c *Client) Chat(model string, messages []Message) (*ChatResponse, error) {
	log.Debug("开始聊天请求，模型: %s，消息数: %d", model, len(messages))
	log.ChatMessage("用户", messages[len(messages)-1].Content, nil)
	
	req := ChatRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	var resp ChatResponse
	err := c.doRequest("POST", "/chat/completions", req, &resp)
	if err != nil {
		log.Error("聊天请求失败: %v", err)
	} else if len(resp.Choices) > 0 {
		log.ChatMessage("助手", resp.Choices[0].Message.Content, resp.Choices[0].Message.ToolCalls)
		log.Info("聊天请求成功")
	}
	return &resp, err
}

// ChatWithTools 发送聊天请求，支持工具调用
func (c *Client) ChatWithTools(model string, messages []Message, tools []Tool) (*ChatResponse, error) {
	log.Debug("开始聊天请求（支持工具），模型: %s，消息数: %d，工具数: %d", 
		model, len(messages), len(tools))
	
	// 记录最后一条用户消息
	if len(messages) > 0 {
		lastMsg := messages[len(messages)-1]
		if lastMsg.Role == "user" {
			log.ChatMessage("用户", lastMsg.Content, nil)
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
		log.Error("聊天请求失败: %v", err)
	} else if len(resp.Choices) > 0 {
		assistantMsg := resp.Choices[0].Message
		log.ChatMessage("助手", assistantMsg.Content, assistantMsg.ToolCalls)
		
		// 工具调用数量日志由命令文件处理
	}
	return &resp, err
}

// FIM 发送 FIM 补全请求
func (c *Client) FIM(model, prompt, suffix string, maxTokens int, temperature float64) (*FIMResponse, error) {
	log.Debug("开始FIM代码补全请求，模型: %s", model)
	log.Debug("提示: %s", prompt)
	if suffix != "" {
		log.Debug("后缀: %s", suffix)
	}
	
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
		log.Error("FIM请求失败: %v", err)
	} else {
		log.Info("FIM请求成功，生成 %d 个补全结果", len(resp.Choices))
	}
	return &resp, err
}
