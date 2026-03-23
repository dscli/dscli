// Package dsc provides deepseek client
package dsc

import (
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// ChatRequest 扩展，支持 tools
type ChatRequest struct {
	Model     string             `json:"model"`
	Messages  []toolcall.Message `json:"messages"`
	Tools     []toolcall.Tool    `json:"tools,omitempty"`
	Stream    bool               `json:"stream"`
	MaxTokens int                `json:"max_tokens,omitempty"`
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
	IsAvailable  bool                  `json:"is_available"`
	BalanceInfos []context.BalanceInfo `json:"balance_infos"`
}

// ChatResponse 响应
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message      toolcall.Message `json:"message"`
	FinishReason string           `json:"finish_reason"`
}

type FIMRequest struct {
	Model       string  `json:"model"`
	Prompt      string  `json:"prompt"`
	Suffix      string  `json:"suffix,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type FIMResponse struct {
	ID      string      `json:"id"`
	Choices []FIMChoice `json:"choices"`
}

type FIMChoice struct {
	Text string `json:"text"`
}
