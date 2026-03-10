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
1. 检查是否有未提交的更改（如果有则返回错误）
2. 获取最新的提交（HEAD）
3. 生成patch格式的代码变更
4. 发送给专家进行审查
5. 返回专家的改进建议

错误处理：
- 如果检测到未提交的更改，工具会立即返回错误
- 错误信息包含详细的Git状态，帮助用户了解需要提交的内容
- 用户需要先提交所有更改，然后才能使用代码审查工具

注意：
- 只审查最新的一个提交（HEAD）
- 专家会看到完整的代码变更
- 建议在push之前使用此工具
- 确保所有更改都已提交，否则工具会返回错误`,
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
		Println("❌ 检测到未提交的更改")
		return "", fmt.Errorf("检测到未提交的更改，请先提交所有更改再审查。当前状态：\n%s", status)
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

	// 调试：显示原始回复长度和内容
	Println(fmt.Sprintf("📊 专家回复长度: %d 字符", len(reply)))
	if len(reply) > 0 && len(reply) < 1000 {
		Println(fmt.Sprintf("📄 专家回复预览: %q", reply))
	}

	// 显示专家回答摘要
	if reply != "" {
		// 清理回复中的多余空白和换行
		cleanReply := strings.TrimSpace(reply)
		Println(fmt.Sprintf("📊 清理后回复长度: %d 字符", len(cleanReply)))

		// 取前几行作为摘要
		lines := strings.Split(cleanReply, "\n")
		Println(fmt.Sprintf("📊 回复行数: %d", len(lines)))

		expertSummary := ""
		nonEmptyLines := 0
		for i := 0; i < len(lines) && i < 5; i++ {
			line := strings.TrimSpace(lines[i])
			if line != "" {
				nonEmptyLines++
				if expertSummary != "" {
					expertSummary += " "
				}
				expertSummary += line
			}
		}

		Println(fmt.Sprintf("📊 非空行数: %d", nonEmptyLines))
		Println(fmt.Sprintf("📊 摘要长度: %d 字符", len(expertSummary)))

		// 如果摘要太长，截断
		if len(expertSummary) > 200 {
			expertSummary = expertSummary[:197] + "..."
		}

		if expertSummary != "" {
			Println("  专家审查摘要:", expertSummary)
		} else {
			Println("  专家审查摘要: [空] - 专家回复可能只包含空白字符或特殊格式")
		}
	} else {
		Println("  专家审查摘要: [空] - 专家回复为空")
	}

	Println("✅ 代码审查完成")
	Println("💡 提示：请仔细考虑专家的建议，如有需要可进行修改")

	return reply, nil
}
