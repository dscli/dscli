package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SystemPromptTool 工具定义 - 获取当前系统提示词
var SystemPromptTool = ToolDef{
	Name:        "system_prompt",
	DisplayName: "系统提示词",
	Description: `获取当前系统提示词。

功能说明：
1. 获取当前使用的系统提示词内容
2. 显示系统提示词的来源（模板、段落管理器等）
3. 帮助理解当前的工作环境和约束条件

使用场景：
1. 了解当前的工作环境和权限
2. 查看系统提示词是否需要更新
3. 调试系统提示词相关问题
4. 学习系统提示词的最佳实践

返回信息：
- 系统提示词完整内容
- 提示词来源信息
- 模板变量替换状态`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"format": map[string]any{
				"type": "string",
				"description": `输出格式，可选值：
- "full": 完整系统提示词（默认）
- "summary": 简要摘要
- "template": 原始模板（未渲染）
- "variables": 模板变量值`,
				"enum": []string{"full", "summary", "template", "variables"},
			},
		},
		"required": []string{},
	},
	Category: "debug",
	Timeout:  10 * time.Second,
	Handler:  handleSystemPrompt,
}

func init() {
	RegisterTool(SystemPromptTool)
}

// handleSystemPrompt 处理系统提示词工具调用
func handleSystemPrompt(ctx context.Context, args map[string]string) (reply string, err error) {
	format := args["format"]
	if format == "" {
		format = "full"
	}

	// 获取系统提示词配置
	config := NewSystemPromptConfig(ctx)

	// 获取当前模型ID
	modelID := GetCurrentModelID()

	// 获取模板
	template := GetTemplateForModel(modelID)

	// 获取渲染后的系统提示词
	systemPrompt := GetSystemPrompt(ctx)

	// 根据格式返回不同信息
	switch format {
	case "full":
		return formatFullPrompt(systemPrompt, config, template), nil
	case "summary":
		return formatSummaryPrompt(systemPrompt, config), nil
	case "template":
		return formatTemplatePrompt(template, config), nil
	case "variables":
		return formatVariablesPrompt(config), nil
	default:
		return "", fmt.Errorf("不支持的格式: %s", format)
	}
}

// formatFullPrompt 格式化完整系统提示词
func formatFullPrompt(prompt string, config *SystemPromptConfig, template *SystemPromptTemplate) string {
	var sb strings.Builder

	sb.WriteString("# 📋 系统提示词信息\n\n")

	// 基本信息
	sb.WriteString("## 🔍 基本信息\n")
	sb.WriteString(fmt.Sprintf("- **模型**: %s\n", getModelName(config.ModelID)))
	sb.WriteString(fmt.Sprintf("- **模板**: %s\n", template.Name))
	sb.WriteString(fmt.Sprintf("- **来源**: %s\n", getPromptSource()))
	sb.WriteString(fmt.Sprintf("- **长度**: %d 字符\n\n", len(prompt)))

	// 模板变量
	sb.WriteString("## 📊 模板变量\n")
	sb.WriteString(fmt.Sprintf("- **当前日期**: %s\n", config.CurrentDate))
	sb.WriteString(fmt.Sprintf("- **项目名称**: %s\n", config.ProjectName))
	sb.WriteString(fmt.Sprintf("- **项目类型**: %s\n", config.ProjectType))
	sb.WriteString(fmt.Sprintf("- **Git用户**: %s <%s>\n", config.GitUserName, config.GitUserEmail))
	sb.WriteString(fmt.Sprintf("- **Git分支**: %s\n", config.GitBranch))
	sb.WriteString(fmt.Sprintf("- **工作目录**: %s\n\n", config.WorkingDirectory))

	// 完整系统提示词
	sb.WriteString("## 📝 完整系统提示词\n")
	sb.WriteString("```\n")
	sb.WriteString(prompt)
	sb.WriteString("\n```")

	return sb.String()
}

