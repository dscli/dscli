package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// codeReviewTool 代码审查工具定义
var codeReviewTool = ToolDef{
	Name:        "code_review",
	DisplayName: "代码审查",
	Description: `对当前最新的Git提交进行代码审查，由专家提供改进建议。

参数说明：
- summary: 可选，提供本次提交的背景说明，帮助专家理解上下文
           （例如：修复了什么bug、实现了什么功能、为什么这样设计等）
- test_command: 可选，单元测试命令，默认为'go test ./...'。设置为空字符串可跳过测试

使用场景：
1. 提交代码前，让专家review一下
2. 学习更好的编程实践
3. 检查潜在的性能、安全、可维护性问题

审查流程：
1. 检查是否有未提交的更改（如果有则返回错误）
2. 检查是否为单一commit（确保没有多个未push的提交）
3. 运行单元测试（确保代码质量）
4. 获取最新的提交（HEAD）
5. 生成patch格式的代码变更
6. 发送给专家进行审查
7. 返回专家的改进建议

错误处理：
- 如果检测到未提交的更改，工具会立即返回错误
- 如果检测到多个未push的提交，工具会返回错误
- 如果单元测试未通过，工具会返回错误
- 错误信息包含详细的原因和修复建议
- 用户需要先解决所有问题，然后才能使用代码审查工具

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
			"test_command": map[string]any{
				"type":        "string",
				"description": "可选，单元测试命令，默认为'go test ./...'。设置为空字符串可跳过测试",
			},
		},
		"required": []string{},
	},
	Category: "git",
	Timeout:  120 * time.Second, // 2分钟超时
	Handler:  handleCodeReview,
}

func init() {
	RegisterTool(codeReviewTool)
}

// handleCodeReview 处理代码审查工具调用
func handleCodeReview(ctx context.Context, args ToolArgs) (reply string, err error) {
	summary := ToolArgsValue(args, "summary", "")
	testCommand := ToolArgsValue(args, "test_command", "go test ./...")
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

	// 检查是否为单一commit（没有多个未push的提交）
	Println("🔍 检查是否为单一commit...")
	singleCommitScript := `git log --oneline @{u}..HEAD`
	unpushedCommits, err := ShellExec(ctx, singleCommitScript)
	if err != nil {
		err = fmt.Errorf("failed to check single commit: %w", err)
		return
	}

	unpushedCommits = strings.TrimSpace(unpushedCommits)
	if unpushedCommits == "" {
		Println("📝 未发现未push的提交，不必代码审查")
		reply = "No unpushed commits found, no need to do code review"
		return
	}

	lines := strings.Split(unpushedCommits, "\n")
	commitCount := len(lines)

	// 如果有多个未push的提交，建议用户先rebase
	if commitCount > 1 {
		Println("❌ 检测到多个未push的提交")
		return "", fmt.Errorf("more than 1 unpushed commit found")
	}

	Println("✅ 单一commit检查通过")
	if testCommand == "" {
		Println("❌ 无法进行单元测试")
		err = fmt.Errorf("没有指定单元测试命令，无法进行代码审查")
		return
	}

	Println("🔍 运行单元测试:", testCommand)
	testOutput, err := ShellExec(ctx, testCommand)
	if err != nil {
		Println("❌ 单元测试未通过")
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
		return "", fmt.Errorf("%s", errorMsg)
	}
	Println("✅ 单元测试通过")

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

	// 如果用户没有提供summary，从提交信息生成
	if summary == "" {
		summary = generateCodeReviewSummary(log)
		Println("📝 自动生成提交摘要:", summary)
	}

	// 获取完整的提交信息用于构建请求
	fullLogScript := `git log --format="%B" -1`
	fullLog, err := ShellExec(ctx, fullLogScript)
	if err != nil {
		fullLog = log // 如果失败，使用简短的log
	}

	// 生成patch
	patchScript := `git format-patch -1 --stdout`
	patch, err := ShellExec(ctx, patchScript)
	if err != nil {
		Println("❌ 生成patch失败")
		return "", fmt.Errorf("生成patch失败: %v", err)
	}

	// 输出审查日志
	Println("🔍 正在请求专家进行代码审查...")
	// 构建结构化请求
	structuredRequest := buildCodeReviewRequest(summary, fullLog, patch)

	// 确保EOF标记不会出现在内容中
	eof := "EOFFOEOFEEFO"
	for strings.Contains(structuredRequest, eof) {
		eof = Shuffle(eof)
	}

	// 执行审查
	Println("📤 正在发送代码变更给专家...")
	script := fmt.Sprintf(`unset InsideShellExec
dscli chat --no-color --model deepseek-reasoner <<`+eof+`
%s
`+eof, structuredRequest)

	reply, err = ShellExec(ctx, script)
	if err != nil {
		Println("❌ 代码审查失败")
		return "", fmt.Errorf("代码审查失败: %v", err)
	}

	// 处理专家响应（自动提取摘要）
	processedReply := processCodeReviewResponse(reply)

	Println("✅ 代码审查完成")

	return processedReply, nil
}

// generateCodeReviewSummary 从Git提交信息生成摘要
func generateCodeReviewSummary(log string) string {
	// 清理log
	cleanLog := strings.TrimSpace(log)

	// 如果log很短，直接返回
	if len(cleanLog) <= 80 {
		return cleanLog
	}

	// 提取提交ID和提交信息
	parts := strings.SplitN(cleanLog, " ", 2)
	if len(parts) == 2 {
		commitMsg := strings.TrimSpace(parts[1])

		// 如果提交信息很长，截断
		if len(commitMsg) > 80 {
			runes := []rune(commitMsg)
			return string(runes[:77]) + "..."
		}
		return commitMsg
	}

	// 如果格式不符合预期，直接截断
	runes := []rune(cleanLog)
	if len(runes) > 80 {
		return string(runes[:77]) + "..."
	}
	return cleanLog
}

// buildCodeReviewRequest 构建代码审查请求
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
		Println("⚠️  专家回复为空")
		return "专家没有提供审查意见，请检查网络连接或稍后重试。"
	}

	// 提取专家摘要
	expertSummary := extractCodeReviewSummary(cleanResponse)
	if expertSummary != "" {
		Println("  专家审查摘要:", expertSummary)
	}

	return cleanResponse
}

// extractCodeReviewSummary 从代码审查响应中提取摘要
func extractCodeReviewSummary(response string) string {
	// 查找摘要标记
	summaryMarkers := []string{"总体评价", "总结", "摘要", "概要", "核心观点", "Summary", "Overall", "Conclusion", "Key Findings", "Executive Summary", "TL;DR", "要点"}

	// 首先尝试查找"总体评价"部分
	for _, marker := range summaryMarkers {
		// 查找标记（考虑中文和英文）
		patterns := []string{
			"###\\s*" + marker + "\\s*\n+(.+?)(?:\n\n|\n###|\n---|$)",
			"##\\s*" + marker + "\\s*\n+(.+?)(?:\n\n|\n##|\n---|$)",
			marker + "[：:]\n*(.+?)(?:\n\n|\n###|\n---|$)",
		}

		for _, pattern := range patterns {
			re, err := regexp.Compile(pattern)
			if err != nil {
				// 正则表达式编译失败，跳过这个模式
				continue
			}

			matches := re.FindStringSubmatch(response)
			if matches != nil && len(matches) > 1 {
				summary := strings.TrimSpace(matches[1])
				if len(summary) > 10 { // 有效摘要
					// 截断
					runes := []rune(summary)
					if len(runes) > 150 {
						return string(runes[:147]) + "..."
					}
					return summary
				}
			}
		}
	}

	// 如果没有找到结构化摘要，提取第一段
	paragraphs := strings.Split(response, "\n\n")
	for _, para := range paragraphs {
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

	// 最后尝试提取前几行
	lines := strings.Split(response, "\n")
	var summaryLines []string
	for i := 0; i < len(lines) && i < 3; i++ {
		line := strings.TrimSpace(lines[i])
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "###") {
			summaryLines = append(summaryLines, line)
		}
	}

	if len(summaryLines) > 0 {
		summary := strings.Join(summaryLines, " ")
		runes := []rune(summary)
		if len(runes) > 150 {
			return string(runes[:147]) + "..."
		}
		return summary
	}

	return ""
}
