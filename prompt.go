package main

import (
	"context"
)

// GetSystemPrompt 获取系统提示词
func GetSystemPrompt(ctx context.Context) string {
	// 获取当前项目的领域ID
	domainID := GetCurrentDomainID()

	// 获取当前模型ID
	modelID := GetCurrentModelID()

	// 获取系统提示词配置
	config := GetSystemPromptConfig()

	// 使用段落管理器渲染系统提示词
	sm := &SegmentManager{}
	prompt, err := sm.RenderSystemPrompt(ctx, modelID, domainID, config)
	if err != nil {
		// 如果失败，使用基础提示词
		return config.GeneratePrompt()
	}

	return prompt
}

// LoadPrompts 加载提示词
func LoadPrompts(ctx context.Context) ([]Message, error) {
	// 获取当前项目的领域ID
	domainID := GetCurrentDomainID()

	// 获取当前模型ID
	modelID := GetCurrentModelID()

	// 获取系统提示词配置
	config := GetSystemPromptConfig()

	// 使用段落管理器渲染系统提示词
	sm := &SegmentManager{}
	prompt, err := sm.RenderSystemPrompt(ctx, modelID, domainID, config)
	if err != nil {
		return nil, err
	}

	return []Message{{
		Role:    "system",
		Content: prompt,
	}}, nil
}

// LoadSimplePrompts 加载简单提示词（不包含段落）
func LoadSimplePrompts(ctx context.Context) ([]Message, error) {
	// 只使用基础系统提示词
	return []Message{{
		Role:    "system",
		Content: GetSystemPrompt(ctx),
	}}, nil
}

// GetCurrentDomainID 获取当前项目的领域ID
func GetCurrentDomainID() int64 {
	// TODO: 从项目配置或数据库获取当前项目的领域ID
	// 暂时返回编程领域的ID
	return 1 // 编程领域
}

// GetCurrentModelID 获取当前模型ID
func GetCurrentModelID() int64 {
	// TODO: 从配置获取当前模型ID
	// 暂时返回Deepseek Chat
	return DeepseekChat
}
