package main

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// TestChatCommandArgs 测试chat命令的参数输入功能
func TestChatCommandArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		input       string
		expectedErr bool
		expectedOut string
	}{
		{
			name:        "直接参数输入",
			args:        []string{"chat", "你好，世界"},
			expectedErr: false,
			expectedOut: "", // 实际输出需要模拟API响应
		},
		{
			name:        "使用--model参数",
			args:        []string{"chat", "--model", "deepseek-chat", "测试模型参数"},
			expectedErr: false,
			expectedOut: "",
		},
		{
			name:        "使用--histsize参数",
			args:        []string{"chat", "--histsize", "5", "测试历史大小"},
			expectedErr: false,
			expectedOut: "",
		},
		{
			name:        "使用--input参数从文件读取",
			args:        []string{"chat", "--input", "test_input.txt"},
			expectedErr: false,
			expectedOut: "",
		},
		{
			name:        "使用--input - 从标准输入读取",
			args:        []string{"chat", "--input", "-"},
			input:       "这是从标准输入读取的内容",
			expectedErr: false,
			expectedOut: "",
		},
		{
			name:        "无效模型参数",
			args:        []string{"chat", "--model", "invalid-model", "测试无效模型"},
			expectedErr: true,
			expectedOut: "",
		},
		{
			name:        "空参数",
			args:        []string{"chat"},
			input:       "这是通过标准输入传递的内容",
			expectedErr: false,
			expectedOut: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 保存原始标准输入
			oldStdin := os.Stdin
			defer func() { os.Stdin = oldStdin }()

			// 如果测试需要输入，设置模拟标准输入
			if tt.input != "" {
				r := strings.NewReader(tt.input)
				os.Stdin = r
			}

			// 创建命令
			cmd := rootCmd
			cmd.SetArgs(tt.args)

			// 捕获输出
			var outBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&outBuf)

			// 执行命令
			err := cmd.Execute()

			// 检查错误
			if tt.expectedErr && err == nil {
				t.Errorf("%s: 期望错误但未发生", tt.name)
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("%s: 不期望错误但发生了: %v", tt.name, err)
			}

			// 检查输出（如果有期望的输出）
			if tt.expectedOut != "" {
				output := outBuf.String()
				if !strings.Contains(output, tt.expectedOut) {
					t.Errorf("%s: 输出不包含期望的内容\n期望: %s\n实际: %s",
						tt.name, tt.expectedOut, output)
				}
			}
		})
	}
}

// TestReadContentWithTimeout 测试ReadContentWithTimeout函数
func TestReadContentWithTimeout(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		timeout     time.Duration
		expected    string
		expectError bool
	}{
		{
			name:        "正常读取",
			input:       "测试内容",
			timeout:     5 * time.Second,
			expected:    "测试内容",
			expectError: false,
		},
		{
			name:        "读取空内容",
			input:       "",
			timeout:     5 * time.Second,
			expected:    "",
			expectError: false,
		},
		{
			name:        "读取带换行的内容",
			input:       "第一行\n第二行\n第三行",
			timeout:     5 * time.Second,
			expected:    "第一行\n第二行\n第三行",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 保存原始标准输入
			oldStdin := os.Stdin
			defer func() { os.Stdin = oldStdin }()

			// 设置模拟标准输入
			if tt.input != "" {
				r := strings.NewReader(tt.input)
				os.Stdin = r
			}

			ctx := context.Background()
			ctx = context.WithValue(ctx, InputContent, "")

			// 执行测试
			result, err := ReadContentWithTimeout(ctx)

			// 检查错误
			if tt.expectError && err == nil {
				t.Errorf("%s: 期望错误但未发生", tt.name)
			}
			if !tt.expectError && err != nil {
				t.Errorf("%s: 不期望错误但发生了: %v", tt.name, err)
			}

			// 检查结果
			if !tt.expectError && result != tt.expected {
				t.Errorf("%s: 结果不匹配\n期望: %q\n实际: %q",
					tt.name, tt.expected, result)
			}
		})
	}
}

// TestChatPreRunE 测试ChatPreRunE函数
func TestChatPreRunE(t *testing.T) {
	tests := []struct {
		name        string
		model       string
		expectedErr bool
	}{
		{
			name:        "默认模型",
			model:       "",
			expectedErr: false,
		},
		{
			name:        "deepseek-chat模型",
			model:       "deepseek-chat",
			expectedErr: false,
		},
		{
			name:        "deepseek-reasoner模型",
			model:       "deepseek-reasoner",
			expectedErr: false,
		},
		{
			name:        "无效模型",
			model:       "invalid-model",
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建模拟命令
			cmd := &cobra.Command{}
			cmd.Flags().String("model", "", "模型名称")
			cmd.Flags().Bool("verbose", false, "详细输出")

			// 设置模型参数
			if tt.model != "" {
				cmd.Flags().Set("model", tt.model)
			}

			// 执行测试
			err := ChatPreRunE(cmd, []string{})

			// 检查错误
			if tt.expectedErr && err == nil {
				t.Errorf("%s: 期望错误但未发生", tt.name)
			}
			if !tt.expectedErr && err != nil {
				t.Errorf("%s: 不期望错误但发生了: %v", tt.name, err)
			}
		})
	}
}

// TestRootCommandFlags 测试根命令的标志
func TestRootCommandFlags(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedErr bool
	}{
		{
			name:        "正常模式参数",
			args:        []string{"--mode", "markdown", "chat", "测试"},
			expectedErr: false,
		},
		{
			name:        "org模式参数",
			args:        []string{"--mode", "org", "chat", "测试"},
			expectedErr: false,
		},
		{
			name:        "无效模式参数",
			args:        []string{"--mode", "invalid", "chat", "测试"},
			expectedErr: true,
		},
		{
			name:        "verbose参数",
			args:        []string{"--verbose", "chat", "测试"},
			expectedErr: false,
		},
		{
			name:        "no-color参数",
			args:        []string{"--no-color", "chat", "测试"},
			expectedErr: false,
		},
		{
			name:        "no-timestamp参数",
			args:        []string{"--no-timestamp", "chat", "测试"},
			expectedErr: false,
		},
		{
			name:        "db参数",
			args:        []string{"--db", "/tmp/test.db", "chat", "测试"},
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 创建命令
			cmd := rootCmd
			cmd.SetArgs(tt.args)

			// 捕获输出
			var outBuf bytes.Buffer
			cmd.SetOut(&outBuf)
			cmd.SetErr(&outBuf)

			// 执行命令
			err := cmd.Execute()

			// 检查错误
			if tt.expectedErr && err == nil {
				t.Errorf("%s: 期望错误但未发生", tt.name)
			}
			if !tt.expectedErr && err != nil {
				// 忽略API密钥错误，因为这是测试环境
				if !strings.Contains(err.Error(), "no api key specified") {
					t.Errorf("%s: 不期望错误但发生了: %v", tt.name, err)
				}
			}
		})
	}
}
