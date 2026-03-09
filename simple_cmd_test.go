package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// TestCommandLineArgs 测试命令行参数解析
func TestCommandLineArgs(t *testing.T) {
	// 测试根命令参数
	t.Run("RootCommandFlags", func(t *testing.T) {
		tests := []struct {
			name        string
			args        []string
			expectError bool
			errorMsg    string
		}{
			{
				name:        "正常模式参数",
				args:        []string{"--mode", "markdown"},
				expectError: false,
			},
			{
				name:        "org模式参数",
				args:        []string{"--mode", "org"},
				expectError: false,
			},
			{
				name:        "无效模式参数",
				args:        []string{"--mode", "invalid"},
				expectError: true,
				errorMsg:    "do not support invalid",
			},
			{
				name:        "verbose参数",
				args:        []string{"--verbose"},
				expectError: false,
			},
			{
				name:        "no-color参数",
				args:        []string{"--no-color"},
				expectError: false,
			},
			{
				name:        "no-timestamp参数",
				args:        []string{"--no-timestamp"},
				expectError: false,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cmd := rootCmd
				cmd.SetArgs(append(tt.args, "chat", "测试"))

				var outBuf bytes.Buffer
				cmd.SetOut(&outBuf)
				cmd.SetErr(&outBuf)

				err := cmd.Execute()

				if tt.expectError {
					if err == nil {
						t.Errorf("期望错误但未发生")
					} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
						t.Errorf("错误消息不匹配\n期望包含: %s\n实际: %v", tt.errorMsg, err)
					}
				} else {
					// 忽略API密钥错误，因为这是测试环境
					if err != nil && !strings.Contains(err.Error(), "no api key specified") {
						t.Errorf("不期望错误但发生了: %v", err)
					}
				}
			})
		}
	})

	// 测试chat命令参数
	t.Run("ChatCommandFlags", func(t *testing.T) {
		tests := []struct {
			name        string
			args        []string
			expectError bool
			errorMsg    string
		}{
			{
				name:        "直接参数输入",
				args:        []string{"chat", "你好，世界"},
				expectError: false,
			},
			{
				name:        "使用model参数",
				args:        []string{"chat", "--model", "deepseek-chat", "测试"},
				expectError: false,
			},
			{
				name:        "使用histsize参数",
				args:        []string{"chat", "--histsize", "5", "测试"},
				expectError: false,
			},
			{
				name:        "无效模型参数",
				args:        []string{"chat", "--model", "invalid-model", "测试"},
				expectError: true,
				errorMsg:    "do not support invalid-model",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cmd := rootCmd
				cmd.SetArgs(tt.args)

				var outBuf bytes.Buffer
				cmd.SetOut(&outBuf)
				cmd.SetErr(&outBuf)

				err := cmd.Execute()

				if tt.expectError {
					if err == nil {
						t.Errorf("期望错误但未发生")
					} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
						t.Errorf("错误消息不匹配\n期望包含: %s\n实际: %v", tt.errorMsg, err)
					}
				} else {
					// 忽略API密钥错误，因为这是测试环境
					if err != nil && !strings.Contains(err.Error(), "no api key specified") {
						t.Errorf("不期望错误但发生了: %v", err)
					}
				}
			})
		}
	})
}

// TestFlagParsing 测试标志解析
func TestFlagParsing(t *testing.T) {
	// 创建一个简单的命令来测试标志解析
	cmd := &cobra.Command{
		Use: "test",
		Run: func(cmd *cobra.Command, args []string) {
			// 什么都不做
		},
	}

	var testFlag string
	var testBool bool
	var testInt int

	cmd.Flags().StringVar(&testFlag, "test-flag", "default", "测试标志")
	cmd.Flags().BoolVar(&testBool, "test-bool", false, "测试布尔标志")
	cmd.Flags().IntVar(&testInt, "test-int", 0, "测试整数标志")

	tests := []struct {
		name     string
		args     []string
		expected struct {
			flag string
			bool bool
			int  int
			args []string
		}
	}{
		{
			name: "默认值",
			args: []string{},
			expected: struct {
				flag string
				bool bool
				int  int
				args []string
			}{
				flag: "default",
				bool: false,
				int:  0,
				args: []string{},
			},
		},
		{
			name: "设置字符串标志",
			args: []string{"--test-flag", "custom-value", "arg1", "arg2"},
			expected: struct {
				flag string
				bool bool
				int  int
				args []string
			}{
				flag: "custom-value",
				bool: false,
				int:  0,
				args: []string{"arg1", "arg2"},
			},
		},
		{
			name: "设置布尔标志",
			args: []string{"--test-bool", "arg1"},
			expected: struct {
				flag string
				bool bool
				int  int
				args []string
			}{
				flag: "default",
				bool: true,
				int:  0,
				args: []string{"arg1"},
			},
		},
		{
			name: "设置整数标志",
			args: []string{"--test-int", "42", "arg1"},
			expected: struct {
				flag string
				bool bool
				int  int
				args []string
			}{
				flag: "default",
				bool: false,
				int:  42,
				args: []string{"arg1"},
			},
		},
		{
			name: "设置所有标志",
			args: []string{"--test-flag", "value", "--test-bool", "--test-int", "99", "arg1", "arg2"},
			expected: struct {
				flag string
				bool bool
				int  int
				args []string
			}{
				flag: "value",
				bool: true,
				int:  99,
				args: []string{"arg1", "arg2"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 重置标志值
			testFlag = "default"
			testBool = false
			testInt = 0

			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			if err != nil {
				t.Fatalf("命令执行失败: %v", err)
			}

			// 检查标志值
			if testFlag != tt.expected.flag {
				t.Errorf("testFlag 不匹配: 期望 %q, 实际 %q", tt.expected.flag, testFlag)
			}
			if testBool != tt.expected.bool {
				t.Errorf("testBool 不匹配: 期望 %v, 实际 %v", tt.expected.bool, testBool)
			}
			if testInt != tt.expected.int {
				t.Errorf("testInt 不匹配: 期望 %d, 实际 %d", tt.expected.int, testInt)
			}
		})
	}
}

// TestStdInReading 测试标准输入读取
func TestStdInReading(t *testing.T) {
	// 保存原始标准输入
	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "简单文本",
			input:    "Hello, World!",
			expected: "Hello, World!",
		},
		{
			name:     "多行文本",
			input:    "Line 1\nLine 2\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "空输入",
			input:    "",
			expected: "",
		},
		{
			name:     "带空格和制表符",
			input:    "  Hello  \tWorld  ",
			expected: "  Hello  \tWorld  ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建临时文件作为标准输入
			tmpfile, err := os.CreateTemp("", "test-stdin-*.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpfile.Name())

			// 写入测试数据
			if _, err := tmpfile.Write([]byte(tt.input)); err != nil {
				t.Fatal(err)
			}
			if err := tmpfile.Close(); err != nil {
				t.Fatal(err)
			}

			// 重新打开文件用于读取
			tmpfile, err = os.Open(tmpfile.Name())
			if err != nil {
				t.Fatal(err)
			}
			defer tmpfile.Close()

			// 设置标准输入
			os.Stdin = tmpfile

			// 读取内容
			content, err := readFromStdin()
			if err != nil {
				t.Fatalf("读取标准输入失败: %v", err)
			}

			// 检查结果
			if content != tt.expected {
				t.Errorf("内容不匹配\n期望: %q\n实际: %q", tt.expected, content)
			}
		})
	}
}

// readFromStdin 辅助函数：从标准输入读取内容
func readFromStdin() (string, error) {
	data, err := os.ReadFile("/dev/stdin")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
