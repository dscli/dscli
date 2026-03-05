package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestNoticeMessageLength(t *testing.T) {
	// 测试Notice消息长度
	testCases := []struct {
		name     string
		delay    int
		expected string
	}{
		{
			name:     "1秒延迟",
			delay:    1,
			expected: "网络异常，1秒后重试...",
		},
		{
			name:     "60秒延迟",
			delay:    60,
			expected: "网络异常，60秒后重试...",
		},
		{
			name:     "300秒延迟",
			delay:    300,
			expected: "网络异常，300秒后重试...",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 捕获输出
			var buf bytes.Buffer
			oldWriter := outputWriter
			SetOutputWriter(&buf)
			defer SetOutputWriter(oldWriter)

			// 调用Notice
			Notice("网络异常，%d秒后重试...", tc.delay)

			// 获取输出（去掉颜色代码）
			output := buf.String()
			// 移除颜色代码和前缀
			output = strings.TrimSpace(output)
			// 移除颜色代码
			output = removeColorCodes(output)
			// 移除Notice前缀 "→ "
			output = strings.TrimPrefix(output, "→ ")

			// 检查长度是否在20字以内
			chineseCharCount := len([]rune(output))
			if chineseCharCount > 20 {
				t.Errorf("Notice消息超过20字: %s (长度: %d)", output, chineseCharCount)
			}

			// 验证内容
			if output != tc.expected {
				t.Errorf("Notice消息不匹配: got %s, want %s", output, tc.expected)
			}
		})
	}
}

func TestSuccessMessageLength(t *testing.T) {
	// 测试Success消息长度
	var buf bytes.Buffer
	oldWriter := outputWriter
	SetOutputWriter(&buf)
	defer SetOutputWriter(oldWriter)

	// 调用Success
	Success("重试成功！")

	// 获取输出（去掉颜色代码）
	output := buf.String()
	output = strings.TrimSpace(output)
	output = removeColorCodes(output)
	output = strings.TrimPrefix(output, "✓ ")

	// 检查长度是否在20字以内
	chineseCharCount := len([]rune(output))
	if chineseCharCount > 20 {
		t.Errorf("Success消息超过20字: %s (长度: %d)", output, chineseCharCount)
	}

	// 验证内容
	if output != "重试成功！" {
		t.Errorf("Success消息不匹配: got %s, want %s", output, "重试成功！")
	}
}

// removeColorCodes 移除ANSI颜色代码
func removeColorCodes(s string) string {
	// ANSI颜色代码正则表达式简化版本
	// 匹配 \033[ 开头，数字和分号，m结尾
	for {
		start := strings.Index(s, "\033[")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "m")
		if end == -1 {
			break
		}
		s = s[:start] + s[start+end+1:]
	}
	return s
}

func TestOutputFunctions(t *testing.T) {
	// 测试所有输出函数的基本功能
	var buf bytes.Buffer
	oldWriter := outputWriter
	SetOutputWriter(&buf)
	defer SetOutputWriter(oldWriter)

	// 测试Println
	buf.Reset()
	Println("测试Println")
	output := strings.TrimSpace(buf.String())
	if output != "测试Println" {
		t.Errorf("Println输出错误: got %s, want %s", output, "测试Println")
	}

	// 测试Printf
	buf.Reset()
	Printf("测试%s", "Printf")
	output = strings.TrimSpace(buf.String())
	if output != "测试Printf" {
		t.Errorf("Printf输出错误: got %s, want %s", output, "测试Printf")
	}

	// 测试Info
	buf.Reset()
	SetLogLevel(LogLevelInfo)
	Info("测试Info")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Info") {
		t.Errorf("Info输出错误: got %s, want contains %s", output, "测试Info")
	}

	// 测试Warn
	buf.Reset()
	SetLogLevel(LogLevelWarn)
	Warn("测试Warn")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Warn") {
		t.Errorf("Warn输出错误: got %s, want contains %s", output, "测试Warn")
	}

	// 测试Error
	buf.Reset()
	SetLogLevel(LogLevelError)
	Error("测试Error")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Error") {
		t.Errorf("Error输出错误: got %s, want contains %s", output, "测试Error")
	}
}
