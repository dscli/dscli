package ask

import (
	"context"
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/shell"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed code_review.md
var code_review_md string

var codeReviewTool = toolcall.ToolDef{
	Name:        "code_review",
	DisplayName: "Code Review",
	Description: code_review_md,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "Required, background and focus of this commit, 1-1024 chars",
			},
			"test_command": map[string]any{
				"type":        "string",
				"description": "Optional test command, default empty skips tests, 1-128 chars",
			},
			"since": map[string]any{
				"type":        "string",
				"description": "Number of commits to review, e.g. '-1' (last), '-2' (last 2), default '-1'",
			},
			"timeout": map[string]any{
				"type":        "integer",
				"description": "Timeout in seconds (default 300). Set longer (e.g. 600) for large projects with many tests.",
			},
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	},
	Category: "communication",
	Timeout:  5 * time.Minute, // 5分钟超时
	Handler:  handleCodeReview,
}

func init() {
	// WebChat is always available (free DeepSeek V4 Pro) — no API key needed.
	toolcall.RegisterTool(codeReviewTool)
}

// handleCodeReview 处理代码审查工具调用
func handleCodeReview(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	summary := toolcall.ToolArgsValue(args, "summary", "")
	testCommand := toolcall.ToolArgsValue(args, "test_command", "")
	since := toolcall.ToolArgsValue(args, "since", "-1")

	if summary == "" {
		outfmt.Println("❌ 必须提供提交摘要")
		err = fmt.Errorf("必须提供提交摘要")
		return result, warning, err
	}

	// 校验 since 格式并提取 N
	n, err := parseSince(since)
	if err != nil {
		outfmt.Printf("❌ since 参数格式错误: %v\n", err)
		return result, warning, err
	}

	// 检查是否有未提交的更改（staged + unstaged，忽略 untracked）
	fmt.Println("🔍 检查是否有未提交的更改...")
	statusScript := `git status --porcelain | grep -v '^??'`
	status, shellErr := shell.SimpleExecute(ctx, statusScript)
	if shellErr != nil {
		// grep 返回非零退出码表示没有匹配，这是正常情况
		status = ""
	}

	if status != "" {
		outfmt.Println("❌ 检测到未提交的更改")
		outfmt.Println("当前状态：")
		outfmt.Println(status)
		err = fmt.Errorf("请使用 'git status' 查看详情，并使用 'git add' 和 'git commit' 提交所有更改后再进行审查")
		return result, warning, err
	}

	outfmt.Println("✅ 没有未提交的更改")

	if testCommand != "" {
		outfmt.Println("🔍 运行单元测试:", testCommand)
		testOutput := ""
		testOutput, err = shell.SimpleExecute(ctx, testCommand)
		if err != nil {
			outfmt.Println("❌ 单元测试未通过")
			errorMsg := fmt.Sprintf("单元测试未通过，请修复测试后再审查。\n测试命令：%s\n", testCommand)
			if testOutput != "" {
				// 截断过长的输出
				outputLines := strings.Split(testOutput, "\n")
				if len(outputLines) > 20 {
					errorMsg += "测试输出（前20行）：\n" + strings.Join(outputLines[:20], "\n")
					errorMsg += fmt.Sprintf("\n... 还有%d行输出", len(outputLines)-20)
				} else {
					errorMsg += "测试输出：\n" + testOutput
				}
			}
			outfmt.Println("❌ 单元测试失败")
			err = fmt.Errorf("%s: %w", errorMsg, err)
			return result, warning, err
		}
		if testOutput != "" {
			outfmt.Println(testOutput)
		}
		outfmt.Println("✅ 单元测试通过")
	}

	// 获取最新的提交信息
	logScript := fmt.Sprintf(`git log --oneline %s`, since)
	log, err := shell.SimpleExecute(ctx, logScript)
	if err != nil {
		outfmt.Println("❌ 获取提交历史失败")
		err = fmt.Errorf("获取提交历史失败: %w", err)
		return result, warning, err
	}

	if strings.TrimSpace(log) == "" {
		outfmt.Println("❌ 没有找到提交记录")
		err = fmt.Errorf("没有找到提交记录，请先提交代码")
		return result, warning, err
	}

	outfmt.Println("📝 提交信息:")
	outfmt.Println(log)

	// 获取完整的提交信息用于构建请求
	fullLogScript := fmt.Sprintf(`git log --format="%%B" %s`, since)
	fullLog, err := shell.SimpleExecute(ctx, fullLogScript)
	if err != nil {
		fullLog = log // 如果失败，使用简短的log
	}

	// 生成patch
	patchScript := fmt.Sprintf(`git --no-pager format-patch --stdout %s`, since)
	patch, err := shell.SimpleExecute(ctx, patchScript)
	if err != nil {
		fmt.Println("❌ 生成patch失败")
		err = fmt.Errorf("生成patch失败: %w", err)
		return result, warning, err
	}

	// 获取修改文件的全文
	filesContent := collectFileContents(ctx, n)

	// 构建审查请求
	structuredRequest := buildCodeReviewRequest(summary, fullLog, patch, filesContent)
	outfmt.Printf("📤 发送代码审查请求到 DeepSeek Web（免费 V4 Pro）...\n%s\n", structuredRequest)
	result, err = AskExpertWithRole(ctx, structuredRequest, "review")
	if err != nil {
		err = fmt.Errorf("代码提交失败: %w", err)
		return result, warning, err
	}

	outfmt.Printf("✅ 代码审查结果\n%s\n", result)
	return result, warning, err
}

// parseSince 解析 since 参数，返回提交数 N。
// since 必须为 "-N" 格式（如 "-1", "-2", "-3"）。
func parseSince(since string) (int, error) {
	if !strings.HasPrefix(since, "-") {
		return 0, fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	n, err := strconv.Atoi(since[1:])
	if err != nil || n < 1 {
		return 0, fmt.Errorf("格式必须为 '-N'（如 '-1', '-2', '-3'），当前值: %q", since)
	}
	return n, nil
}

// collectFileContents 收集最近 n 个提交中修改文件的全文内容。
func collectFileContents(ctx context.Context, n int) string {
	const maxFileSize = 500 * 1024 // 500KB per file

	filesScript := fmt.Sprintf(`git diff --name-only HEAD~%d HEAD`, n)
	output, err := shell.SimpleExecute(ctx, filesScript)
	if err != nil || strings.TrimSpace(output) == "" {
		return ""
	}

	files := strings.Split(strings.TrimSpace(output), "\n")
	var sb strings.Builder
	for _, file := range files {
		if file == "" {
			continue
		}
		// 使用 git show 从 object store 读取，不依赖 CWD，
		// 且始终读取 HEAD 版本（避免工作区未保存更改的干扰）。
		content, err := shell.SimpleExecute(ctx, fmt.Sprintf("git show HEAD:%s", file))
		if err != nil {
			fmt.Fprintf(&sb, "\n## File: %s\n[无法读取文件: %v]\n", file, err)
		} else if len(content) > maxFileSize {
			fmt.Fprintf(&sb, "\n## File: %s\n[文件过大 (%d bytes)，已跳过]\n", file, len(content))
		} else {
			fmt.Fprintf(&sb, "\n## File: %s\n```\n%s\n```\n", file, content)
		}
	}
	return sb.String()
}

func buildCodeReviewRequest(summary, commitLog, patch, fileContents string) string {
	req := `## Commit Background
` + summary + `

## Commit Message
` + commitLog + `

## Code Changes
` + patch

	if fileContents != "" {
		req += `

## Full File Contents
` + fileContents
	}

	return req
}
