package ask

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/shell"
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
			},
			"content": map[string]any{
				"type":        "string",
				"description": "要询问的详细内容（必填）",
			},
			"attachments": map[string]any{
				"type":        "array",
				"description": `作为附件的文件名列表`,
				"items": map[string]string{
					"type":        "string",
					"description": "作为附件的文件名",
				},
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
	attachments := toolcall.ToolArgsValue(args, "attachments", []string{})

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
	structuredRequest, attachmentErrors := buildStructuredRequest(summary, content, attachments)

	// 如果有附件错误，向用户报告但继续执行
	if len(attachmentErrors) > 0 {
		outfmt.Println("⚠️  附件处理警告:")
		for _, attachmentErr := range attachmentErrors {
			outfmt.Printf("  - %v\n", attachmentErr)
		}
	}

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
// 该函数通过 internal/shell 包执行 dscli chat 命令，将输入内容写入临时文件，
// 通过 --input 参数传递给 dscli，避免了命令行长度限制和 stdin 传递问题。
//
// 参数:
//
//	ctx: 上下文对象，用于传递执行环境配置
//	input: 要发送给AI模型的输入文本，可以是任意长度（受系统内存限制）
//
// 返回值:
//
//	reply: AI模型的回复文本。如果执行失败且没有获得回复，返回空字符串。
//	err: 执行过程中的错误。如果执行成功，返回nil。常见错误包括：
//	     - dscli命令执行失败
//	     - 临时文件创建/写入失败
//
// 功能说明:
//
//	函数通过执行以下命令调用AI模型:
//	     dscli chat --no-color --no-timestamp --histsize 0 --model <模型名称> --input <临时文件>
//	其中模型名称由ModelDeepseekReasoner变量指定。
//
// 注意事项:
//   - 确保dscli命令行工具已正确安装并配置
//   - 输入内容通过临时文件传递，执行后自动清理
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
//   - shell.SimpleExecute: 执行shell命令的函数
//   - handleAskExpert: 使用此函数的工具处理函数
//   - handleAskExpert: 使用此函数的工具处理函数
func AskExpert(ctx context.Context, input string) (reply string, err error) {
	// 将输入内容写入临时文件，避免 shell 命令长度限制和 stdin 传递问题
	tmpFile, err := os.CreateTemp("", "dscli-ask-*.md")
	if err != nil {
		return "", fmt.Errorf("创建临时文件失败: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(input); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("写入临时文件失败: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return "", fmt.Errorf("关闭临时文件失败: %w", err)
	}

	script := fmt.Sprintf(`dscli chat --no-color --no-timestamp --histsize 0 --model %s --input %s`,
		context.ModelDeepseekReasoner, tmpPath)
	reply, err = shell.SimpleExecute(ctx, script)
	return
}

// buildStructuredRequest 构建结构化请求
func buildStructuredRequest(userSummary string, originalContent string, attachments []string) (string, []error) {
	var errors []error
	attachmentSection := ""

	if len(attachments) > 0 {
		var attachmentContent strings.Builder
		attachmentContent.WriteString("\n## 附件\n")

		for _, filename := range attachments {
			// 安全检查：防止路径遍历攻击
			if !isSafePath(filename) {
				errors = append(errors, fmt.Errorf("不安全路径: %s", filename))
				continue
			}

			// 检查文件大小（限制为1MB）
			if info, err := os.Stat(filename); err == nil && info.Size() > 1024*1024 {
				errors = append(errors, fmt.Errorf("文件过大: %s (%d字节 > 1MB)", filename, info.Size()))
				continue
			}

			b, err := os.ReadFile(filename)
			if err != nil {
				errors = append(errors, fmt.Errorf("读取文件失败 %s: %w", filename, err))
				continue
			}

			content := strings.TrimSpace(string(b))
			if content == "" {
				errors = append(errors, fmt.Errorf("文件为空: %s", filename))
				continue
			}

			// 使用Markdown代码块格式
			fmt.Fprintf(&attachmentContent, "### %s\n```\n%s\n```\n\n", filename, content)
		}

		if attachmentContent.Len() > len("\n## 附件\n") {
			attachmentSection = attachmentContent.String()
		}
	}

	request := `请以结构化格式回答以下问题。

## 问题背景
` + userSummary + `

## 详细问题
` + originalContent + attachmentSection + `

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

`
	return request, errors
}

// isSafePath 检查文件路径是否安全
// 防止路径遍历攻击，只允许当前目录及其子目录
func isSafePath(filename string) bool {
	// 清理路径
	cleanPath := filepath.Clean(filename)

	// 检查是否包含路径遍历
	if strings.Contains(cleanPath, "..") {
		return false
	}

	// 检查是否为绝对路径
	if filepath.IsAbs(cleanPath) {
		return false
	}

	// 检查是否在当前工作目录下
	cwd, err := os.Getwd()
	if err != nil {
		return false
	}

	fullPath, err := filepath.Abs(cleanPath)
	if err != nil {
		return false
	}

	return strings.HasPrefix(fullPath, cwd)
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