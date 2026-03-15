package main

import (
	"context"
	"io"
	"time"
)

var (
	HistSize         = ContextKeyType[int]{"HistSize"}
	StartTime        = ContextKeyType[time.Time]{"StartTime"}
	StartBalance     = ContextKeyType[BalanceInfo]{"StartBalance"}
	CurrentModelID   = ContextKeyType[int64]{"CurrentModelID"}
	CurrentModelName = ContextKeyType[string]{"CurrentModelName"}
	CurrentDomainID  = ContextKeyType[int64]{"CurrentDomainID"}
	CurrentSessionID = ContextKeyType[int64]{"CurrentSessionID"}
	ToolCallID       = ContextKeyType[string]{"ToolCallID"}
	ShellName        = ContextKeyType[string]{"ShellName"}
	ShellArgs        = ContextKeyType[[]string]{"ShellArgs"}
	ShellStdin       = ContextKeyType[io.Reader]{"ShellStdin"}
	InputContent     = ContextKeyType[string]{"InputContent"}
	VerboseKey       = ContextKeyType[bool]{"Verbose"}
	InsideShellExec  = ContextKeyType[bool]{"InsideShellExec"}
	StreamKey        = ContextKeyType[bool]{"Stream"}
	LeftTokens       = ContextKeyType[int]{"LeftTokens"}
)

type ContextKeyType[T any] struct {
	name string
}

func ContextValue[T any](ctx context.Context, k ContextKeyType[T], d T) (v T) {
	v, ok := ctx.Value(k).(T)
	if ok {
		return v
	}
	return d
}
