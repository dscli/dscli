package main

import (
	"context"
	"reflect"
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
				if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '-') {
					t.Errorf("结果包含非字母字符: %c", r)
				}
			}
		})
	}
}