// formatSummaryPrompt 格式化简要摘要
func formatSummaryPrompt(prompt string, config *SystemPromptConfig) string {
	var sb strings.Builder

	sb.WriteString("# 📋 系统提示词摘要\n\n")

	// 基本信息
	sb.WriteString("## 🔍 基本信息\n")
	sb.WriteString(fmt.Sprintf("- **模型**: %s\n", getModelName(config.ModelID)))
	sb.WriteString(fmt.Sprintf("- **项目**: %s (%s)\n", config.ProjectName, config.ProjectType))
	sb.WriteString(fmt.Sprintf("- **日期**: %s\n", config.CurrentDate))
	sb.WriteString(fmt.Sprintf("- **Git**: %s @ %s\n", config.GitUserName, config.GitBranch))
	sb.WriteString(fmt.Sprintf("- **长度**: %d 字符\n\n", len(prompt)))

	// 关键段落
	sb.WriteString("## 📊 关键段落\n")

	lines := strings.Split(prompt, "\n")
	sectionCount := 0
	currentSection := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if currentSection != "" && sectionCount < 5 {
				sb.WriteString(fmt.Sprintf("- %s\n", currentSection))
				sectionCount++
			}
			currentSection = strings.TrimPrefix(line, "## ")
		}
	}

	if currentSection != "" && sectionCount < 5 {
		sb.WriteString(fmt.Sprintf("- %s\n", currentSection))
	}

	return sb.String()
}

// formatTemplatePrompt 格式化原始模板
func formatTemplatePrompt(template *SystemPromptTemplate, config *SystemPromptConfig) string {
	var sb strings.Builder

	sb.WriteString("# 📋 系统提示词模板\n\n")

	sb.WriteString("## 🔍 模板信息\n")
	sb.WriteString(fmt.Sprintf("- **名称**: %s\n", template.Name))
	sb.WriteString(fmt.Sprintf("- **模型**: %s\n", getModelName(config.ModelID)))
	sb.WriteString(fmt.Sprintf("- **长度**: %d 字符\n\n", len(template.Template)))

	// 模板内容
	sb.WriteString("## 📝 模板内容\n")
	sb.WriteString("```\n")
	sb.WriteString(template.Template)
	sb.WriteString("\n```")

	return sb.String()
}

// formatVariablesPrompt 格式化模板变量
func formatVariablesPrompt(config *SystemPromptConfig) string {
	var sb strings.Builder

	sb.WriteString("# 📋 模板变量值\n\n")

	// 基础信息
	sb.WriteString("## 📊 基础信息\n")
	sb.WriteString(fmt.Sprintf("- **当前日期**: %s\n", config.CurrentDate))
	sb.WriteString(fmt.Sprintf("- **项目根目录**: %s\n", config.ProjectRoot))
	sb.WriteString(fmt.Sprintf("- **配置目录**: %s\n", config.ConfigDir))
	sb.WriteString(fmt.Sprintf("- **工作目录**: %s\n\n", config.WorkingDirectory))

	// Git信息
	sb.WriteString("## 🔧 Git信息\n")
	sb.WriteString(fmt.Sprintf("- **用户名**: %s\n", config.GitUserName))
	sb.WriteString(fmt.Sprintf("- **邮箱**: %s\n", config.GitUserEmail))
	sb.WriteString(fmt.Sprintf("- **分支**: %s\n", config.GitBranch))
	sb.WriteString(fmt.Sprintf("- **状态**: %s\n\n", config.GitStatus))

	// 项目信息
	sb.WriteString("## 📁 项目信息\n")
	sb.WriteString(fmt.Sprintf("- **项目名称**: %s\n", config.ProjectName))
	sb.WriteString(fmt.Sprintf("- **项目类型**: %s\n", config.ProjectType))

	// 环境信息
	sb.WriteString(fmt.Sprintf("- **主机名**: %s\n", config.Hostname))
	sb.WriteString(fmt.Sprintf("- **用户名**: %s\n", config.Username))

	return sb.String()
}

// getModelName 获取模型名称
func getModelName(modelID int64) string {
	switch modelID {
	case DeepseekChat:
		return "Deepseek Chat"
	case DeepseekReasoner:
		return "Deepseek Reasoner"
	default:
		return fmt.Sprintf("未知模型 (%d)", modelID)
	}
}

// getPromptSource 获取提示词来源
func getPromptSource() string {
	// 这里可以添加更复杂的逻辑来检测提示词来源
	// 目前假设使用模板
	return "模板渲染"
}
