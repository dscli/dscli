package main

import (
	"context"
	"time"
)

// ToolDef 工具定义
type ToolDef struct {
	Name        string
	DisplayName string
	Description string
	Parameters  map[string]any
	Category    string
	Timeout     time.Duration // 工具执行超时时间
	Handler     func(ctx context.Context, args ToolArgs) (string, error)
}

// ToolArgs 参数定义
type ToolArgs map[string]any

func ToolArgsValue[T any](args ToolArgs, key string, d T) T {
	v, ok := args[key].(T)
	if ok {
		return v
	}
	return d
}
