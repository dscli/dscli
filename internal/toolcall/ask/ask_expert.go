package ask

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// askExpertTool 工具定义
var askExpertTool = toolcall.ToolDef{
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
	Strict: true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"summary": map[string]any{
				"type":        "string",
				"description": "问题摘要（可选），用于快速理解问题背景",
				"pattern":     toolcall.TitleLikePattern(128),
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要询问的详细内容（必填）",
				"pattern":     toolcall.ContentLikePattern(4096),
			},
		},
		"required":             []string{"content"},
		"additionalProperties": false,
	},
	Category: "communication",
	Timeout:  10 * time.Minute, // 给专家10分钟时间回答
	Handler:  handleAskExpert,
}

func init() {
	if context.ReasonerModelOK() {
		toolcall.RegisterTool(askExpertTool)
	}
}

// handleAskExpert 处理提问工具调用
func handleAskExpert(ctx context.Context, args toolcall.ToolArgs) (reply string, user string, err error) {
	// 向后兼容：支持旧参数名
	summary := toolcall.ToolArgsValue(args, "summary", "")
	content := toolcall.ToolArgsValue(args, "content", "")

	// 如果content为空，尝试使用旧参数名
	if content == "" {
		content = toolcall.ToolArgsValue(args, "question", "")
	}

	if content == "" {
		err = fmt.Errorf("问题内容不能为空")
		return
	}

	// 如果用户没有提供summary，自动从content生成
	if summary == "" {
		summary = generateUserSummary(content)
		outfmt.Println("📝 自动生成问题摘要:", summary)
	}

	// 输出咨询日志
	outfmt.Println("📞 正在向专家咨询...")
	outfmt.Println("  问题摘要:", summary)

	// 构建结构化请求（不再要求专家生成摘要）
	structuredRequest := buildStructuredRequest(summary, content)

	reply, err = AskExpert(ctx, structuredRequest)
	if err != nil {
		outfmt.Println("❌ 专家咨询失败")
		return
	}

	// 智能处理专家响应（自动生成摘要）
	reply = processExpertResponse(reply)

	outfmt.Println("✅ 专家咨询完成")

	return
}

// AskExpert 调用AI专家模型进行咨询并返回回复
//
// 该函数通过执行shell命令调用AI模型来处理输入内容，并将模型回复返回给调用者。
// 函数使用标准输入(stdin)传递输入内容，避免了命令行长度限制。
//
// 参数:
//
//	ctx: 上下文对象，用于传递执行环境配置。函数会设置以下上下文值（将覆盖原有值）:
//	     - ShellName: shell执行器名称，设置为"/usr/bin/env"
//	     - ShellArgs: shell参数，设置为[]string{"bash"}
//	     - ShellStdin: 包含输入内容的io.Reader
//	input: 要发送给AI模型的输入文本，可以是任意长度（受系统内存限制）
//
// 返回值:
//
//	reply: AI模型的回复文本。如果执行失败且没有获得回复，返回空字符串。
//	err: 执行过程中的错误。如果执行成功，返回nil。常见错误包括：
//	     - dscli命令执行失败
//	     - shell命令执行失败
//	     - 上下文配置错误
//
// 功能说明:
//
//	函数通过执行以下命令调用AI模型:
//	     dscli chat --no-color --no-timestamp --model <模型名称>
//	其中模型名称由ModelDeepseekReasoner变量指定，默认为"deepseek-reasoner"。
//
// 注意事项:
//   - 确保dscli命令行工具已正确安装并配置
//   - 函数会覆盖上下文中的ShellName、ShellArgs和ShellStdin值
//   - 输入内容通过标准输入传递，可以包含任意字符，没有EOF标记限制
//
// 示例:
//
//	ctx := context.Background()
//	reply, err := AskExpert(ctx, "请分析这段代码的质量")
//	if err != nil {
//	    log.Printf("咨询失败: %v", err)
//	} else {
//	    fmt.Println(reply)
//	}
//
// 参见:
//   - ShellExec: 执行shell命令的函数
//   - handleAskExpert: 使用此函数的工具处理函数
func AskExpert(ctx context.Context, input string) (reply string, err error) {
	script := fmt.Sprintf(`unset InsideShellExec
dscli chat --no-color --no-timestamp --histsize 0 --model %s`, config.Get("model-deepseek-reasoner", "deepseek-reasoner"))
	ctx = context.WithValue(ctx, context.ShellStdinKey, strings.NewReader(input))
	reply, err = toolcall.ShellExec(ctx, script)
	return
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
		outfmt.Println("  专家回答摘要:", expertSummary)
	}

	// 返回完整的响应
	return cleanResponse
}

// extractExpertSummary 从专家响应中提取摘要
func extractExpertSummary(response string) string {
	// 尝试从专家回答中提取摘要
	// 专家回答通常以"## 摘要"或"**摘要**"开头

	// 查找摘要标记
	summaryMarkers := []string{
		"## 摘要",
		"**摘要**",
		"摘要：",
		"Summary:",
		"## Summary",
		"**Summary**",
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

	// 如果没有找到摘要标记，尝试提取第一段
	for line := range strings.SplitSeq(response, "\n") {
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

	return "专家未提供摘要"
}

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
	return slices.Contains(sentenceEnds, r)
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
