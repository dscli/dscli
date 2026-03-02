package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestPrintReasoningContent(t *testing.T) {
	ctx := context.Background()
	ctx = context.WithValue(ctx, StartTime, time.Now())
	// make sure two keys  no overlap
	ctx = context.WithValue(ctx, CurrentModel, chatModel)
	buf := bytes.NewBuffer([]byte{})
	SetOutputWriter(buf)
	PrintReasoningContent(ctx, "reasoning", "content")
	s := buf.String()
	tag := "用时"
	idx := strings.Index(s, tag)
	if idx == -1 {
		t.Fatal(idx)
	}
	s = s[idx+len(tag):]
	s = strings.Fields(s)[0]
	d, err := time.ParseDuration(s)
	if err != nil {
		t.Fatal(err)
	}
	if d > time.Minute {
		t.Fatal(d)
	}
}
