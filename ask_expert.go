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
- summary: 问题摘要（必填），用于快速理解问题背景
- content: 要询问的详细内容（必填）

使用场景：
1. 技术上有困难时
2. 技术方案需审阅
3. 需要专家深度分析时

注意：专家会生成自己的摘要，放在回答的开头，格式为"摘要："`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "问题摘要，用于快速理解问题背景",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要询问的详细内容",
			},
		},
		"required": []string{"summary", "content"},
	},
	Category: "communication",
	Timeout:  10 * time.Minute, // 给专家10分钟时间回答
	Handler:  handleAskExpert,
}

func init() {
	RegisterTool(askExpertTool)
}

// handleAsk 处理提问工具调用
// handleAskExpert 处理提问工具调用
// handleAskExpert 处理提问工具调用
func handleAskExpert(ctx context.Context, args ToolArgs) (reply string, err error) {
	summary := ToolArgsValue(args, "summary", "")
	content := ToolArgsValue(args, "content", "")

	if summary == "" {
		return "", fmt.Errorf("问题摘要不能为空")
	}
	if content == "" {
		return "", fmt.Errorf("问题内容不能为空")
	}

	// 输出咨询日志
	Println("📞 正在向专家咨询...")
	Println("  问题摘要:", summary)

	// 构建结构化请求（告诉专家要生成摘要）
	structuredRequest := buildStructuredRequest(summary, content)

	// expert 或 reasoner（已映射到 expert）
	eof := "EOFFOEOFEEFO"
	for strings.Contains(structuredRequest, eof) {
		eof = Shuffle(eof)
	}
	script := fmt.Sprintf(`unset InsideShellExec
dscli chat --no-color --no-timestamp --model deepseek-reasoner <<`+eof+`
%s
`+eof, structuredRequest)
	ctx = context.WithValue(ctx, ShellName, "/usr/bin/env")
	ctx = context.WithValue(ctx, ShellArgs, []string{"bash"})
	reply, err = ShellExec(ctx, script)
	if err != nil {
		Println("❌ 专家咨询失败")
		return
	}

	// 处理专家响应
	processedReply := processExpertResponse(reply)

	Println("✅ 专家咨询完成")

	return processedReply, nil
}

// buildStructuredRequest 构建结构化请求
func buildStructuredRequest(userSummary string, originalContent string) string {
	return `请以结构化格式回答以下问题。

## 问题背景
用户提供的摘要：` + userSummary + `

## 详细问题
` + originalContent + `

## 回答要求
请按以下格式组织您的回答：

1. 首先，生成一个简洁的摘要（1-3句话），格式为：
   摘要：[您的摘要内容]

2. 然后，提供详细的分析和建议。

3. 最后，提供置信度评分，格式为：
   置信度：[0.0-1.0的评分]

## 注意事项
- 摘要要准确反映核心观点
- 分析要逻辑严谨，考虑全面
- 建议要具体可行
- 置信度要真实反映您的把握程度

现在请开始您的回答：`
}

// processExpertResponse 处理专家响应
func processExpertResponse(response string) string {
	// 清理响应
	cleanResponse := strings.TrimSpace(response)

	// 提取专家生成的摘要
	expertSummary := extractExpertSummary(cleanResponse)
	if expertSummary != "" {
		Println("  专家回答摘要:", expertSummary)
	}

	// 返回完整的响应
	return cleanResponse
}

// extractExpertSummary 从专家响应中提取摘要
func extractExpertSummary(response string) string {
	// 查找摘要标记
	summaryMarkers := []string{"摘要：", "摘要:", "summary:", "Summary:"}

	for _, marker := range summaryMarkers {
		if idx := strings.Index(response, marker); idx != -1 {
			// 提取摘要内容（到换行或句号为止）
			summaryStart := idx + len(marker)
			summaryText := response[summaryStart:]

			// 查找摘要结束位置
			endMarkers := []string{"\n\n", "\n分析", "\n建议", "\n置信度", "\n1.", "\n2.", "\n3."}
			var endPos int = len(summaryText)

			for _, endMarker := range endMarkers {
				if pos := strings.Index(summaryText, endMarker); pos != -1 && pos < endPos {
					endPos = pos
				}
			}

			// 提取摘要
			summary := strings.TrimSpace(summaryText[:endPos])

			// 如果摘要太长，截断
			runes := []rune(summary)
			if len(runes) > 150 {
				summary = string(runes[:147]) + "..."
			}

			return summary
		}
	}

	// 如果没有找到摘要标记，尝试提取第一段
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			// 取第一段非空行作为摘要
			runes := []rune(trimmed)
			if len(runes) > 150 {
				trimmed = string(runes[:147]) + "..."
			}
			return trimmed
		}
	}

	return ""
}
