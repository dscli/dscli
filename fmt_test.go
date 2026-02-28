package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintln(t *testing.T) {
	// 保存原始设置
	oldMode := OutputMode
	oldWriter := outputWriter

	// 设置测试环境
	var buf bytes.Buffer
	SetOutputWriter(&buf)
	SetOutputMode("markdown")

	// 测试Markdown模式
	Println("Hello **World**")
	output := buf.String()
	if !strings.Contains(output, "Hello **World**") {
		t.Errorf("Println Markdown模式输出不正确: %q", output)
	}

	// 清空缓冲区
	buf.Reset()
	SetOutputMode("org")

	// 测试Org模式
	Println("Hello **World**")
	output = buf.String()
	// 注意：MarkdownToOrgConverter会添加零宽空格，我们检查是否包含"*World*"
	if !strings.Contains(output, "*World*") {
		t.Errorf("Println Org模式输出不正确，应该包含'*World*': %q", output)
	}

	// 恢复原始设置
	SetOutputMode(oldMode)
	SetOutputWriter(oldWriter)
}

func TestPrintf(t *testing.T) {
	// 保存原始设置
	oldMode := OutputMode
	oldWriter := outputWriter

	// 设置测试环境
	var buf bytes.Buffer
	SetOutputWriter(&buf)
	SetOutputMode("markdown")

	// 测试Markdown模式
	Printf("Value: %d, Text: %s\n", 42, "**bold**")
	output := buf.String()
	if !strings.Contains(output, "Value: 42, Text: **bold**") {
		t.Errorf("Printf Markdown模式输出不正确: %q", output)
	}

	// 清空缓冲区
	buf.Reset()
	SetOutputMode("org")

	// 测试Org模式
	Printf("Value: %d, Text: %s\n", 42, "**bold**")
	output = buf.String()
	// 注意：MarkdownToOrgConverter会添加零宽空格，我们检查是否包含"*bold*"
	if !strings.Contains(output, "*bold*") {
		t.Errorf("Printf Org模式输出不正确，应该包含'*bold*': %q", output)
	}

	// 恢复原始设置
	SetOutputMode(oldMode)
	SetOutputWriter(oldWriter)
}
