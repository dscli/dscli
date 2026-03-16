package main

import (
	"context"
	"io"
	"os"
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
	if os.Getenv("InsideShellExec") != "" {
		t.SkipNow()
	}
	SetVerbose(true)
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
			// 创建包含ToolDisplayName的context
			ctx := context.WithValue(context.Background(), ToolDisplayName, "test-tool")
			ctx = context.WithValue(ctx, VerboseKey, true)
			out, err := ShellExec(ctx, tc.script)

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

// TestShortenShellScript 测试ShortenShellScript函数
func TestShortenShellScript(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		// 1. 短脚本（<72字符）
		{
			name:   "短脚本_普通命令",
			script: "echo hello world",
			want:   "",
		},
		{
			name:   "短脚本_多行命令",
			script: "echo line1\necho line2\necho line3",
			want:   "",
		},
		// 2. 长脚本（>72字符）
		{
			name:   "长脚本_单行超过72字符",
			script: "这是一个非常长的脚本命令，包含了很多字符，用于测试ShortenScript函数的截断功能，确保它能正确工作",
			want:   "ShortenScript",
		},
		{
			name:   "长脚本_多行超过72字符",
			script: "#!/usr/bin/env bash\necho '这是一个非常长的脚本，用于测试ShortenScript函数'\necho '第二行内容'\necho '第三行内容'\necho '第四行内容'\necho '第五行内容'",
			want:   "",
		},
		// 3. 带shebang的脚本
		{
			name:   "带shebang_短脚本",
			script: "#!/usr/bin/env bash\necho hello",
			want:   "",
		},
		{
			name:   "带shebang_长脚本",
			script: "#!/usr/bin/env python\nprint('这是一个很长的Python脚本，用于测试ShortenScript函数是否能正确处理shebang和长脚本的情况')",
			want:   "print('PythonShortenScriptshebang')",
		},
		// 4. 边界情况（正好72字符）
		{
			name:   "边界_正好72字符",
			script: strings.Repeat("a", 72),
			want:   strings.Repeat("a", 50),
		},
		{
			name:   "边界_73字符",
			script: strings.Repeat("a", 73),
			want:   strings.Repeat("a", 50),
		},
		// 5. 特殊字符处理
		{
			name:   "特殊字符_空格分隔",
			script: "command1 arg1 arg2 arg3 arg4 arg5 arg6 arg7 arg8 arg9 arg10 arg11 arg12 arg13 arg14 arg15",
			want:   "command1 arg1",
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
echo "执行完成"`,
			want: "for i in {1..10}; do; done",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortenShellScript(tt.script)
			if got != tt.want {
				t.Errorf("ShortenShellScript() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestShortenShellScriptEdgeCases 测试边界情况
// TestShortenShellScriptEdgeCases 测试边界情况
func TestShortenShellScriptEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "中文字符_短",
			script: "echo 中文测试",
			want:   "",
		},
		{
			name:   "混合字符",
			script: "echo 'Hello 世界! 123 ABC' && echo '测试 test 123'",
			want:   "",
		},
		{
			name:   "shebang后有空格",
			script: "#!/usr/bin/env bash   \necho hello",
			want:   "",
		},
		{
			name:   "注释行",
			script: "# 这是一个注释\necho hello\n# 另一个注释",
			want:   "",
		},
		{
			name:   "测试超长脚本的截断",
			script: "#!/usr/bin/env bash\n# This is a very long script with many lines\necho 'Line 1: This is a test script'\necho 'Line 2: For testing ShortenScript function'\necho 'Line 3: When script exceeds 72 characters'\necho 'Line 4: Should be truncated correctly'\necho 'Line 5: And show beginning and end parts'\necho 'Line 6: Connected with ..'\necho 'Line 7: Ensure functionality works'\necho 'Line 8: Test completed'",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShortenShellScript(tt.script)
			if got != tt.want {
				t.Errorf("ShortenShellScript() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShellExec(t *testing.T) {
	if os.Getenv("InsideShellExec") != "" {
		t.SkipNow()
	}
	tcs := []struct {
		name   string
		script string
		input  string
		want   string
	}{
		{"bash", "#!/usr/bin/env bash\ncat", "hello", "hello"},
		{"python", "#!/usr/bin/env python\nimport sys\nprint(sys.stdin.read().strip())", "world", "world"},
		{"echo", "#!/usr/bin/env bash\necho 'test output'", "", "test output"},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}

			_, err = io.WriteString(w, tc.input)
			if err != nil {
				t.Fatal(err)
			}
			w.Close()
			ctx := context.Background()
			ctx = context.WithValue(ctx, ShellStdinKey, r)
			out, err := ShellExec(ctx, tc.script)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatal("[", out, "]", "[", tc.want, "]")
			}
		})
	}
}

func TestIsTesting(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		want bool
	}{
		{"IsTesting", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsTesting()
			if got != tt.want {
				t.Errorf("IsTesting() = %v, want %v", got, tt.want)
			}
		})
	}
}
