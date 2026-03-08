package main

import (
	"context"
	"fmt"
	"time"
)

// AskTool 工具定义
var AskTool = ToolDef{
	Name:        "ask",
	DisplayName: "提问",
	Description: "向用户提问，期望用户回答（使用编辑器输入）",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"question": map[string]any{
				"type":        "string",
				"description": "要问用户的问题",
			},
		},
		"required": []string{"question"},
	},
	Category: "interaction",
	Timeout:  300 * time.Second, // 给用户5分钟时间回答
	Handler:  handleAsk,
}

func init() {
	RegisterTool(AskTool)
}

// handleAsk 处理提问工具调用
func handleAsk(ctx context.Context, args map[string]string) (string, error) {
	question := args["question"]
	if question == "" {
		return "", fmt.Errorf("问题不能为空")
	}

	// 显示问题
	fmt.Printf("\n❓ %s\n\n", question)
	fmt.Println("请在编辑器中输入您的回答，保存并退出后继续。")

	// 使用编辑器获取回答
	answer, err := OpenEditor("")
	if err != nil {
		return "", fmt.Errorf("获取用户回答失败: %v", err)
	}

	if len(answer) == 0 {
		return "", fmt.Errorf("用户未提供回答")
	}

	return string(answer), nil
}
