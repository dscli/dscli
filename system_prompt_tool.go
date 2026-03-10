package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SystemPromptTool 系统提示词工具 - 简化实用版
var SystemPromptTool = ToolDef{
	Name:        "system_prompt",
	DisplayName: "SystemPrompt",
	Description: `获取当前系统提示词

功能：显示当前使用的系统提示词内容

使用场景：
- 调试系统提示词相关问题
- 查看当前工作环境配置
- 学习系统提示词的结构和内容`,
	Category: "debug",
	Timeout:  5 * time.Second,
	Handler:  HandleSystemPrompt,
}

func init() {
	RegisterTool(SystemPromptTool)
}

// HandleSystemPrompt 处理系统提示词工具调用
func HandleSystemPrompt(ctx context.Context, args map[string]string) (reply string, err error) {
	// 获取系统提示词
	prompt := GetSystemPrompt(ctx)

	// 获取配置信息（用于显示关键信息）
	config := NewSystemPromptConfig(ctx)

	var sb strings.Builder

	// 1. 显示关键环境信息（简洁有用）
	sb.WriteString("## 当前环境\n")
	sb.WriteString(fmt.Sprintf("- 项目: %s\n", config.ProjectName))
	sb.WriteString(fmt.Sprintf("- 目录: %s\n", config.WorkingDirectory))
	sb.WriteString(fmt.Sprintf("- Git: %s @ %s\n", config.GitUserName, config.GitBranch))
	sb.WriteString(fmt.Sprintf("- 时间: %s\n\n", config.CurrentDate))

	// 2. 显示提示词基本信息
	lines := strings.Count(prompt, "\n") + 1
	words := len(strings.Fields(prompt))
	sb.WriteString("## 提示词信息\n")
	sb.WriteString(fmt.Sprintf("- 长度: %d 字符\n", len(prompt)))
	sb.WriteString(fmt.Sprintf("- 行数: %d 行\n", lines))
	sb.WriteString(fmt.Sprintf("- 词数: %d 词\n\n", words))

	// 3. 显示系统提示词内容
	sb.WriteString("## 系统提示词内容\n")
	sb.WriteString("```\n")
	sb.WriteString(prompt)
	sb.WriteString("\n```")

	return sb.String(), nil
}
