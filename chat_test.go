package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestPrintContent(t *testing.T) {
	ctx := context.WithValue(context.Background(), StartTime, time.Now())
	buf := bytes.NewBuffer([]byte{})
	SetOutputWriter(buf)
	PrintContent(ctx, "yes")
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
