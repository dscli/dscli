package main

import (
	"context"
)

func GetSystemPrompt(ctx context.Context) (prompt string) {
	// 使用模板化的系统提示词
	return GetTemplateSystemPrompt(ctx)
}

func LoadPrompts(ctx context.Context) ([]Message, error) {
	// 使用模板化的提示词加载
	return LoadTemplatePrompts(ctx)
}
