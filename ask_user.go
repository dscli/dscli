package main

import (
	"context"
	"fmt"
	"time"
)

// AskTool 工具定义
var askUserTool = ToolDef{
	Name:        "ask_user",
	DisplayName: "问用户",
	Description: `向 user 提问需求，期望用户把需求澄清
期望expert对自己方案审阅，给出建设性意见

参数说明：
- content: 要咨询的内容

使用场景：
需求不明确，资源不到位问用户
`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "要咨询的内容",
			},
		},
		"required": []string{"content"},
	},
	Category: "communication",
	Timeout:  1 * time.Hour, // 给用户一小时回答
	Handler:  handleAskUser,
}

func init() {
	RegisterTool(askUserTool)
}

// handleAskUser 处理提问工具调用
func handleAskUser(ctx context.Context, args ToolArgs) (reply string, err error) {
	content := ToolArgsValue(args, "content", "")
	if content == "" {
		return "", fmt.Errorf("内容不能为空")
	}

	// 输出咨询日志
	Println("📞 正在向用户咨询...")

	// 生成问题摘要（避免过长）
	summary := []rune(content)
	if len(summary) > 100 {
		summary = append(summary[:97], []rune("...")...)
	}
	Println("  问题摘要:", summary)

	reply, err = OpenEditor(content)
	if err != nil {
		Println("❌ 获取用户回答失败")
		return "", fmt.Errorf("获取用户回答失败: %v", err)
	}

	// 显示用户回答摘要
	if reply != "" {
		replySummary := []rune(reply)
		if len(replySummary) > 100 {
			replySummary = append(replySummary[:97], []rune("...")...)
		}
		Println("  用户回答摘要:", replySummary)
	}

	Println("✅ 用户咨询完成")
	return reply, nil
}
