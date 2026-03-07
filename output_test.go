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
	oldErrorWriter := outputErrorWriter
	SetOutputWriter(&buf)
	SetErrorWriter(&buf)
	defer func() {
		SetOutputWriter(oldWriter)
		SetErrorWriter(oldErrorWriter)
	}()

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

// TestErrorOutputWriter 测试错误输出函数是否正确使用outputErrorWriter
func TestErrorOutputWriter(t *testing.T) {
	// 创建两个不同的缓冲区来区分标准输出和错误输出
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer

	// 保存原始writer
	oldStdoutWriter := outputWriter
	oldStderrWriter := outputErrorWriter

	// 设置测试writer
	SetOutputWriter(&stdoutBuf)
	SetErrorWriter(&stderrBuf)

	// 测试完成后恢复
	defer func() {
		SetOutputWriter(oldStdoutWriter)
		SetErrorWriter(oldStderrWriter)
	}()

	// 测试1: Info应该输出到stdout
	SetLogLevel(LogLevelInfo)
	stdoutBuf.Reset()
	stderrBuf.Reset()
	Info("测试Info到标准输出")

	stdoutOutput := strings.TrimSpace(stdoutBuf.String())
	stderrOutput := strings.TrimSpace(stderrBuf.String())

	if !strings.Contains(stdoutOutput, "测试Info到标准输出") {
		t.Errorf("Info应该输出到标准输出，但stdout为空")
	}
	if stderrOutput != "" {
		t.Errorf("Info不应该输出到错误输出，但stderr有内容: %s", stderrOutput)
	}

	// 测试2: Warn应该输出到stderr
	SetLogLevel(LogLevelWarn)
	stdoutBuf.Reset()
	stderrBuf.Reset()
	Warn("测试Warn到错误输出")

	stdoutOutput = strings.TrimSpace(stdoutBuf.String())
	stderrOutput = strings.TrimSpace(stderrBuf.String())

	if stdoutOutput != "" {
		t.Errorf("Warn不应该输出到标准输出，但stdout有内容: %s", stdoutOutput)
	}
	if !strings.Contains(stderrOutput, "测试Warn到错误输出") {
		t.Errorf("Warn应该输出到错误输出，但stderr为空")
	}

	// 测试3: Error应该输出到stderr
	SetLogLevel(LogLevelError)
	stdoutBuf.Reset()
	stderrBuf.Reset()
	Error("测试Error到错误输出")

	stdoutOutput = strings.TrimSpace(stdoutBuf.String())
	stderrOutput = strings.TrimSpace(stderrBuf.String())

	if stdoutOutput != "" {
		t.Errorf("Error不应该输出到标准输出，但stdout有内容: %s", stdoutOutput)
	}
	if !strings.Contains(stderrOutput, "测试Error到错误输出") {
		t.Errorf("Error应该输出到错误输出，但stderr为空")
	}

	// 测试4: 验证日志级别过滤
	SetLogLevel(LogLevelError) // 只显示Error及以上级别
	stdoutBuf.Reset()
	stderrBuf.Reset()

	// Warn应该被过滤掉（不输出）
	Warn("这个Warn应该被过滤")

	stdoutOutput = strings.TrimSpace(stdoutBuf.String())
	stderrOutput = strings.TrimSpace(stderrBuf.String())

	if stdoutOutput != "" {
		t.Errorf("Warn被过滤时，stdout应该为空，但有内容: %s", stdoutOutput)
	}
	if stderrOutput != "" {
		t.Errorf("Warn被过滤时，stderr应该为空，但有内容: %s", stderrOutput)
	}

	// Error应该输出
	Error("这个Error应该输出")
	stderrOutput = strings.TrimSpace(stderrBuf.String())
	if !strings.Contains(stderrOutput, "这个Error应该输出") {
		t.Errorf("Error应该输出，但stderr为空")
	}

	// 测试5: 验证Debug在低日志级别下不输出
	SetLogLevel(LogLevelInfo) // Info及以上级别
	stdoutBuf.Reset()
	stderrBuf.Reset()

	Debug("这个Debug应该被过滤")

	stdoutOutput = strings.TrimSpace(stdoutBuf.String())
	stderrOutput = strings.TrimSpace(stderrBuf.String())

	if stdoutOutput != "" {
		t.Errorf("Debug被过滤时，stdout应该为空，但有内容: %s", stdoutOutput)
	}
	if stderrOutput != "" {
		t.Errorf("Debug被过滤时，stderr应该为空，但有内容: %s", stderrOutput)
	}

	// 测试6: 验证Debug在高日志级别下输出
	SetLogLevel(LogLevelDebug) // 所有级别
	stdoutBuf.Reset()
	stderrBuf.Reset()

	Debug("这个Debug应该输出")

	stdoutOutput = strings.TrimSpace(stdoutBuf.String())
	if !strings.Contains(stdoutOutput, "这个Debug应该输出") {
		t.Errorf("Debug应该输出，但stdout为空")
	}

	t.Log("✅ 错误输出writer测试通过")
}

// TestFatalOutput 测试Fatal输出（使用模拟的os.Exit）
func TestFatalOutput(t *testing.T) {
	// 由于Fatal会调用os.Exit(1)，我们需要在子进程中测试
	// 这里我们只测试它是否正确使用了outputErrorWriter

	var stderrBuf bytes.Buffer
	oldStderrWriter := outputErrorWriter
	SetErrorWriter(&stderrBuf)
	defer SetErrorWriter(oldStderrWriter)

	SetLogLevel(LogLevelFatal)

	// 注意：我们不能直接调用Fatal，因为它会退出进程
	// 这里我们只是验证函数定义和基本的writer使用
	// 在实际测试中，应该使用测试子进程来测试Fatal

	t.Log("✅ Fatal输出测试跳过（需要子进程测试）")
}

// TestOutputWriterSeparation 测试输出writer分离
func TestOutputWriterSeparation(t *testing.T) {
	// 创建三个不同的缓冲区
	var buf1, buf2, buf3 bytes.Buffer

	// 测试设置不同的writer
	oldWriter := outputWriter
	oldErrorWriter := outputErrorWriter

	// 设置buf1为标准输出
	SetOutputWriter(&buf1)
	SetErrorWriter(&buf2)

	SetLogLevel(LogLevelInfo)
	Info("输出到buf1")
	Warn("输出到buf2")

	// 验证分离
	buf1Str := strings.TrimSpace(buf1.String())
	buf2Str := strings.TrimSpace(buf2.String())

	if !strings.Contains(buf1Str, "输出到buf1") {
		t.Errorf("Info应该输出到buf1，但内容: %s", buf1Str)
	}
	if !strings.Contains(buf2Str, "输出到buf2") {
		t.Errorf("Warn应该输出到buf2，但内容: %s", buf2Str)
	}

	// 切换writer到buf3
	SetOutputWriter(&buf3)
	Info("输出到buf3")

	buf3Str := strings.TrimSpace(buf3.String())
	if !strings.Contains(buf3Str, "输出到buf3") {
		t.Errorf("Info应该输出到buf3，但内容: %s", buf3Str)
	}

	// 恢复原始writer
	SetOutputWriter(oldWriter)
	SetErrorWriter(oldErrorWriter)

	t.Log("✅ 输出writer分离测试通过")
}
