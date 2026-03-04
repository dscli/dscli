package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

var ToolDisplayName = &struct{}{}

// toolRegistry 工具注册表
var toolRegistry = map[string]ToolDef{}

func GetToolDisplayName(name string) string {
	words := strings.Split(name, "_")
	for i, word := range words {
		word = strings.ToUpper(word[0:1]) + word[1:]
		words[i] = word
	}
	return strings.Join(words, "")
}

// RegisterTool 注册工具
func RegisterTool(tool ToolDef) {
	tool.DisplayName = GetToolDisplayName(tool.Name)
	toolRegistry[tool.Name] = tool
}

// GetAllTools 获取所有工具定义（用于API调用）
func GetAllTools() []Tool {
	if ModelID == DeepseekReasoner {
		return nil
	}

	var tools []Tool
	for name, def := range toolRegistry {
		tools = append(tools, Tool{
			Type: "function",
			Function: Function{
				Name:        name,
				Description: def.Description,
				Parameters:  def.Parameters,
			},
		})
	}
	return tools
}

// HandleToolCalls 处理工具调用（带统计）
func HandleToolCalls(ctx context.Context, tcs []ToolCall) []Message {
	inputs := []Message{}
	// 处理每个工具调用
	for _, tc := range tcs {
		// 使用新的工具调用处理器
		result, err := HandleToolCall(ctx, tc.Function.Name, []byte(tc.Function.Arguments))
		if err != nil {
			// But we still need to tell the result to assistant
			result = err.Error()
		}

		inputs = append(inputs, Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    result,
		})
	}
	return inputs
}

// HandleToolCall 处理工具调用（带统计和超时）
func HandleToolCall(ctx context.Context, toolName string, argsRaw json.RawMessage) (string, error) {
	// 获取工具处理器
	tool, ok := toolRegistry[toolName]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", toolName)
	}
	args := map[string]string{}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		n := len(argsRaw)
		if n > 80 {
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about raw argument tool %q received`+
				` which lead error:
- the length of the argument string: %d
- the last 40 bytes of the argument string: %q
- the first 40 bytes of the argument string: %q`, err, toolName, n,
				string(argsRaw[n-40:]), string(argsRaw[0:40]))
		} else {
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about the raw argument tool %q received, 
which lead to the error:
- the length of the argument string：%d
- the argument raw：%q`, err, toolName, n, string(argsRaw))
		}
		return "", err
	}

	// 创建带超时的context（如果工具设置了超时）
	var cancel context.CancelFunc
	if tool.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, tool.Timeout)
		defer cancel()
	}

	ctx = context.WithValue(ctx, ToolDisplayName, tool.DisplayName)
	toolID, err := GetOrCreateTool(tool.Name, tool.Description, tool.Category)
	if err != nil {
		Error(err.Error(), "name", tool.Name)
		// 继续执行工具，但不记录统计
		return tool.Handler(ctx, args)
	}

	// 执行工具
	result, err := tool.Handler(ctx, args)

	// 检查是否超时
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("工具执行超时（%v）", tool.Timeout)
	}

	// 记录使用情况
	success := err == nil
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	if err := RecordToolUsage(toolID, success, errorMsg); err != nil {
		return "", err
	}

	return result, err
}
