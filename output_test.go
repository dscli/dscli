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

// TestWarnWritesToStderr verifies that Warn() writes to outputErrorWriter,
// not to the main outputWriter.
func TestWarnWritesToStderr(t *testing.T) {
	var stdoutBuf bytes.Buffer
	oldWriter := outputWriter
	SetOutputWriter(&stdoutBuf)
	defer SetOutputWriter(oldWriter)

	var stderrBuf bytes.Buffer
	oldErrWriter := outputErrorWriter
	SetErrorWriter(&stderrBuf)
	defer SetErrorWriter(oldErrWriter)

	oldLevel := outputCurrentLogLevel
	SetLogLevel(LogLevelDebug)
	defer SetLogLevel(oldLevel)

	Warn("test warning %d", 42)

	if stderrBuf.Len() == 0 {
		t.Fatal("Warn() did not write to outputErrorWriter")
	}
	if !strings.Contains(stderrBuf.String(), "test warning 42") {
		t.Errorf("Warn() stderr output missing message, got: %q", stderrBuf.String())
	}
	if strings.Contains(stdoutBuf.String(), "test warning 42") {
		t.Error("Warn() leaked message to stdout instead of stderr")
	}
}

// TestErrorWritesToStderr verifies that Error() writes to outputErrorWriter.
func TestErrorWritesToStderr(t *testing.T) {
	var stdoutBuf bytes.Buffer
	oldWriter := outputWriter
	SetOutputWriter(&stdoutBuf)
	defer SetOutputWriter(oldWriter)

	var stderrBuf bytes.Buffer
	oldErrWriter := outputErrorWriter
	SetErrorWriter(&stderrBuf)
	defer SetErrorWriter(oldErrWriter)

	oldLevel := outputCurrentLogLevel
	SetLogLevel(LogLevelDebug)
	defer SetLogLevel(oldLevel)

	Error("test error %s", "msg")

	if stderrBuf.Len() == 0 {
		t.Fatal("Error() did not write to outputErrorWriter")
	}
	if !strings.Contains(stderrBuf.String(), "test error msg") {
		t.Errorf("Error() stderr output missing message, got: %q", stderrBuf.String())
	}
	if strings.Contains(stdoutBuf.String(), "test error msg") {
		t.Error("Error() leaked message to stdout instead of stderr")
	}
}
