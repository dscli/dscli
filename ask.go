package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// AskTool 工具定义
var AskTool = ToolDef{
	Name:        "ask",
	DisplayName: "询问",
	Description: `向 user 问需求，期望用户把需求澄清
期望reasoner对自己方案审阅，给出建设性意见`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"advisor": map[string]any{
				"type": "string",
				"description": `要询问的对象，只能为 user 或 reasoner
user - 用户
reasoner - deepseek reasoner 大模型
一般需求不明问 user，技术上不会问 reasoner，`,
			},

			"content": map[string]any{
				"type":        "string",
				"description": "要询问的内容，比如",
			},
		},
		"required": []string{"content", "advisor"},
	},
	Category: "interaction",
	Timeout:  300 * time.Second, // 给用户5分钟时间回答
	Handler:  handleAsk,
}

func init() {
	RegisterTool(AskTool)
}

// handleAsk 处理提问工具调用
func handleAsk(ctx context.Context, args map[string]string) (reply string, err error) {
	content := args["content"]
	if content == "" {
		return "", fmt.Errorf("内容不能为空")
	}
	advisor := args["advisor"]
	if advisor != "reasoner" && advisor != "user" {
		return "", fmt.Errorf("只能为user或reasoner")
	}

	if advisor == "user" {
		reply, err = OpenEditor(content)
		if err != nil {
			return "", fmt.Errorf("获取用户回答失败: %v", err)
		}
	} else {
		eof := "EOFFOEOFEEFO"
		for strings.Contains(content, eof) {
			eof = Shuffle(eof)
		}
		script := fmt.Sprintf(`unset InsideShellExec
dscli chat --no-color --model deepseek-reasoner <<`+eof+`
%s
`+eof, content)
		ctx := context.Background()
		ctx = context.WithValue(ctx, ShellName, "/usr/bin/env")
		ctx = context.WithValue(ctx, ShellArgs, []string{"bash"})
		reply, err = ShellExec(ctx, script)
		return
	}

	return reply, nil
}
