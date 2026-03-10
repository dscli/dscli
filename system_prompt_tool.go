package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SystemPromptTool 系统提示词工具
var SystemPromptTool = ToolDef{
	Name:        "system_prompt",
	DisplayName: "SystemPrompt",
	Description: `获取当前系统提示词，帮助理解工作环境和约束条件

参数说明：
- format: 输出格式，可选值：
  - "full": 完整系统提示词（默认）
  - "summary": 简要摘要（基本信息）
  - "template": 原始模板内容
  - "variables": 模板变量值

使用场景：
1. 了解当前工作环境、权限和约束条件
2. 调试系统提示词相关问题
3. 学习系统提示词的最佳实践
4. 检查模板变量是否正确渲染

注意：系统提示词包含重要的环境信息、工作流程和工具使用指南。`,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"format": map[string]any{
				"type": "string",
				"description": `输出格式，可选值：
- "full": 完整系统提示词（默认）
- "summary": 简要摘要（基本信息）
- "template": 原始模板内容
- "variables": 模板变量值`,
				"enum": []string{"full", "summary", "template", "variables"},
			},
		},
		"required": []string{},
	},
	Category: "debug",
	Timeout:  10 * time.Second,
	Handler:  HandleSystemPrompt,
}

func init() {
	RegisterTool(SystemPromptTool)
}

// HandleSystemPrompt 处理系统提示词工具调用
func HandleSystemPrompt(ctx context.Context, args map[string]string) (reply string, err error) {
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

// formatFullPrompt 格式化完整提示词
func formatFullPrompt(prompt string, config *SystemPromptConfig, template *SystemPromptTemplate) string {
	var sb strings.Builder

	sb.WriteString("# 📋 系统提示词信息\n\n")

	// 基本信息
	sb.WriteString("## 📊 基本信息\n")
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
	sb.WriteString("## 📊 基本信息\n")
	sb.WriteString(fmt.Sprintf("- **当前日期**: %s\n", config.CurrentDate))
	sb.WriteString(fmt.Sprintf("- **项目名称**: %s\n", config.ProjectName))
	sb.WriteString(fmt.Sprintf("- **项目类型**: %s\n", config.ProjectType))
	sb.WriteString(fmt.Sprintf("- **Git用户**: %s <%s>\n", config.GitUserName, config.GitUserEmail))
	sb.WriteString(fmt.Sprintf("- **Git分支**: %s\n", config.GitBranch))
	sb.WriteString(fmt.Sprintf("- **Git状态**: %s\n", config.GitStatus))
	sb.WriteString(fmt.Sprintf("- **工作目录**: %s\n", config.WorkingDirectory))
	sb.WriteString(fmt.Sprintf("- **主机**: %s\n", config.Hostname))
	sb.WriteString(fmt.Sprintf("- **用户**: %s\n\n", config.Username))

	// 提示词长度信息
	lines := strings.Count(prompt, "\n") + 1
	words := len(strings.Fields(prompt))
	sb.WriteString("## 📏 提示词统计\n")
	sb.WriteString(fmt.Sprintf("- **总行数**: %d\n", lines))
	sb.WriteString(fmt.Sprintf("- **总字数**: %d\n", words))
	sb.WriteString(fmt.Sprintf("- **模型ID**: %d\n\n", config.ModelID))

	// 关键信息摘要
	sb.WriteString("## 🔑 关键信息\n")

	// 提取关键段落
	if strings.Contains(prompt, "## 文件操作权限") {
		sb.WriteString("- **文件权限**: 可操作工作目录和配置目录文件\n")
	}
	if strings.Contains(prompt, "## 你的工作流程") {
		sb.WriteString("- **工作流程**: 5步工作法（分析、工具调用、执行、分析结果、给出答案）\n")
	}
	if strings.Contains(prompt, "## 重要原则") {
		sb.WriteString("- **重要原则**: 逻辑严谨、使用现有工具、代码质量、Git保存、尊重版权\n")
	}
	if strings.Contains(prompt, "## 工具选择指南") {
		sb.WriteString("- **工具指南**: 优先使用基于代码结构的新工具\n")
	}
	if strings.Contains(prompt, "## 代码质量保证流程") {
		sb.WriteString("- **质量流程**: 代码修改、专家审阅、问题修复、推送远程\n")
	}

	return sb.String()
}

// formatTemplatePrompt 格式化模板内容
func formatTemplatePrompt(template *SystemPromptTemplate, config *SystemPromptConfig) string {
	var sb strings.Builder

	sb.WriteString("# 📋 系统提示词模板\n\n")

	sb.WriteString("## 📊 模板信息\n")
	sb.WriteString(fmt.Sprintf("- **模板名称**: %s\n", template.Name))
	sb.WriteString(fmt.Sprintf("- **模型ID**: %d\n\n", config.ModelID))

	sb.WriteString("## 📝 原始模板内容\n")
	sb.WriteString("```\n")
	sb.WriteString(template.Template)
	sb.WriteString("\n```")

	return sb.String()
}

// formatVariablesPrompt 格式化模板变量
func formatVariablesPrompt(config *SystemPromptConfig) string {
	var sb strings.Builder

	sb.WriteString("# 📋 系统提示词模板变量\n\n")

	sb.WriteString("## 📊 基础信息\n")
	sb.WriteString(fmt.Sprintf("- **CurrentDate**: %s\n", config.CurrentDate))
	sb.WriteString(fmt.Sprintf("- **ProjectRoot**: %s\n", config.ProjectRoot))
	sb.WriteString(fmt.Sprintf("- **ConfigDir**: %s\n", config.ConfigDir))
	sb.WriteString(fmt.Sprintf("- **WorkingDirectory**: %s\n", config.WorkingDirectory))
	sb.WriteString(fmt.Sprintf("- **Hostname**: %s\n", config.Hostname))
	sb.WriteString(fmt.Sprintf("- **Username**: %s\n\n", config.Username))

	sb.WriteString("## 📊 Git信息\n")
	sb.WriteString(fmt.Sprintf("- **GitUserName**: %s\n", config.GitUserName))
	sb.WriteString(fmt.Sprintf("- **GitUserEmail**: %s\n", config.GitUserEmail))
	sb.WriteString(fmt.Sprintf("- **GitBranch**: %s\n", config.GitBranch))
	sb.WriteString(fmt.Sprintf("- **GitStatus**: %s\n\n", config.GitStatus))

	sb.WriteString("## 📊 项目信息\n")
	sb.WriteString(fmt.Sprintf("- **ProjectName**: %s\n", config.ProjectName))
	sb.WriteString(fmt.Sprintf("- **ProjectType**: %s\n\n", config.ProjectType))

	sb.WriteString("## 📊 模型配置\n")
	sb.WriteString(fmt.Sprintf("- **ModelID**: %d\n", config.ModelID))

	return sb.String()
}
