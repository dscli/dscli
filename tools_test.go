package main

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestShebang(t *testing.T) {
	tcs := []struct {
		script string
		name   string
		arg    []string
	}{
		{"echo hi", "/usr/bin/env", []string{"bash"}},
		{"#!/usr/bin/env bash\necho hi", "/usr/bin/env", []string{"bash"}},
		{"#!/usr/bin/env python\nprint('hi')", "/usr/bin/env", []string{"python"}},
		{"#!/bin/bash\necho hi", "/bin/bash", []string{}},
		{"# comment\necho hi", "/usr/bin/env", []string{"bash"}},
	}
	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			name, arg := Shebang(tc.script)
			if name != tc.name {
				t.Errorf("name mismatch: want %s, got %s", tc.name, name)
			}
			if !reflect.DeepEqual(arg, tc.arg) {
				t.Errorf("arg mismatch: want %v, got %v", tc.arg, arg)
			}
		})
	}
}

func TestRunScriptShebang(t *testing.T) {
	tcs := []struct {
		script   string
		expected string
		checkErr func(error) bool
	}{
		{"echo -n hi", "hi", nil},
		{"echo -n 'hello world'", "hello world", nil},
		{`#!/usr/bin/env bash
echo -n test`, "test", nil},
		{`#!/usr/bin/env python
print("OK")`, "OK\n", nil},
	}
	for _, tc := range tcs {
		t.Run("", func(t *testing.T) {
			name, arg := Shebang(tc.script)
			// 创建包含ToolDisplayName的context
			ctx := context.WithValue(context.Background(), ToolDisplayName, "test-tool")
			out, err := runScript(ctx, tc.script, name, arg)

			if tc.checkErr == nil {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if out != tc.expected {
					t.Errorf("output mismatch: want %q, got %q", tc.expected, out)
				}
			} else {
				if !tc.checkErr(err) {
					t.Errorf("error mismatch: %v", err)
				}
			}
		})
	}
}

// TestShebangExported 测试导出的Shebang函数
func TestShebangExported(t *testing.T) {
	testCases := []struct {
		name     string
		script   string
		expected string
	}{
		{
			name:     "普通脚本",
			script:   "echo hello",
			expected: "/usr/bin/env",
		},
		{
			name:     "带shebang的bash脚本",
			script:   "#!/usr/bin/env bash\necho hello",
			expected: "/usr/bin/env",
		},
		{
			name:     "带shebang的python脚本",
			script:   "#!/usr/bin/env python\nprint('hello')",
			expected: "/usr/bin/env",
		},
		{
			name:     "直接指定解释器",
			script:   "#!/bin/bash\necho hello",
			expected: "/bin/bash",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			name, _ := Shebang(tc.script)
			if name != tc.expected {
				t.Errorf("Shebang解析错误: 期望=%s, 实际=%s", tc.expected, name)
			}
		})
	}
}

// TestRegisterToolAndGetAllTools 测试工具注册和获取
func TestRegisterToolAndGetAllTools(t *testing.T) {
	// 测试获取工具列表
	tools := GetAllTools()
	if len(tools) == 0 {
		t.Error("GetAllTools应该返回至少一个工具")
	}

	// 检查返回的Tool结构体
	for _, tool := range tools {
		if tool.Type == "" {
			t.Error("工具应该有Type字段")
		}
		if tool.Function.Name == "" {
			t.Error("工具函数应该有名称")
		}
		if tool.Function.Description == "" {
			t.Error("工具函数应该有描述")
		}
	}
}

// TestShuffleExported 测试导出的Shuffle函数
func TestShuffleExported(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string // 期望不同的字符串
	}{
		{
			name:     "短字符串",
			input:    "abc",
			expected: "abc", // 长度相同
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "长字符串",
			input:    "test-string-for-shuffle",
			expected: "test-string-for-shuffle", // 长度相同
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := Shuffle(tc.input)

			// 检查长度是否相同
			if len(result) != len(tc.expected) {
				t.Errorf("长度不匹配: 输入长度=%d, 输出长度=%d",
					len(tc.input), len(result))
			}

			// 检查是否只包含字母字符
			for _, r := range result {
				if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
					t.Errorf("结果包含非字母字符: %c", r)
				}
			}
		})
	}
}

