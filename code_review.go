package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CodeReviewTool 代码审查工具定义
var CodeReviewTool = ToolDef{
	Name:        "code_review",
	DisplayName: "代码审查",
	Description: `对当前最新的Git提交进行代码审查，由专家提供改进建议。

参数说明：
- summary: 可选，提供本次提交的背景说明，帮助专家理解上下文
           （例如：修复了什么bug、实现了什么功能、为什么这样设计等）

使用场景：
1. 提交代码前，让专家review一下
2. 学习更好的编程实践
3. 检查潜在的性能、安全、可维护性问题

审查流程：
1. 获取最新的提交（HEAD）
2. 生成patch格式的代码变更
3. 发送给专家进行审查
4. 返回专家的改进建议

注意：
- 只审查最新的一个提交（HEAD）
- 专家会看到完整的代码变更
- 建议在push之前使用此工具`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "可选，提供本次提交的背景说明，帮助专家理解上下文",
			},
		},
		"required": []string{},
	},
	Category: "git",
	Timeout:  120 * time.Second, // 2分钟超时
	Handler:  handleCodeReview,
}

func init() {
	RegisterTool(CodeReviewTool)
}

// handleCodeReview 处理代码审查工具调用
func handleCodeReview(ctx context.Context, args map[string]string) (reply string, err error) {
	summary := args["summary"]

	// 输出审查日志
	Println("🔍 正在请求专家进行代码审查...")

	// 获取Git状态，确保有提交可审查
	statusScript := `git status --short`
	ctx = context.Background()
	ctx = context.WithValue(ctx, ShellName, "/usr/bin/env")
	ctx = context.WithValue(ctx, ShellArgs, []string{"bash"})

	status, err := ShellExec(ctx, statusScript)
	if err != nil {
		Println("❌ 获取Git状态失败")
		return "", fmt.Errorf("获取Git状态失败: %v", err)
	}

	// 检查是否有未提交的更改
	if strings.Contains(status, "Changes not staged for commit") ||
		strings.Contains(status, "Changes to be committed") ||
		(status != "" && !strings.Contains(status, "nothing to commit")) {
		Println("⚠️  检测到未提交的更改，建议先提交再审查")
	}

	// 获取最新的提交信息
	logScript := `git log --oneline -1`
	log, err := ShellExec(ctx, logScript)
	if err != nil {
		Println("❌ 获取提交历史失败")
		return "", fmt.Errorf("获取提交历史失败: %v", err)
	}

	if strings.TrimSpace(log) == "" {
		Println("❌ 没有找到提交记录")
		return "", fmt.Errorf("没有找到提交记录，请先提交代码")
	}

	Println("📝 审查提交:", strings.TrimSpace(log))

	// 构建审查脚本
	eof := "EOFFOEOFEEFO"
	script := `unset InsideShellExec
# 生成patch并发送给专家审查
(
    echo "=== 代码审查请求 ==="
    echo ""
`

	// 如果有summary，添加到脚本中
	if summary != "" {
		script += `    echo "提交背景说明："
    echo "` + strings.ReplaceAll(summary, "\n", "\\n") + `"
    echo ""
`
	}

	script += `    echo "提交信息："
    git log --oneline -1
    echo ""
    echo "=== 代码变更详情 ==="
    git format-patch -1 --stdout
) | dscli chat --no-color --model deepseek-reasoner`

	// 确保EOF标记不会出现在内容中
	for strings.Contains(script, eof) {
		eof = Shuffle(eof)
	}

	// 执行审查
	Println("📤 正在发送代码变更给专家...")
	reply, err = ShellExec(ctx, script)
	if err != nil {
		Println("❌ 代码审查失败")
		return "", fmt.Errorf("代码审查失败: %v", err)
	}

	// 显示专家回答摘要
	if reply != "" {
		// 清理回复中的多余空白和换行
		cleanReply := strings.TrimSpace(reply)
		// 取前几行作为摘要
		lines := strings.Split(cleanReply, "\n")
		expertSummary := ""
		for i := 0; i < len(lines) && i < 5; i++ {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				if expertSummary != "" {
					expertSummary += " "
				}
				expertSummary += line
			}
		}

		// 如果摘要太长，截断
		if len(expertSummary) > 200 {
			expertSummary = expertSummary[:197] + "..."
		}

		if expertSummary != "" {
			Println("  专家审查摘要:", expertSummary)
		}
	}

	Println("✅ 代码审查完成")
	Println("💡 提示：请仔细考虑专家的建议，如有需要可进行修改")

	return reply, nil
}
