package main

// Tool 定义可调用的工具
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema 对象
}

// ChatRequest 扩展，支持 tools
type ChatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Tools    []Tool    `json:"tools,omitempty"`
	Stream   bool      `json:"stream"`
}

// Message 扩展，支持工具调用（注意：Content 字段不再使用 omitempty）
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`                // 始终输出，即使为空字符串
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`   // 仅当有工具调用时输出
	ToolCallID string     `json:"tool_call_id,omitempty"` // 仅当 role="tool" 时输出
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

// Models 响应
type ModelsResponse struct {
	Object string  `json:"object"`
	Data   []Model `json:"data"`
}

type Model struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	OwnedBy string `json:"owned_by"`
}

// Balance 响应
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

// FIM 请求/响应
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
