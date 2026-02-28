package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintln(t *testing.T) {
	// 设置测试环境
	var buf bytes.Buffer
	SetOutputWriter(&buf)
	// 测试Markdown模式
	Println("Hello **World**")
	output := buf.String()
	if !strings.Contains(output, "Hello **World**") {
		t.Errorf("Println Markdown模式输出不正确: %q", output)
	}
}

func TestPrintf(t *testing.T) {
	// 设置测试环境
	var buf bytes.Buffer
	SetOutputWriter(&buf)
	// 测试Markdown模式
	Printf("Value: %d, Text: %s\n", 42, "**bold**")
	output := buf.String()
	if !strings.Contains(output, "Value: 42, Text: **bold**") {
		t.Errorf("Printf Markdown模式输出不正确: %q", output)
	}
}
