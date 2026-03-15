package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ChatRequest 扩展，支持 tools
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
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
	IsAvailable  bool          `json:"is_available"`
	BalanceInfos []BalanceInfo `json:"balance_infos"`
}

type BalanceInfo struct {
	Currency        string `json:"currency"`
	TotalBalance    string `json:"total_balance"`
	GrantedBalance  string `json:"granted_balance"`
	ToppedUpBalance string `json:"topped_up_balance"`
}

// ChatResponse 响应
type ChatResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Message Message `json:"message"`
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

// ==================== Issue 相关类型 ====================

// RawIssue 用于接收原始JSON数据
type RawIssue struct {
	ID        json.RawMessage `json:"id"`
	Number    string          `json:"number"`
	State     string          `json:"state"`
	Title     string          `json:"title"`
	Body      string          `json:"body"`
	CreatedAt string          `json:"created_at"`
	UpdatedAt string          `json:"updated_at"`
	ClosedAt  string          `json:"closed_at"`
	Labels    []Label         `json:"labels"`
	Assignee  *RawUser        `json:"assignee"`
	User      RawUser         `json:"user"`
}

// RawUser 原始用户数据
type RawUser struct {
	ID        json.RawMessage `json:"id"`
	Login     string          `json:"login"`
	Name      string          `json:"name"`
	AvatarURL string          `json:"avatar_url"`
}

// Label 表示issue的标签
type Label struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	Description string `json:"description"`
}

// Issue 处理后的issue数据结构
type Issue struct {
	ID        int
	Number    string
	State     string
	Title     string
	Body      string
	CreatedAt time.Time
	UpdatedAt time.Time
	ClosedAt  time.Time
	Labels    []Label
	Assignee  *User
	User      User
}

// User 处理后的用户信息
type User struct {
	ID        int
	Login     string
	Name      string
	AvatarURL string
}

// IssueAPIError 表示issue API调用错误
type IssueAPIError struct {
	StatusCode int
	Message    string
	Details    string
}

func (e *IssueAPIError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("issue API错误 (状态码: %d): %s\n详情: %s", e.StatusCode, e.Message, e.Details)
	}
	return fmt.Sprintf("issue API错误 (状态码: %d): %s", e.StatusCode, e.Message)
}

// IssueConfig 包含issue操作的配置信息
type IssueConfig struct {
	BaseURL string
	Token   string
}
