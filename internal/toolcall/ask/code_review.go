package ask

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/shell"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

var codeReviewTool = toolcall.ToolDef{
	Name:        "code_review",
	DisplayName: "代码审查",
	Description: `对当前最新的Git提交进行代码审查，由专家提供改进建议。

参数说明：
- summary: 必选，提供本次提交的背景说明，关注重点，帮助专家理解上下文，长度1-1024字符
- test_command: 可选，单元测试命令，默认为空，跳过测试，长度1-128字符

使用场景：
1. 提交代码前，让专家review一下
2. 学习更好的编程实践
3. 检查潜在的性能、安全、可维护性问题

审查流程：
1. 检查是否有未提交的更改（如果有则返回错误）
2. 运行单元测试（确保代码质量），test_command为空跳过测试
3. 获取最新的提交（HEAD）
4. 生成该提交的patch格式代码变更
5. 发送给专家进行审查并返回建议

错误处理：
- 如果检测到未提交的更改，工具会立即返回错误
- 如果单元测试设置但未通过，工具会返回错误
- 错误信息包含详细的原因和修复建议
- 用户需要先解决所有问题，然后才能使用代码审查工具

注意：
- 默认只审查最新的1个提交（HEAD）
- 专家会看到该提交的完整代码变更
- 建议在push之前使用此工具，但也可以审查已push的代码
- 确保所有更改都已提交，否则工具会返回错误`,
	Strict: true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "必选，提供本次提交的背景说明，关注重点，帮助专家理解上下文, 长度1-1024字符",
			},
			"test_command": map[string]any{
				"type":        "string",
				"description": "可选，单元测试命令，默认为空，跳过测试, 长度1-128字符",
			},
		},
		"required":             []string{"summary"},
		"additionalProperties": false,
	},
	Category: "git",
	Timeout:  5 * time.Minute, // 5分钟超时
	Handler:  handleCodeReview,
}

func init() {
	if context.ReasonerModelOK() {
		toolcall.RegisterTool(codeReviewTool)
	}
}

// handleCodeReview 处理代码审查工具调用
func handleCodeReview(ctx context.Context, args toolcall.ToolArgs) (reply string, user string, err error) {
	// 常量定义：每次审查的提交数量
	const reviewCommitCount = 1

	summary := toolcall.ToolArgsValue(args, "summary", "")
	testCommand := toolcall.ToolArgsValue(args, "test_command", "")

	if summary == "" {
		outfmt.Println("❌ 必须提供提交摘要")
		err = fmt.Errorf("必须提供提交摘要")
		return
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
		return
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
			return
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
		return
	}

	if strings.TrimSpace(log) == "" {
		outfmt.Println("❌ 没有找到提交记录")
		err = fmt.Errorf("没有找到提交记录，请先提交代码")
		return
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
		return
	}

	// 构建审查请求
	structuredRequest := buildCodeReviewRequest(summary, fullLog, patch)
	outfmt.Printf("📤 发送代码审查请求...\n%s\n", structuredRequest)
	reply, err = AskExpert(ctx, structuredRequest)
	if err != nil {
		outfmt.Println("❌ 代码提交失败")
		err = fmt.Errorf("代码提交失败: %w", err)
		return
	}

	outfmt.Printf("✅ 代码审查结果\n%s\n", reply)
	return
}

func buildCodeReviewRequest(summary string, commitLog string, patch string) string {
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

// processCodeReviewResponse 处理代码审查响应
func processCodeReviewResponse(response string) string {
	// 清理响应
	cleanResponse := strings.TrimSpace(response)

	if cleanResponse == "" {
		fmt.Println("⚠️  专家回复为空")
		return "专家没有提供审查意见，请检查网络连接或稍后重试。"
	}

	// 提取专家摘要
	expertSummary := extractCodeReviewSummary(cleanResponse)
	if expertSummary != "" {
		fmt.Println("  专家审查摘要:", expertSummary)
	}

	return cleanResponse
}

// extractCodeReviewSummary 从代码审查响应中提取摘要
func extractCodeReviewSummary(response string) string {
	// 尝试从代码审查结果中提取摘要
	// 代码审查通常以"## 摘要"或"**摘要**"开头

	// 查找摘要标记
	summaryMarkers := []string{
		"## 摘要",
		"**摘要**",
		"摘要：",
		"Summary:",
		"## Summary",
		"**Summary**",
		"## 审查摘要",
		"**审查摘要**",
	}

	for _, marker := range summaryMarkers {
		idx := strings.Index(response, marker)
		if idx != -1 {
			// 找到摘要标记，提取摘要内容
			summaryStart := idx + len(marker)
			summaryEnd := strings.Index(response[summaryStart:], "\n\n")
			if summaryEnd == -1 {
				summaryEnd = len(response) - summaryStart
			}

			summary := strings.TrimSpace(response[summaryStart : summaryStart+summaryEnd])
			if summary != "" {
				// 如果摘要太长，截断
				runes := []rune(summary)
				if len(runes) > 150 {
					summary = string(runes[:147]) + "..."
				}

				return summary
			}
		}
	}

	// 如果没有找到结构化摘要，提取第一段
	for para := range strings.SplitSeq(response, "\n\n") {
		trimmed := strings.TrimSpace(para)
		if trimmed != "" && len(trimmed) > 20 {
			// 排除太短的段落（可能是标题）
			runes := []rune(trimmed)
			if len(runes) > 150 {
				return string(runes[:147]) + "..."
			}
			return trimmed
		}
	}

	return "代码审查完成"
}