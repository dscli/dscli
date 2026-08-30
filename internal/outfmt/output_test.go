package outfmt

import (
	"bytes"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
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

// TestFormatChatHeader 测试头部构建：email 为空时不输出 <> 包裹。
func TestFormatChatHeader(t *testing.T) {
	withEmail := formatChatHeader("🔍", "review·代码审查", "a@b.c", "22:15:31")
	if strings.Contains(withEmail, "<>") {
		t.Errorf("header should not contain empty angle brackets: %q", withEmail)
	}
	if !strings.Contains(withEmail, "a@b.c") {
		t.Errorf("header should contain email: %q", withEmail)
	}

	noEmail := formatChatHeader("🔍", "review·代码审查", "", "22:15:31")
	if strings.Contains(noEmail, "<") || strings.Contains(noEmail, ">") {
		t.Errorf("header without email should not contain angle brackets: %q", noEmail)
	}
	if !strings.Contains(noEmail, "review·代码审查") {
		t.Errorf("header should contain name: %q", noEmail)
	}
}

// withEA sets DefaultCondition.EastAsianWidth for the duration of the test.
// runewidth.StringWidth/Truncate 全部委托 DefaultCondition；包级 EastAsianWidth
// 仅在 init 时同步一次，运行时直接改包级变量无效。
func withEA(t *testing.T, ea bool) {
	t.Helper()
	old := runewidth.DefaultCondition.EastAsianWidth
	t.Cleanup(func() { runewidth.DefaultCondition.EastAsianWidth = old })
	runewidth.DefaultCondition.EastAsianWidth = ea
}

// assertHeaderWidth asserts the rendered header width. exact=true 时要求恰好
// headerLineWidth（不截断场景）；任何情况下都不得超过 headerLineWidth。
func assertHeaderWidth(t *testing.T, got string, exact bool) {
	t.Helper()
	w := runewidth.StringWidth(got)
	if exact && w != headerLineWidth {
		t.Errorf("header width = %d, want %d: %q", w, headerLineWidth, got)
	}
	if w > headerLineWidth {
		t.Errorf("header width %d exceeds %d: %q", w, headerLineWidth, got)
	}
}

// TestFormatChatHeaderWithRole 测试带角色标签的头部构建。
func TestFormatChatHeaderWithRole(t *testing.T) {
	t.Run("with role label", func(t *testing.T) {
		got := formatChatHeaderWithRole("🐦", "玻尔", "bohr@dscli.io", "🏗️ architect·软件架构师", "12:43:10")
		if !strings.Contains(got, "architect·软件架构师") {
			t.Errorf("header should contain role label: %q", got)
		}
		if !strings.Contains(got, "bohr@dscli.io") {
			t.Errorf("header should contain email: %q", got)
		}
		if strings.Contains(got, "<>") {
			t.Errorf("header should not contain empty angle brackets: %q", got)
		}
	})

	t.Run("empty role label matches base", func(t *testing.T) {
		base := formatChatHeader("🐦", "玻尔", "bohr@dscli.io", "12:43:10")
		withEmpty := formatChatHeaderWithRole("🐦", "玻尔", "bohr@dscli.io", "", "12:43:10")
		if base != withEmpty {
			t.Errorf("empty roleLabel should match base:\nbase:  %q\nwith:  %q", base, withEmpty)
		}
	})

	t.Run("exact width in both EA modes", func(t *testing.T) {
		// 不截断场景：整行应恰好对齐 headerLineWidth。
		for _, ea := range []bool{false, true} {
			withEA(t, ea)
			got := formatChatHeaderWithRole("🐦", "玻尔", "bohr@dscli.io", "🏗️ architect·软件架构师", "12:43:10")
			assertHeaderWidth(t, got, true)
		}
	})

	t.Run("width invariant matrix", func(t *testing.T) {
		// 宽度不变量矩阵：长名字、空邮箱、长 roleLabel、双 EA 模式，全部不得超宽。
		longName := strings.Repeat("很长的名字", 20)
		longRole := "🏗️ " + strings.Repeat("超长角色标签", 6)
		inputs := []struct {
			name      string
			cn        string
			email     string
			roleLabel string
		}{
			{"long name", longName, "bohr@dscli.io", "🏗️ architect·软件架构师"},
			{"empty email", "玻尔", "", "🏗️ architect·软件架构师"},
			{"long role label", "玻尔", "bohr@dscli.io", longRole},
			{"long name empty email", longName, "", "🏗️ architect·软件架构师"},
		}
		for _, ea := range []bool{false, true} {
			for _, in := range inputs {
				withEA(t, ea)
				got := formatChatHeaderWithRole("🐦", in.cn, in.email, in.roleLabel, "12:43:10")
				if w := runewidth.StringWidth(got); w > headerLineWidth {
					t.Errorf("EA=%v %s: header width %d exceeds %d: %q", ea, in.name, w, headerLineWidth, got)
				}
			}
		}
	})

	t.Run("truncation with long identity", func(t *testing.T) {
		longName := strings.Repeat("很长的名字", 20)
		for _, ea := range []bool{false, true} {
			withEA(t, ea)
			got := formatChatHeaderWithRole("🐦", longName, "bohr@dscli.io", "🏗️ architect·软件架构师", "12:43:10")
			// 任一 locale 下整行宽度都不得超过 headerLineWidth（含双宽 · padding）。
			assertHeaderWidth(t, got, false)
			// 截断作用于左侧身份，其末尾应为省略号。
			idx := strings.Index(got, "…")
			if idx < 0 {
				t.Fatalf("EA=%v truncated header should contain ellipsis: %q", ea, got)
			}
			identity := got[:idx+len("…")]
			if !strings.HasSuffix(identity, "…") {
				t.Errorf("EA=%v truncated identity should end with ellipsis: %q", ea, identity)
			}
		}
	})
}
