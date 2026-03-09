package main

import (
	"context"
)

var (
	HistSize        = ContextKeyType("HistSize")
	StartTime       = ContextKeyType("StartTime")
	StartBalance    = ContextKeyType("StartBalance")
	CurrentModel    = ContextKeyType("CurrentModel")
	ToolCallID      = ContextKeyType("ToolCallID")
	ShellName       = ContextKeyType("ShellName")
	ShellArgs       = ContextKeyType("ShellArgs")
	ShellStdin      = ContextKeyType("ShellStdin")
	InputContent    = ContextKeyType("InputContent")
	VerboseKey      = ContextKeyType("Verbose")
	InsideShellExec = ContextKeyType("InsideShellExec")
)

type ContextKeyType string

func ContextValue[T any](ctx context.Context, k any, d T) (v T) {
	v, ok := ctx.Value(k).(T)
	if ok {
		return v
	}
	return d
}
