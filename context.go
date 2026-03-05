package main

import (
	"context"
)

var (
	StartTime    = ContextKeyType("StartTime")
	CurrentModel = ContextKeyType("CurrentModel")
	IsReload     = ContextKeyType("IsReload")
)

type ContextKeyType string

func ContextValue[T any](ctx context.Context, k any, d T) (v T) {
	v, ok := ctx.Value(k).(T)
	if ok {
		return v
	}
	return d
}
