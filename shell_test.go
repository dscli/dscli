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
			want:   "echo hello world",
		},
		{
			name:   "短脚本_多行命令",
			script: "echo line1\necho line2\necho line3",
			want:   "echo line1; echo line2; echo line3",
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
			want:   `echo 'ShortenScript'; echo ''; echo ''; echo ''; e`,
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
			want:   "command1 arg1 arg2 arg3 arg4 arg5 arg6 arg7 arg8 a",
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
			want: `echo ""; for i in {1..10}; do; echo " $i"; done; e`,
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
func TestShortenShellScriptEdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   string
	}{
		{
			name:   "中文字符_短",
			script: "echo 中文测试",
			want:   "echo ",
		},
		{
			name:   "中文字符_长",
			script: "这是一个包含很多中文字符的脚本，用于测试ShortenScript函数对中文字符的处理能力，确保不会出现乱码或截断错误",
			want:   "ShortenScript",
		},
		{
			name:   "混合字符",
			script: "echo 'Hello 世界! 123 ABC' && echo '测试 test 123'",
			want:   "echo 'Hello ! 123 ABC' && echo ' test 123'",
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
			want:   "echo hello",
		},
		{
			name: "测试超长脚本的截断",
			script: `#!/usr/bin/env bash
# 这是一个非常长的脚本，包含很多行
echo "第一行：这是一个测试脚本"
echo "第二行：用于验证ShortenScript函数"
echo "第三行：当脚本超过72个字符时"
echo "第四行：应该被正确截断"
echo "第五行：并显示前后部分"
echo "第六行：用..连接"
echo "第七行：确保功能正常"
echo "第八行：测试完成"`,
			want: `echo ""; echo "ShortenScript"; echo "72"; echo "";`,
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
	goJson := `{
  "content": "package main\nfunc main(){\n\n}",
  "language": "go"
}`
	mdJson := `{
  "content": "# GO Code main.go\n` + "```" + `package main\nfunc main(){\n\n}\n` + "```" + `",
  "language": "markdown"
}`
	tcs := []struct {
		name   string
		script string
		input  string
		want   string
	}{
		{"bash", `#!/usr/bin/env bash
cat`, "hello", "hello"},
		{"go", pythonScript, goJson, "functions"},
		{"markdown", pythonScript, mdJson, `"lineno": 1`},
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
			ctx = context.WithValue(ctx, ShellStdin, r)
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
