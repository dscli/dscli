package ask

import (
	_ "embed"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
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
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	},
	Category: "communication",
	Timeout:  5 * time.Minute, // 5分钟超时
	Handler:  handleCodeReview,
}

func init() {
	if context.ReasonerModelOK() {
		toolcall.RegisterTool(codeReviewTool)
	}
}

// handleCodeReview 处理代码审查工具调用
func handleCodeReview(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	// 常量定义：每次审查的提交数量
	const reviewCommitCount = 1

	summary := toolcall.ToolArgsValue(args, "summary", "")
	testCommand := toolcall.ToolArgsValue(args, "test_command", "")

	if summary == "" {
		outfmt.Println("❌ 必须提供提交摘要")
		err = fmt.Errorf("必须提供提交摘要")
		return result, warning, err
	}

	// 检查是否有未提交的更改
	fmt.Println("🔍 检查是否有未提交的更改...")
	// 只检查已修改但未提交的变更，忽略未跟踪文件
	statusScript := `git status --porcelain | grep -E '^(M|A|D|R|C)'`
	status, shellErr := shell.SimpleExecute(ctx, statusScript)
	if shellErr != nil {
		// grep返回非零退出码表示没有匹配，这是正常情况
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
	logScript := `git log --oneline -` + strconv.Itoa(reviewCommitCount)
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
	fullLogScript := `git log --format="%B" -` + strconv.Itoa(reviewCommitCount)
	fullLog, err := shell.SimpleExecute(ctx, fullLogScript)
	if err != nil {
		fullLog = log // 如果失败，使用简短的log
	}

	// 生成patch
	patchScript := `git --no-pager format-patch --stdout -` + strconv.Itoa(reviewCommitCount)
	patch, err := shell.SimpleExecute(ctx, patchScript)
	if err != nil {
		fmt.Println("❌ 生成patch失败")
		err = fmt.Errorf("生成patch失败: %w", err)
		return result, warning, err
	}

	// 构建审查请求
	structuredRequest := buildCodeReviewRequest(summary, fullLog, patch)
	outfmt.Printf("📤 发送代码审查请求...\n%s\n", structuredRequest)
	result, err = AskExpertWithRole(ctx, structuredRequest, "review")
	if err != nil {
		err = fmt.Errorf("代码提交失败: %w", err)
		return result, warning, err
	}

	outfmt.Printf("✅ 代码审查结果\n%s\n", result)
	return result, warning, err
}

func buildCodeReviewRequest(summary, commitLog, patch string) string {
	return `请对以下代码提交进行审查，提供详细的改进建议。

## 提交背景
` + summary + `

## 提交信息
` + commitLog + `

## 代码变更
` + patch + `

## 审查要求
请按以下结构提供审查意见：

### 1. 总体评价
- 代码质量总体评价
- 是否符合最佳实践
- 是否有明显的设计问题

### 2. 具体问题
- 代码风格问题（命名、格式、注释等）
- 逻辑错误或潜在bug
- 性能问题
- 安全问题
- 可维护性问题

### 3. 改进建议
- 具体的修改建议
- 重构建议（如有必要）
- 测试建议

### 4. 总结
- 最重要的几点建议
- 优先级建议（哪些需要立即修改，哪些可以后续优化）

## 注意事项
- 请具体指出问题所在的行号或代码片段
- 提供具体的修改示例
- 考虑代码的可读性、可维护性和性能
- 如果是新功能，考虑其设计合理性

现在请开始您的代码审查：`
}
