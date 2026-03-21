package main

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

// PromptAnalyzeTool 提示词分析工具
var promptAnalyzeTool = ToolDef{
	Name:        "prompt_analyze",
	DisplayName: "PromptAnalyze",
	Description: `分析系统提示词，识别潜在问题和改进机会

功能：分析当前模型的系统提示词，提供改进建议

参数：
- model: 可选，指定要分析的模型类型
  - chat (默认): Deepseek Chat 模型
  - reasoner: Deepseek Reasoner 模型
  - 或使用数字: 0=chat, 1=reasoner

分析维度：
1. 完整性检查：是否包含所有必要部分
2. 清晰度检查：是否有模糊或矛盾的描述
3. 有效性检查：指导原则是否明确可执行
4. 结构检查：组织是否合理

输出：
- 分析报告，包含问题识别和改进建议
- 不实际修改提示词，只提供分析

使用场景：
- 优化AI助手的行为指南
- 识别提示词中的潜在问题
- 为提示词改进提供数据支持

示例：
- prompt_analyze: 分析当前Chat模型的提示词
- prompt_analyze model=reasoner: 分析Reasoner模型的提示词`,
	Strict: true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"model": map[string]any{
				"type":        "string",
				"description": `模型名字。`,
				"pattern":     TitleLikePattern(40),
			},
		},
		"required":             []string{},
		"additionalProperties": false,
	},
	Category: "debug",
	Timeout:  10 * time.Second,
	Handler:  handlePromptAnalyze,
}

func init() {
	RegisterTool(promptAnalyzeTool)
}

// PromptAnalysisResult 提示词分析结果
type PromptAnalysisResult struct {
	ModelID     int64
	Prompt      string
	Issues      []PromptIssue
	Suggestions []PromptSuggestion
}

// PromptIssue 提示词问题描述
type PromptIssue struct {
	Type        string // "completeness", "clarity", "effectiveness", "structure"
	Description string
	Severity    string // "low", "medium", "high"
	Location    string // 大致位置描述
}

// PromptSuggestion 提示词改进建议
type PromptSuggestion struct {
	Title      string
	Problem    string
	Suggestion string
	Priority   string // "low", "medium", "high"
}

// handlePromptAnalyze 处理提示词分析工具调用
func handlePromptAnalyze(ctx context.Context, args ToolArgs) (reply string, err error) {
	// 解析参数
	modelID := int64(0) // 默认Deepseek Chat
	model := ToolArgsValue(args, "model", ModelDeepseekChat)
	if model == "" {
		return "", fmt.Errorf("model参数必须是字符串")
	}

	switch strings.ToLower(model) {
	case "chat", "deepseek-chat", "0":
		modelID = 0
	case "reasoner", "deepseek-reasoner", "1":
		modelID = 1
	default:
		return "", fmt.Errorf("不支持的模型: %s。支持: chat(0), reasoner(1)", model)
	}

	// 获取指定模型的系统提示词
	config := NewSystemPromptConfig(ctx)
	config.ModelID = modelID
	prompt := config.GeneratePromptWithTemplate()

	// 执行分析
	analysis := analyzePrompt(prompt, modelID)

	var sb strings.Builder

	// 1. 基本信息
	sb.WriteString("# 系统提示词分析报告\n\n")
	sb.WriteString("## 基本信息\n")
	fmt.Fprintf(&sb, "- 分析模型: %s\n", getPromptModelName(modelID))
	fmt.Fprintf(&sb, "- 提示词长度: %d 字符\n", len(prompt))
	fmt.Fprintf(&sb, "- 提示词行数: %d 行\n", strings.Count(prompt, "\n")+1)
	fmt.Fprintf(&sb, "- 分析时间: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))

	// 2. 分析摘要
	sb.WriteString("## 分析摘要\n")
	if len(analysis.Issues) == 0 {
		sb.WriteString("✅ 提示词结构良好，未发现明显问题。\n\n")
	} else {
		fmt.Fprintf(&sb, "发现 %d 个潜在问题：\n", len(analysis.Issues))
		for i, issue := range analysis.Issues {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, issue.Description)
		}
		sb.WriteString("\n")
	}

	// 3. 详细分析
	sb.WriteString("## 详细分析\n")

	// 3.1 完整性检查
	sb.WriteString("### 1. 完整性检查\n")
	completeness := checkPromptCompleteness(prompt, modelID)
	sb.WriteString(completeness)
	sb.WriteString("\n")

	// 3.2 清晰度检查
	sb.WriteString("### 2. 清晰度检查\n")
	clarity := checkPromptClarity(prompt)
	sb.WriteString(clarity)
	sb.WriteString("\n")

	// 3.3 有效性检查
	sb.WriteString("### 3. 有效性检查\n")
	effectiveness := checkPromptEffectiveness(prompt, modelID)
	sb.WriteString(effectiveness)
	sb.WriteString("\n")

	// 4. 改进建议
	sb.WriteString("## 改进建议\n")
	if len(analysis.Suggestions) == 0 {
		sb.WriteString("暂无具体改进建议。当前提示词设计合理。\n\n")
	} else {
		for i, suggestion := range analysis.Suggestions {
			fmt.Fprintf(&sb, "%d. **%s**\n", i+1, suggestion.Title)
			fmt.Fprintf(&sb, "   - 问题: %s\n", suggestion.Problem)
			fmt.Fprintf(&sb, "   - 建议: %s\n", suggestion.Suggestion)
			if suggestion.Priority != "" {
				fmt.Fprintf(&sb, "   - 优先级: %s\n", suggestion.Priority)
			}
			sb.WriteString("\n")
		}
	}

	// 5. 后续步骤
	sb.WriteString("## 后续步骤\n")
	sb.WriteString("1. 如需应用这些改进，请使用 `prompt_suggest` 工具生成具体修改方案\n")
	sb.WriteString("2. 修改前请确保理解所有变更的影响\n")
	sb.WriteString("3. 建议小范围测试后再全面应用\n")

	reply = sb.String()
	outfmt.Println(reply)
	return
}

