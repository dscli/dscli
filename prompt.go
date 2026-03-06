package main

import (
	"context"
)

// GetSystemPrompt 获取系统提示词
func GetSystemPrompt(ctx context.Context) (prompt string) {
	// 使用模板化的系统提示词
	return GetTemplateSystemPrompt(ctx)
}

// LoadPrompts 加载提示词
func LoadPrompts(ctx context.Context) ([]Message, error) {
	// 使用包含段落的系统消息
	return BuildSystemMessages(ctx)
}

// LoadSimplePrompts 加载简单提示词（不包含段落）
func LoadSimplePrompts(ctx context.Context) ([]Message, error) {
	// 只使用基础系统提示词
	return []Message{{
		Role:    "system",
		Content: GetTemplateSystemPrompt(ctx),
	}}, nil
}
