package main

import (
	"context"
	"io"
	"time"
)

var (
	HistSizeKey         = ContextKeyType[int]{"HistSize"}
	StartTimeKey        = ContextKeyType[time.Time]{"StartTime"}
	StartBalanceKey     = ContextKeyType[BalanceInfo]{"StartBalance"}
	CurrentModelIDKey   = ContextKeyType[int64]{"CurrentModelID"}
	CurrentModelNameKey = ContextKeyType[string]{"CurrentModelName"}
	CurrentDomainIDKey  = ContextKeyType[int64]{"CurrentDomainID"}
	CurrentSessionIDKey = ContextKeyType[int64]{"CurrentSessionID"}
	ToolCallIDKey       = ContextKeyType[string]{"ToolCallID"}
	ShellNameKey        = ContextKeyType[string]{"ShellName"}
	ShellArgsKey        = ContextKeyType[[]string]{"ShellArgs"}
	ShellStdinKey       = ContextKeyType[io.Reader]{"ShellStdin"}
	InputContentKey     = ContextKeyType[string]{"InputContent"}
	VerboseKey          = ContextKeyType[bool]{"Verbose"}
	InsideShellExecKey  = ContextKeyType[bool]{"InsideShellExec"}
	StreamKey           = ContextKeyType[bool]{"Stream"}
	LeftTokensKey       = ContextKeyType[int]{"LeftTokens"}
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