// analyzePrompt 分析提示词
func analyzePrompt(prompt string, modelID int64) PromptAnalysisResult {
	result := PromptAnalysisResult{
		ModelID: modelID,
		Prompt:  prompt,
	}

	// 收集所有问题
	result.Issues = collectPromptIssues(prompt, modelID)

	// 基于问题生成建议
	result.Suggestions = generatePromptSuggestions(result.Issues, modelID)

	return result
}

// collectPromptIssues 收集提示词问题
func collectPromptIssues(prompt string, modelID int64) []PromptIssue {
	var issues []PromptIssue

	// 检查关键部分是否存在
	if modelID == 0 {
		if !containsAnyString(prompt, []string{"工作流程", "执行指导", "操作步骤"}) {
			issues = append(issues, PromptIssue{
				Type:        "completeness",
				Description: "缺少明确的工作流程或执行指导",
				Severity:    "medium",
				Location:    "整体结构",
			})
		}
	}

	// 检查是否有模糊的描述
	if containsAnyString(prompt, []string{"可能", "大概", "或许", "一般", "通常"}) {
		issues = append(issues, PromptIssue{
			Type:        "clarity",
			Description: "包含模糊的描述词汇，可能导致理解不一致",
			Severity:    "low",
			Location:    "描述性内容",
		})
	}

	// 检查是否有矛盾的内容
	// 这里可以添加更复杂的逻辑检查

	// 检查结构是否清晰
	sectionCount := countPromptSections(prompt)
	if sectionCount < 3 && modelID == 0 {
		issues = append(issues, PromptIssue{
			Type:        "structure",
			Description: "结构可能过于简单，缺乏分层组织",
			Severity:    "low",
			Location:    "整体结构",
		})
	}

	return issues
}

// generatePromptSuggestions 生成提示词改进建议
func generatePromptSuggestions(issues []PromptIssue, modelID int64) []PromptSuggestion {
	var suggestions []PromptSuggestion

	for _, issue := range issues {
		switch issue.Type {
		case "completeness":
			if issue.Description == "缺少明确的工作流程或执行指导" {
				suggestions = append(suggestions, PromptSuggestion{
					Title:      "添加明确的工作流程",
					Problem:    "AI助手可能不清楚如何系统地处理问题",
					Suggestion: "添加清晰的工作流程步骤，如：1)理解问题 2)分析方案 3)执行操作 4)验证结果",
					Priority:   "medium",
				})
			}
		case "clarity":
			if strings.Contains(issue.Description, "模糊的描述词汇") {
				suggestions = append(suggestions, PromptSuggestion{
					Title:      "替换模糊词汇",
					Problem:    "模糊词汇可能导致AI行为不一致",
					Suggestion: "将模糊词汇替换为明确的指导，如将'通常'改为'应该'，将'可能'改为'需要'",
					Priority:   "low",
				})
			}
		case "structure":
			if strings.Contains(issue.Description, "结构可能过于简单") {
				suggestions = append(suggestions, PromptSuggestion{
					Title:      "优化提示词结构",
					Problem:    "简单的结构可能无法有效指导复杂任务",
					Suggestion: "使用分层结构组织内容，如：核心身份 → 工作流程 → 工具指南 → 质量要求",
					Priority:   "low",
				})
			}
		}
	}

	// 添加通用建议
	if modelID == 0 {
		suggestions = append(suggestions, PromptSuggestion{
			Title:      "添加具体示例",
			Problem:    "抽象的描述可能不够具体",
			Suggestion: "在关键部分添加具体示例，如工具使用示例、代码质量示例",
			Priority:   "low",
		})
	}

	return suggestions
}

