package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestPrintContent(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, StartTime, time.Now())
	// make sure two keys  no overlap
	ctx = context.WithValue(ctx, CurrentModel, chatModel)
	buf := bytes.NewBuffer([]byte{})
	SetOutputWriter(buf)
	PrintContent(ctx, "reasoning", "content")
	s := buf.String()

	// 检查输出是否包含 reasoning 和 content
	if !strings.Contains(s, "reasoning") {
		t.Error("missing reasoning")
	}
	if !strings.Contains(s, "content") {
		t.Error("missing content")
	}
	// 检查是否包含执行时间信息
	if !strings.Contains(s, "执行时间") {
		t.Error("missing execution time")
	}
}

func TestPrintToolCalls(t *testing.T) {
}
