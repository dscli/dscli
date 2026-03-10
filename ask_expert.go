package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// askExpertTool 工具定义
var askExpertTool = ToolDef{
	Name:        "ask_expert",
	DisplayName: "问专家",
	Description: `向专家发问，期望专家审阅方案，解答疑难问题

参数说明：
- content: 要询问的内容

使用场景：
2. 技术上有困难时
3. 技术方案需审阅`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "要询问的内容",
			},
		},
		"required": []string{"content"},
	},
	Category: "communication",
	Timeout:  10 * time.Minute, // 给专家10分钟时间回答
	Handler:  handleAskExpert,
}

func init() {
	RegisterTool(askExpertTool)
}

// handleAsk 处理提问工具调用
func handleAskExpert(ctx context.Context, args ToolArgs) (reply string, err error) {
	content := ToolArgsValue(args, "content", "")
	if content == "" {
		return "", fmt.Errorf("内容不能为空")
	}
	// 输出咨询日志
	Println("📞 正在向专家咨询...")

	// 生成问题摘要（避免过长）
	summary := []rune(content)
	if len(summary) > 100 {
		summary = append(summary[:97], []rune("...")...)
	}
	Println("  问题摘要:", summary)

	// expert 或 reasoner（已映射到 expert）
	eof := "EOFFOEOFEEFO"
	for strings.Contains(content, eof) {
		eof = Shuffle(eof)
	}
	script := fmt.Sprintf(`unset InsideShellExec
dscli chat --no-color --no-timestamp --model deepseek-reasoner <<`+eof+`
%s
`+eof, content)
	ctx = context.WithValue(ctx, ShellName, "/usr/bin/env")
	ctx = context.WithValue(ctx, ShellArgs, []string{"bash"})
	reply, err = ShellExec(ctx, script)
	if err != nil {
		Println("❌ 专家咨询失败")
		return
	}

	// 显示专家回答摘要
	if reply != "" {
		// 清理回复中的多余空白和换行
		cleanReply := strings.TrimSpace(reply)
		// 取前几行作为摘要
		lines := strings.Split(cleanReply, "\n")
		expertSummary := ""
		for i := 0; i < len(lines) && i < 3; i++ {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				if expertSummary != "" {
					expertSummary += " "
				}
				expertSummary += line
			}
		}
		runes := []rune(expertSummary)
		// 如果摘要太长，截断
		if len(runes) > 150 {
			runes = append(runes[:147], []rune("...")...)
		}
		expertSummary = string(runes)
		if expertSummary != "" {
			Println("  专家回答摘要:", expertSummary)
		}
	}

	Println("✅ 专家咨询完成")

	return reply, nil
}
