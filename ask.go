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
期望expert对自己方案审阅，给出建设性意见

参数说明：
- advisor: 要询问的对象，只能为 user 或 expert
  * user - 用户（用于澄清需求、确认细节等）
  * expert - 专家（用于技术咨询、方案审阅等）
  * 注意：reasoner 已弃用，请使用 expert
- content: 要询问的内容

使用场景：
1. 需求不明确时问 user
2. 技术上有困难时问 expert
3. 需要方案审阅时问 expert`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"advisor": map[string]any{
				"type": "string",
				"description": `要询问的对象，只能为 user 或 expert
user - 用户（用于澄清需求、确认细节等）
expert - 专家（用于技术咨询、方案审阅等）
注意：reasoner 已弃用，请使用 expert`,
			},

			"content": map[string]any{
				"type":        "string",
				"description": "要询问的内容",
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
// handleAsk 处理提问工具调用
func handleAsk(ctx context.Context, args map[string]string) (reply string, err error) {
	content := args["content"]
	if content == "" {
		return "", fmt.Errorf("内容不能为空")
	}
	advisor := args["advisor"]

	// 参数标准化和向后兼容处理
	advisorName := "用户"
	switch strings.ToLower(advisor) {
	case "user":
		advisorName = "用户"
	case "expert":
		advisorName = "专家"
	case "reasoner":
		// 向后兼容：reasoner 映射到 expert
		advisorName = "专家"
		Println("⚠️  注意：参数 'reasoner' 已弃用，请使用 'expert'")
	default:
		return "", fmt.Errorf("advisor 只能为 user 或 expert (reasoner 已弃用)")
	}

	// 输出咨询日志
	Println("📞 正在向", advisorName, "咨询...")

	// 生成问题摘要（避免过长）
	summary := content
	if len(summary) > 100 {
		summary = summary[:97] + "..."
	}
	Println("  问题摘要:", summary)

	if advisor == "user" || strings.ToLower(advisor) == "user" {
		reply, err = OpenEditor(content)
		if err != nil {
			Println("❌ 获取用户回答失败")
			return "", fmt.Errorf("获取用户回答失败: %v", err)
		}

		// 显示用户回答摘要
		if reply != "" {
			replySummary := reply
			if len(replySummary) > 100 {
				replySummary = replySummary[:97] + "..."
			}
			Println("  用户回答摘要:", replySummary)
		}

		Println("✅ 用户咨询完成")
	} else {
		// expert 或 reasoner（已映射到 expert）
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

			// 如果摘要太长，截断
			if len(expertSummary) > 150 {
				expertSummary = expertSummary[:147] + "..."
			}

			if expertSummary != "" {
				Println("  专家回答摘要:", expertSummary)
			}
		}

		Println("✅ 专家咨询完成")
	}

	return reply, nil
}