func TestShortenScript(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		// 1. 短脚本（<72字符）
		{
			name:   "短脚本_普通命令",
			script: "echo hello world",
			want:   "echo hello world",
		},
		{
			name:   "短脚本_多行命令",
			script: "echo line1\necho line2\necho line3",
			want:   "echo line1\necho line2\necho line3",
		},
		// 2. 长脚本（>72字符）
		{
			name:   "长脚本_单行超过72字符",
			script: "这是一个非常长的脚本命令，包含了很多字符，用于测试ShortenScript函数的截断功能，确保它能正确工作",
			want:   "这是一个非常长的脚本命令，包含了很多字符，用于测试ShortenScript函数的截断功能，确保它能正确工作",
		},
		{
			name:   "长脚本_多行超过72字符",
			script: "#!/usr/bin/env bash\necho '这是一个非常长的脚本，用于测试ShortenScript函数'\necho '第二行内容'\necho '第三行内容'\necho '第四行内容'\necho '第五行内容'",
			want:   "echo '这是一个非常长的脚本，用于测试ShortenScript函数..ho '第三行内容' echo '第四行内容' echo '第五行内容'",
		},
		// 3. 带shebang的脚本
		{
			name:   "带shebang_短脚本",
			script: "#!/usr/bin/env bash\necho hello",
			want:   "echo hello",
		},
		{
			name:   "带shebang_长脚本",
			script: "#!/usr/bin/env python\nprint('这是一个很长的Python脚本，用于测试ShortenScript函数是否能正确处理shebang和长脚本的情况')",
			want:   "print('这是一个很长的Python脚本，用于测试ShortenScript函数是否能正确处理shebang和长脚本的情况')",
		},
		// 4. 边界情况（正好72字符）
		{
			name:   "边界_正好72字符",
			script: strings.Repeat("a", 72),
			want:   strings.Repeat("a", 72),
		},
		{
			name:   "边界_73字符",
			script: strings.Repeat("a", 73),
			want:   strings.Repeat("a", 36) + ".." + strings.Repeat("a", 36),
		},
		// 5. 特殊字符处理
		{
			name:   "特殊字符_空格分隔",
			script: "command1 arg1 arg2 arg3 arg4 arg5 arg6 arg7 arg8 arg9 arg10 arg11 arg12 arg13 arg14 arg15",
			want:   "command1 arg1 arg2 arg3 arg4 arg5 ar..arg10 arg11 arg12 arg13 arg14 arg15",
		},
		// 6. 空脚本
		{
			name:   "空脚本",
			script: "",
			want:   "",
		},
		// 7. 只有shebang
		{
			name:   "只有shebang",
			script: "#!/usr/bin/env bash",
			want:   "",
		},
		// 8. 复杂脚本
		{
			name: "复杂脚本_长命令",
			script: `#!/usr/bin/env bash
# 这是一个复杂的脚本
echo "开始执行"
for i in {1..10}; do
  echo "循环 $i"
done
echo "执行完成"
# 结束脚本`,
			want: `# 这是一个复杂的脚本 echo "开始执行" for i in {1...echo "循环 $i" done echo "执行完成" # 结束脚本`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortenScript(tt.script)
			if got != tt.want {
				t.Errorf("ShortenScript() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShortenScriptEdgeCases 测试边界情况
func TestShortenScriptEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "中文字符_短",
			script: "echo 中文测试",
			want:   "echo 中文测试",
		},
		{
			name:   "中文字符_长",
			script: "这是一个包含很多中文字符的脚本，用于测试ShortenScript函数对中文字符的处理能力，确保不会出现乱码或截断错误",
			want:   "这是一个包含很多中文字符的脚本，用于测试ShortenScript函数对中文字符的处理能力，确保不会出现乱码或截断错误",
		},
		{
			name:   "混合字符",
			script: "echo 'Hello 世界! 123 ABC' && echo '测试 test 123'",
			want:   "echo 'Hello 世界! 123 ABC' && echo '测试 test 123'",
		},
		{
			name:   "shebang后有空格",
			script: "#!/usr/bin/env bash    \necho hello",
			want:   "echo hello",
		},
		{
			name:   "shebang后无空格",
			script: "#!/usr/bin/env bash\necho hello",
			want:   "echo hello",
		},
		{
			name:   "注释行",
			script: "# 这是一个注释\necho hello\n# 另一个注释",
			want:   "# 这是一个注释\necho hello\n# 另一个注释",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortenScript(tt.script)
			if got != tt.want {
				t.Errorf("ShortenScript() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShortenScriptLongScript 测试超长脚本的截断
func TestShortenScriptLongScript(t *testing.T) {
	// 创建一个超长脚本（超过72个字符）
	longScript := `#!/usr/bin/env bash
# 这是一个非常长的脚本，包含很多行
echo "第一行：这是一个测试脚本"
echo "第二行：用于验证ShortenScript函数"
echo "第三行：当脚本超过72个字符时"
echo "第四行：应该被正确截断"
echo "第五行：并显示前后部分"
echo "第六行：用..连接"
echo "第七行：确保功能正常"
echo "第八行：测试完成"`

	// 计算期望结果
	// 首先移除shebang行
	scriptWithoutShebang := strings.TrimSpace(strings.TrimPrefix(longScript, "#!/usr/bin/env bash"))
	// 转换为rune
	r := []rune(scriptWithoutShebang)
	n := len(r)
	if n > 72 {
		first := strings.Fields(string(r[0:36]))
		last := strings.Fields(string(r[n-36 : n]))
		expected := strings.Join(first, " ") + ".." + strings.Join(last, " ")

		got := ShortenScript(longScript)
		if got != expected {
			t.Errorf("ShortenScript() = %v, want %v", got, expected)
		}
	}
}
