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

	// 测试Info（始终显示）
	buf.Reset()
	Info("测试Info")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Info") {
		t.Errorf("Info输出错误: got %s, want contains %s", output, "测试Info")
	}

	// 测试Warn（始终显示）
	buf.Reset()
	Warn("测试Warn")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Warn") {
		t.Errorf("Warn输出错误: got %s, want contains %s", output, "测试Warn")
	}

	// 测试Error（始终显示）
	buf.Reset()
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

	// 测试4: Debug在verbose=false时不输出
	SetVerbose(false)
	stdoutBuf.Reset()
	stderrBuf.Reset()
	Debug("这个Debug不应该输出")

	stdoutOutput = strings.TrimSpace(stdoutBuf.String())
	stderrOutput = strings.TrimSpace(stderrBuf.String())

	if stdoutOutput != "" {
		t.Errorf("Debug在verbose=false时不应该输出，但stdout有内容: %s", stdoutOutput)
	}
	if stderrOutput != "" {
		t.Errorf("Debug在verbose=false时不应该输出，但stderr有内容: %s", stderrOutput)
	}

	// 测试5: Debug在verbose=true时输出
	SetVerbose(true)
	stdoutBuf.Reset()
	stderrBuf.Reset()
	Debug("这个Debug应该输出")

	stdoutOutput = strings.TrimSpace(stdoutBuf.String())
	if !strings.Contains(stdoutOutput, "这个Debug应该输出") {
		t.Errorf("Debug在verbose=true时应该输出，但stdout为空")
	}

	t.Log("✅ 错误输出writer测试通过")
}

// TestVerboseMode 测试verbose模式
func TestVerboseMode(t *testing.T) {
	var buf bytes.Buffer
	oldWriter := outputWriter
	SetOutputWriter(&buf)
	defer SetOutputWriter(oldWriter)

	// 测试verbose=false时Debug不输出
	SetVerbose(false)
	buf.Reset()
	Debug("测试Debug1")
	output := strings.TrimSpace(buf.String())
	if output != "" {
		t.Errorf("verbose=false时Debug不应该输出，但输出: %s", output)
	}

	// 测试verbose=true时Debug输出
	SetVerbose(true)
	buf.Reset()
	Debug("测试Debug2")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Debug2") {
		t.Errorf("verbose=true时Debug应该输出，但输出为空")
	}

	// 测试Info始终输出（无论verbose状态）
	SetVerbose(false)
	buf.Reset()
	Info("测试Info")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Info") {
		t.Errorf("Info应该始终输出，但输出为空")
	}

	SetVerbose(true)
	buf.Reset()
	Info("测试Info2")
	output = strings.TrimSpace(buf.String())
	if !strings.Contains(output, "测试Info2") {
		t.Errorf("Info应该始终输出，但输出为空")
	}
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

// TestDebugBytes 测试DebugBytes函数
func TestDebugBytes(t *testing.T) {
	var buf bytes.Buffer
	oldWriter := outputWriter
	SetOutputWriter(&buf)
	defer SetOutputWriter(oldWriter)

	// 测试verbose=false时DebugBytes不输出
	SetVerbose(false)
	buf.Reset()
	DebugBytes("json", []byte(`{"key": "value"}`))
	output := buf.String()
	if output != "" {
		t.Errorf("verbose=false时DebugBytes不应该输出，但输出: %s", output)
	}

	// 测试verbose=true时DebugBytes输出
	SetVerbose(true)
	buf.Reset()
	DebugBytes("json", []byte(`{"key": "value"}`))
	output = buf.String()
	expected := "```json\n{\"key\": \"value\"}\n```\n"
	if output != expected {
		t.Errorf("DebugBytes输出不匹配:\n期望: %q\n实际: %q", expected, output)
	}
}

// TestJSONMarshal 测试JSONMarshal函数
func TestJSONMarshal(t *testing.T) {
	data := map[string]string{"key": "value"}

	// 测试verbose=false时紧凑格式
	SetVerbose(false)
	compactJSON, err := JSONMarshal(data)
	if err != nil {
		t.Fatalf("JSONMarshal失败: %v", err)
	}
	expectedCompact := `{"key":"value"}`
	if string(compactJSON) != expectedCompact {
		t.Errorf("verbose=false时JSON应该紧凑:\n期望: %s\n实际: %s", expectedCompact, string(compactJSON))
	}

	// 测试verbose=true时格式化格式
	SetVerbose(true)
	formattedJSON, err := JSONMarshal(data)
	if err != nil {
		t.Fatalf("JSONMarshal失败: %v", err)
	}
	expectedFormatted := "{\n  \"key\": \"value\"\n}"
	if string(formattedJSON) != expectedFormatted {
		t.Errorf("verbose=true时JSON应该格式化:\n期望: %s\n实际: %s", expectedFormatted, string(formattedJSON))
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name   string // description of this test case
		s      string
		maxLen int
		want   string
	}{
		{"Empty", "", 5, ""},
		{"Short", "hello world", 12, "hello world"},
		{"Pure English", "hello world", 11, "hello world"},
		{"English Truncate", "hello world", 10, "hello w..."},
		{"Chinese", "世界，你好", 5, "世界，你好"},
		{"Chinese Truncate", "世界，你好！", 4, "世..."},
		{"MaxLen less than 3", "hello", 2, ""},
		{"MaxLen zero", "hello", 0, ""},
		{"MaxLen negative", "hello", -1, ""},
		{"MaxLen exactly 3", "hello", 3, "..."},
		{"MaxLen 2 with Chinese", "你好", 2, ""},
		{"MaxLen 4 with Chinese", "你好世界", 4, "你..."},
		{"Emoji", "Hello 😊 World", 8, "Hello ..."},
		{"Emoji truncate", "Hello 😊 World", 7, "He..."},
		{"Long string", strings.Repeat("a", 100), 50, strings.Repeat("a", 47) + "..."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := TruncateString(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("TruncateString(%q, %d) = %q, want %q",
					tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}
