package main

import (
	"context"
)

// GetSystemPrompt 获取系统提示词
func GetSystemPrompt(ctx context.Context) string {
	// 获取当前项目的领域ID
	domainID := GetCurrentDomainID(ctx)

	// 获取当前模型ID
	modelID := GetCurrentModelID(ctx)

	// 获取系统提示词配置
	config := GetSystemPromptConfig(ctx)

	// 使用段落管理器渲染系统提示词
	sm := &SegmentManager{}
	prompt, err := sm.RenderSystemPrompt(ctx, modelID, domainID, config)
	if err != nil || prompt == "" {
		// 如果失败或为空，使用模板化的系统提示词
		return GetEnhancedSystemPromptWithTemplate(ctx)
	}
	return prompt
}

// LoadPrompts 加载提示词
func LoadPrompts(ctx context.Context) ([]Message, error) {
	return []Message{{
		Role:    "system",
		Content: GetSystemPrompt(ctx),
	}}, nil
}

// GetCurrentDomainID 获取当前项目的领域ID
func GetCurrentDomainID(ctx context.Context) int64 {
	return ContextValue(ctx, CurrentDomainID, int64(0))
}

// GetCurrentModelID 获取当前模型ID
func GetCurrentModelID(ctx context.Context) int64 {
	return ContextValue(ctx, CurrentModelID, DeepseekChat)
}
