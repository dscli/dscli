package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// SystemPromptTool 系统提示词工具 - 简化实用版
var systemPromptTool = ToolDef{
	Name:        "system_prompt",
	DisplayName: "SystemPrompt",
	Description: `获取系统提示词

功能：显示指定模型的系统提示词内容

参数：
- model: 可选，指定模型类型
  - chat (默认): Deepseek Chat 模型
  - reasoner: Deepseek Reasoner 模型
  - 或使用数字: 0=chat, 1=reasoner

使用场景：
- 调试系统提示词相关问题
- 查看不同模型的提示词配置
- 学习系统提示词的结构和内容
- 对比不同模型的提示词差异

示例：
- system_prompt: 显示当前Chat模型的提示词
- system_prompt model=reasoner: 显示Reasoner模型的提示词
- system_prompt model=1: 显示Reasoner模型的提示词`,
	Strict: true,
	Parameters: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": false,
	},
	Category: "debug",
	Timeout:  5 * time.Second,
	Handler:  handleSystemPrompt,
}

func init() {
	RegisterTool(systemPromptTool)
}

// handleSystemPrompt 处理系统提示词工具调用
func handleSystemPrompt(ctx context.Context, args ToolArgs) (reply string, err error) {
	// 解析参数
	modelID := int64(0) // 默认Deepseek Chat
	if modelArg, ok := args["model"]; ok {
		modelStr, ok := modelArg.(string)
		if !ok {
			return "", fmt.Errorf("model参数必须是字符串")
		}
		switch strings.ToLower(modelStr) {
		case "chat", "deepseek-chat", "0":
			modelID = 0
		case "reasoner", "deepseek-reasoner", "1":
			modelID = 1
		default:
			return "", fmt.Errorf("不支持的模型: %s。支持: chat(0), reasoner(1)", modelStr)
		}
	}

	// 获取指定模型的系统提示词
	config := NewSystemPromptConfig(ctx)
	config.ModelID = modelID
	prompt := config.GeneratePromptWithTemplate()

	var sb strings.Builder

	// 1. 显示关键环境信息（简洁有用）
	sb.WriteString("## 当前环境\n")
	fmt.Fprintf(&sb, "- 项目: %s\n", config.ProjectName)
	fmt.Fprintf(&sb, "- 目录: %s\n", config.WorkingDirectory)
	fmt.Fprintf(&sb, "- Git: %s @ %s\n", config.GitUserName, config.GitBranch)
	fmt.Fprintf(&sb, "- 时间: %s\n", config.CurrentDate)
	fmt.Fprintf(&sb, "- 模型: %s\n\n", getModelName(modelID))

	// 2. 显示提示词基本信息
	lines := strings.Count(prompt, "\n") + 1
	words := len(strings.Fields(prompt))
	sb.WriteString("## 提示词信息\n")
	fmt.Fprintf(&sb, "- 长度: %d 字符\n", len(prompt))
	fmt.Fprintf(&sb, "- 行数: %d 行\n", lines)
	fmt.Fprintf(&sb, "- 词数: %d 词\n\n", words)

	// 3. 显示系统提示词内容
	sb.WriteString("## 系统提示词内容\n")
	sb.WriteString("```\n")
	sb.WriteString(prompt)
	sb.WriteString("\n```")
	reply = sb.String()
	Println(reply)
	return
}

// getModelName 获取模型名称
func getModelName(modelID int64) string {
	switch modelID {
	case 0:
		return "Deepseek Chat"
	case 1:
		return "Deepseek Reasoner"
	default:
		return fmt.Sprintf("未知模型(%d)", modelID)
	}
}
