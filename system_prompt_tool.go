package main

import (
	"context"
	"time"
)

// SystemPromptTool 系统提示词工具
var SystemPromptTool = ToolDef{
	Name:        "system_prompt",
	DisplayName: "SystemPrompt",
	Description: `获取当前系统提示词，帮助理解工作环境和约束条件

参数说明：无参数
使用场景：
1. 了解当前工作环境、权限和约束条件
2. 调试系统提示词相关问题
3. 学习系统提示词的最佳实践
4. 检查模板变量是否正确渲染

注意：系统提示词包含重要的环境信息、工作流程和工具使用指南。`,
	Parameters: map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	},
	Category: "debug",
	Timeout:  1 * time.Second,
	Handler:  HandleSystemPrompt,
}

func init() {
	RegisterTool(SystemPromptTool)
}

// HandleSystemPrompt 处理系统提示词工具调用
func HandleSystemPrompt(ctx context.Context, args map[string]string) (reply string, err error) {
	Println("获取当前系统提示词：")
	prompt := GetSystemPrompt(ctx)
	Println(prompt)
	return prompt, nil
}
