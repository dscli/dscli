// Package dsc provides deepseek client
package dsc

import (
	"time"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/price"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// DefaultMaxTokens 默认 max_tokens，可通过 config max-tokens 覆盖
var DefaultMaxTokens = config.GetInt("max-tokens", 8192*48) // 384K

// V4Enabled deepseek-v4 专有参数开关，默认开启。
var V4Enabled = config.GetBool("deepseek-v4", true)

// ChatRequest 扩展，支持 tools
type ChatRequest struct {
	Model           string           `json:"model"`
	Messages        []prompt.Message `json:"messages"`
	Tools           []toolcall.Tool  `json:"tools,omitzero"`
	Stream          bool             `json:"stream"`
	MaxTokens       int              `json:"max_tokens,omitzero"`
	Thinking        Thinking         `json:"thinking"`
	ReasoningEffort string           `json:"reasoning_effort,omitzero"`
}

type Thinking struct {
	Type string `json:"type,omitzero"`
}

type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

type BalanceResponse struct {
	IsAvailable  bool                `json:"is_available"`
	BalanceInfos []map[string]string `json:"balance_infos"`
}

// ChatResponse 响应
type ChatResponse struct {
	ID      string       `json:"id"`
	Choices []Choice     `json:"choices"`
	Usage   *price.Usage `json:"usage"`
}

type Choice struct {
	Message      prompt.Message `json:"message"`
	FinishReason string         `json:"finish_reason"`
}

// FIMRequest FIM (Fill-In-the-Middle) completion request.
// For streaming, set Stream=true or use context.StreamKey.
type FIMRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Suffix      string  `json:"suffix,omitzero"`
	MaxTokens   int     `json:"max_tokens,omitzero"`
	Temperature float64 `json:"temperature,omitzero"`
	TopP        float64 `json:"top_p,omitzero"`
	Stream      bool    `json:"stream,omitzero"`
	Echo        bool    `json:"echo,omitzero"`
	Stop        any     `json:"stop,omitzero"`
}

// FIMResponse non-streaming FIM response.
type FIMResponse struct {
	ID      string      `json:"id"`
	Choices []FIMChoice `json:"choices"`
	Usage   FIMUsage    `json:"usage,omitzero"`
}

type FIMChoice struct {
	Text         string `json:"text"`
	Index        int    `json:"index"`
	FinishReason string `json:"finish_reason,omitzero"`
}

type FIMUsage struct {
	CompletionTokens int `json:"completion_tokens"`
	PromptTokens     int `json:"prompt_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// FIMStreamChunk SSE streaming response chunk for FIM.
type FIMStreamChunk struct {
	ID      string      `json:"id"`
	Object  string      `json:"object"`
	Created int64       `json:"created"`
	Model   string      `json:"model"`
	Choices []FIMChoice `json:"choices"`
	Usage   *FIMUsage   `json:"usage,omitzero"`
}

type Deepseek struct {
	apiKey     string
	baseURL    string
	maxRetries int           // 最大重试次数
	retryDelay time.Duration // 重试延迟（指数退避的初始延迟）
}

type Client interface {
	Models() (*ModelsResponse, error)
	Balance() (*BalanceResponse, error)
	FIM(ctx context.Context, req FIMRequest) (*FIMResponse, error)
	Chat(ctx context.Context, messages []prompt.Message, tools []toolcall.Tool) (*ChatResponse, error)
}

func NewClient(apiKey, baseURL string) Client {
	// 默认重试配置
	maxRetries := 600
	retryDelay := 10 * time.Second

	return &Deepseek{
		apiKey:     apiKey,
		baseURL:    baseURL,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
	}
}

// Models 获取模型列表
func (c *Deepseek) Models() (*ModelsResponse, error) {
	var resp ModelsResponse
	err := c.doRequest("GET", "/models", nil, &resp)
	return &resp, err
}

// Balance 获取余额信息
func (c *Deepseek) Balance() (*BalanceResponse, error) {
	var resp BalanceResponse
	err := c.doRequest("GET", "/user/balance", nil, &resp)
	return &resp, err
}
