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
- content: 要询问的详细内容（必填）
- summary: 问题摘要（可选），用于快速理解问题背景，如不提供会自动生成

使用场景：
1. 技术上有困难时
2. 技术方案需审阅
3. 需要专家深度分析时

注意：程序会自动从专家回答中提取摘要，无需专家手动生成。`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "问题摘要（可选），用于快速理解问题背景",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要询问的详细内容（必填）",
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

// handleAskExpert 处理提问工具调用
func handleAskExpert(ctx context.Context, args ToolArgs) (reply string, err error) {
	// 向后兼容：支持旧参数名
	summary := ToolArgsValue(args, "summary", "")
	content := ToolArgsValue(args, "content", "")

	// 如果content为空，尝试使用旧参数名
	if content == "" {
		content = ToolArgsValue(args, "question", "")
	}

	if content == "" {
		return "", fmt.Errorf("问题内容不能为空")
	}

	// 如果用户没有提供summary，自动从content生成
	if summary == "" {
		summary = generateUserSummary(content)
		Println("📝 自动生成问题摘要:", summary)
	}

	// 输出咨询日志
	Println("📞 正在向专家咨询...")
	Println("  问题摘要:", summary)

	// 构建结构化请求（不再要求专家生成摘要）
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

	// 智能处理专家响应（自动生成摘要）
	processedReply := processExpertResponse(reply)

	Println("✅ 专家咨询完成")

	return processedReply, nil
}

// buildStructuredRequest 构建结构化请求
func buildStructuredRequest(userSummary string, originalContent string) string {
	return `请以结构化格式回答以下问题。

## 问题背景
` + userSummary + `

## 详细问题
` + originalContent + `

## 回答要求
请提供详细的分析和建议，包括：
1. 问题分析：深入分析问题的核心和关键点
2. 解决方案：提供具体可行的解决方案
3. 建议：给出可操作的建议和注意事项
4. 风险评估：指出潜在的风险和应对措施

## 注意事项
- 分析要逻辑严谨，考虑全面
- 建议要具体可行，有优先级
- 风险评估要客观全面

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

// generateUserSummary 从用户内容自动生成摘要
// generateUserSummary 从用户内容自动生成摘要
func generateUserSummary(content string) string {
	// 如果内容很短，直接返回
	if len(content) <= 100 {
		return content
	}

	// 策略1：尝试提取第一段
	paragraphs := strings.Split(content, "\n\n")
	if len(paragraphs) > 0 {
		firstPara := strings.TrimSpace(paragraphs[0])
		if len(firstPara) > 20 && len(firstPara) <= 150 {
			return firstPara
		}
	}

	// 策略2：提取前几个句子
	sentences := extractFirstSentences(content, 2)
	if len(sentences) > 0 {
		summary := strings.Join(sentences, " ")
		if len(summary) <= 150 {
			return summary
		}
	}

	// 策略3：智能截断
	return smartTruncate(content, 100)
}

// extractFirstSentences 提取前n个句子
func extractFirstSentences(content string, n int) []string {
	var sentences []string
	var currentSentence strings.Builder

	for _, r := range content {
		currentSentence.WriteRune(r)

		// 检查句子结束标记
		if isSentenceEnd(r) {
			sentence := strings.TrimSpace(currentSentence.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
				if len(sentences) >= n {
					break
				}
			}
			currentSentence.Reset()
		}
	}

	// 如果最后一个句子不完整但有内容，也添加
	if currentSentence.Len() > 0 {
		sentence := strings.TrimSpace(currentSentence.String())
		if sentence != "" {
			sentences = append(sentences, sentence)
		}
	}

	return sentences
}

// isSentenceEnd 检查是否是句子结束标记
func isSentenceEnd(r rune) bool {
	sentenceEnds := []rune{'.', '。', '!', '！', '?', '？'}
	for _, end := range sentenceEnds {
		if r == end {
			return true
		}
	}
	return false
}

// smartTruncate 智能截断文本
func smartTruncate(content string, maxLength int) string {
	runes := []rune(content)
	if len(runes) <= maxLength {
		return content
	}

	// 确保摘要以完整句子结束
	for i := maxLength - 1; i >= 0; i-- {
		if isSentenceEnd(runes[i]) {
			return string(runes[:i+1])
		}
	}

	// 如果没有找到句子结束标记，查找最后一个空格
	for i := maxLength - 1; i >= 0; i-- {
		if runes[i] == ' ' || runes[i] == '\n' || runes[i] == '\t' {
			return string(runes[:i]) + "..."
		}
	}

	// 如果连空格都没有，直接截断
	return string(runes[:maxLength]) + "..."
}
