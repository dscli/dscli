package main

import (
	"context"
)

func GetSystemPrompt(ctx context.Context) (prompt string) {
	// 使用增强的系统提示词
	return GetEnhancedSystemPrompt(ctx)
}

func LoadPrompts(ctx context.Context) ([]Message, error) {
	// 使用增强的提示词加载
	return LoadEnhancedPrompts(ctx)
}
