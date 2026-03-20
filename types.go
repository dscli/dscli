package main

import (
	"database/sql"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
)

// ChatRequest 扩展，支持 tools
type ChatRequest struct {
	Model     string    `json:"model"`
	Messages  []Message `json:"messages"`
	Tools     []Tool    `json:"tools,omitempty"`
	Stream    bool      `json:"stream"`
	MaxTokens int       `json:"max_tokens,omitempty"`
}

// Session 表示一个对话会话
type Session struct {
	ID          int64
	ProjectPath string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Skill 表示一个技能
type Skill struct {
	ID          int64
	Name        string
	Description string
	Content     string
	Category    string
	Priority    int
	IsGlobal    bool
	UsageCount  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ToolDesc 表示一个工具
type ToolDesc struct {
	ID          int64
	Name        string
	Description string
	Category    string
	UsageCount  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ToolUsage 表示工具使用记录
type ToolUsage struct {
	ID          int64
	ProjectPath string
	ToolID      int64
	UsedAt      time.Time
	Success     bool
	ErrorMsg    string
}

type ToolUsageStat struct {
	Name        string
	UsageCount  int
	SuccessRate float64
	LastUsed    time.Time
}

// ProjectSkill 表示项目与技能的关联
type ProjectSkill struct {
	ProjectPath string
	SkillID     int64
	IsEnabled   bool
	EnabledAt   time.Time
	LastUsed    sql.NullTime
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
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
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
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