// checkPromptCompleteness 检查提示词完整性
func checkPromptCompleteness(prompt string, modelID int64) string {
	var sb strings.Builder

	var requiredParts []string
	if modelID == 0 {
		requiredParts = []string{"身份", "工作流程", "工具", "质量", "注意事项"}
	} else {
		requiredParts = []string{"思考", "原则", "流程"}
	}

	foundCount := 0
	for _, part := range requiredParts {
		if strings.Contains(prompt, part) {
			foundCount++
			fmt.Fprintf(&sb, "✅ 包含'%s'相关内容\n", part)
		} else {
			fmt.Fprintf(&sb, "⚠️  缺少'%s'相关内容\n", part)
		}
	}

	coverage := float64(foundCount) / float64(len(requiredParts)) * 100
	fmt.Fprintf(&sb, "\n完整性覆盖率: %.1f%%\n", coverage)

	if coverage < 80 {
		sb.WriteString("建议：补充缺失的关键部分\n")
	}

	return sb.String()
}

// checkPromptClarity 检查提示词清晰度
func checkPromptClarity(prompt string) string {
	var sb strings.Builder

	// 检查模糊词汇
	vagueWords := []string{"可能", "大概", "或许", "一般", "通常", "有时"}
	vagueCount := 0
	for _, word := range vagueWords {
		if strings.Contains(prompt, word) {
			vagueCount++
		}
	}

	if vagueCount == 0 {
		sb.WriteString("✅ 未发现模糊词汇\n")
	} else {
		sb.WriteString(fmt.Sprintf("⚠️  发现 %d 个模糊词汇\n", vagueCount))
		sb.WriteString("建议：使用更明确的指导语言\n")
	}

	// 检查句子长度
	lines := strings.Split(prompt, "\n")
	longLineCount := 0
	for _, line := range lines {
		if len(line) > 100 && strings.TrimSpace(line) != "" {
			longLineCount++
		}
	}

	if longLineCount == 0 {
		sb.WriteString("✅ 句子长度适中\n")
	} else {
		fmt.Fprintf(&sb, "⚠️  有 %d 行超过100字符\n", longLineCount)
		sb.WriteString("建议：拆分过长的句子，提高可读性\n")
	}

	return sb.String()
}

// checkPromptEffectiveness 检查提示词有效性
func checkPromptEffectiveness(prompt string, modelID int64) string {
	var sb strings.Builder

	// 检查是否有具体的行动指导
	actionWords := []string{"应该", "必须", "需要", "确保", "检查", "验证"}
	actionCount := 0
	for _, word := range actionWords {
		if strings.Contains(prompt, word) {
			actionCount++
		}
	}

	fmt.Fprintf(&sb, "行动指导词汇: %d 个\n", actionCount)
	if actionCount < 3 {
		sb.WriteString("⚠️  行动指导可能不够具体\n")
		sb.WriteString("建议：添加更多具体的行动指导\n")
	} else {
		sb.WriteString("✅ 有足够的行动指导\n")
	}

	// 检查是否有具体的质量要求
	if modelID == 0 {
		qualityWords := []string{"简洁", "注释", "错误处理", "最佳实践", "规范"}
		qualityCount := 0
		for _, word := range qualityWords {
			if strings.Contains(prompt, word) {
				qualityCount++
			}
		}

		fmt.Fprintf(&sb, "质量要求相关: %d 项\n", qualityCount)
		if qualityCount < 2 {
			sb.WriteString("⚠️  质量要求可能不够全面\n")
			sb.WriteString("建议：明确代码质量和最佳实践要求\n")
		} else {
			sb.WriteString("✅ 质量要求明确\n")
		}
	}

	return sb.String()
}

// 辅助函数
func containsAnyString(s string, substrs []string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func countPromptSections(prompt string) int {
	// 简单统计章节标题数量
	count := 0
	for line := range strings.SplitSeq(prompt, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "##") ||
			strings.HasPrefix(trimmed, "###") || strings.HasPrefix(trimmed, "####") {
			count++
		}
	}
	return count
}

func getPromptModelName(modelID int64) string {
	switch modelID {
	case 0:
		return "Deepseek Chat"
	case 1:
		return "Deepseek Reasoner"
	default:
		return fmt.Sprintf("未知模型(%d)", modelID)
	}
}
