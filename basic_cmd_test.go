package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"github.com/spf13/cobra"
)

// TestBasicCommandParsing 测试基本的命令行参数解析
func TestBasicCommandParsing(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedMode   string
		expectedVerbose bool
		expectedArgs   []string
		expectError    bool
	}{
		{
			name:           "默认参数",
			args:           []string{"testcmd", "arg1"},
			expectedMode:   "markdown",
			expectedVerbose: false,
			expectedArgs:   []string{"arg1"},
			expectError:    false,
		},
		{
			name:           "设置模式参数",
			args:           []string{"testcmd", "--mode", "org", "arg1", "arg2"},
			expectedMode:   "org",
			expectedVerbose: false,
			expectedArgs:   []string{"arg1", "arg2"},
			expectError:    false,
		},
		{
			name:           "设置详细参数",
			args:           []string{"testcmd", "--verbose", "arg1"},
			expectedMode:   "markdown",
			expectedVerbose: true,
			expectedArgs:   []string{"arg1"},
			expectError:    false,
		},
		{
			name:           "同时设置多个参数",
			args:           []string{"testcmd", "--mode", "custom", "--verbose", "arg1"},
			expectedMode:   "custom",
			expectedVerbose: true,
			expectedArgs:   []string{"arg1"},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 在每个测试中创建新的命令实例
			var testMode string
			var testVerbose bool
			
			testCmd := &cobra.Command{
				Use:   "testcmd",
				Short: "测试命令",
				RunE: func(cmd *cobra.Command, args []string) error {
					// 获取标志值
					mode, err := cmd.Flags().GetString("mode")
					if err != nil {
						return err
					}
					testMode = mode

					verbose, err := cmd.Flags().GetBool("verbose")
					if err != nil {
						return err
					}
					testVerbose = verbose

					fmt.Printf("模式: %s, 详细: %v, 参数: %v\n", mode, verbose, args)
					return nil
				},
			}

			// 添加标志
			testCmd.Flags().String("mode", "markdown", "输出模式")
			testCmd.Flags().Bool("verbose", false, "详细输出")

			// 创建根命令并添加测试命令
			rootCmd := &cobra.Command{Use: "test"}
			rootCmd.AddCommand(testCmd)

			// 设置参数并执行
			rootCmd.SetArgs(tt.args)
			// 检查标志值
			if !tt.expectError {
				if testMode != tt.expectedMode {
					t.Errorf("模式不匹配: 期望 %q, 实际 %q", tt.expectedMode, testMode)
				}
				if testVerbose != tt.expectedVerbose {
					t.Errorf("详细模式不匹配: 期望 %v, 实际 %v", tt.expectedVerbose, testVerbose)
				}
			}
		})
	}
}

// TestArgumentHandling 测试参数处理
func TestArgumentHandling(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedOut string
		expectError bool
	}{
		{
			name:        "简单回显",
			args:        []string{"echo", "hello", "world"},
			expectedOut: "hello world\n",
			expectError: false,
		},
		{
			name:        "大写转换",
			args:        []string{"echo", "--upper", "hello", "world"},
			expectedOut: "HELLO WORLD\n",
			expectError: false,
		},
		{
			name:        "无参数错误",
			args:        []string{"echo"},
			expectedOut: "",
			expectError: true,
		},
		{
			name:        "多个单词",
			args:        []string{"echo", "this", "is", "a", "test"},
			expectedOut: "this is a test\n",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 在每个测试中创建新的命令实例
			var output strings.Builder
			
			cmd := &cobra.Command{
				Use:   "echo",
				Short: "回显参数",
				RunE: func(cmd *cobra.Command, args []string) error {
					if len(args) == 0 {
						return fmt.Errorf("需要至少一个参数")
					}
					
					// 检查是否有特定标志
					upper, _ := cmd.Flags().GetBool("upper")
					
					result := strings.Join(args, " ")
					if upper {
						result = strings.ToUpper(result)
					}
					
					output.WriteString(result + "\n")
					return nil
				},
			}
			
			cmd.Flags().Bool("upper", false, "转换为大写")

			rootCmd := &cobra.Command{Use: "test"}
			rootCmd.AddCommand(cmd)
			
			rootCmd.SetArgs(tt.args)
			
			var outBuf bytes.Buffer
			rootCmd.SetOut(&outBuf)
			rootCmd.SetErr(&outBuf)

			err := rootCmd.Execute()
			
			if tt.expectError {
				if err == nil {
					t.Errorf("期望错误但未发生")
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但发生了: %v", err)
// TestStdInSupport 测试标准输入支持
func TestStdInSupport(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		args        []string
		expectedOut string
		expectError bool
	}{
		{
			name:        "简单输入",
			input:       "Hello, World!\n",
			args:        []string{"readstdin"},
			expectedOut: "Hello, World!\n",
			expectError: false,
		},
		{
			name:        "反转输入",
			input:       "Hello\n",
			args:        []string{"readstdin", "--reverse"},
			expectedOut: "olleH\n",
			expectError: false,
		},
		{
			name:        "多行输入",
			input:       "Line 1\nLine 2\nLine 3\n",
			args:        []string{"readstdin"},
			expectedOut: "Line 1\nLine 2\nLine 3\n",
			expectError: false,
		},
		{
			name:        "空输入",
			input:       "",
			args:        []string{"readstdin"},
			expectedOut: "",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 保存原始标准输入
			oldStdin := os.Stdin
			defer func() { os.Stdin = oldStdin }()

			// 创建临时文件作为标准输入
			tmpfile, err := os.CreateTemp("", "test-input-*.txt")
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

			// 在每个测试中创建新的命令实例
			var output strings.Builder
			
			cmd := &cobra.Command{
				Use:   "readstdin",
				Short: "读取标准输入",
				RunE: func(cmd *cobra.Command, args []string) error {
					// 从标准输入读取
					data, err := io.ReadAll(os.Stdin)
					if err != nil {
						return err
					}
					
					// 检查是否有反转标志
					reverse, _ := cmd.Flags().GetBool("reverse")
					
					content := string(data)
					if reverse {
						// 反转字符串
						runes := []rune(content)
						for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
							runes[i], runes[j] = runes[j], runes[i]
						}
						content = string(runes)
					}
					
					output.WriteString(content)
					return nil
				},
			}
// TestCombinedFlagsAndArgs 测试标志和参数的组合
func TestCombinedFlagsAndArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectedOut string
	}{
		{
			name:        "基本处理",
			args:        []string{"process", "hello", "world"},
			expectedOut: "hello\nworld\n",
		},
		{
			name:        "添加前缀后缀",
			args:        []string{"process", "--prefix", "[", "--suffix", "]", "hello", "world"},
			expectedOut: "[hello]\n[world]\n",
		},
		{
			name:        "重复输出",
			args:        []string{"process", "--repeat", "3", "test"},
			expectedOut: "test test test\n",
		},
		{
			name:        "所有选项组合",
			args:        []string{"process", "--prefix", ">", "--suffix", "<", "--repeat", "2", "a", "b"},
			expectedOut: ">a< >a<\n>b< >b<\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 在每个测试中创建新的命令实例
			var output strings.Builder
			
			cmd := &cobra.Command{
				Use:   "process",
				Short: "处理输入",
				RunE: func(cmd *cobra.Command, args []string) error {
					prefix, _ := cmd.Flags().GetString("prefix")
					suffix, _ := cmd.Flags().GetString("suffix")
					repeat, _ := cmd.Flags().GetInt("repeat")
					
					for _, arg := range args {
						result := prefix + arg + suffix
						for i := 0; i < repeat; i++ {
							output.WriteString(result)
							if i < repeat-1 {
								output.WriteString(" ")
							}
						}
						output.WriteString("\n")
					}
					
					return nil
				},
			}
			
			cmd.Flags().String("prefix", "", "前缀")
			cmd.Flags().String("suffix", "", "后缀")
			cmd.Flags().Int("repeat", 1, "重复次数")

			rootCmd := &cobra.Command{Use: "test"}
			rootCmd.AddCommand(cmd)
			
			rootCmd.SetArgs(tt.args)
			
			var outBuf bytes.Buffer
			rootCmd.SetOut(&outBuf)
			rootCmd.SetErr(&outBuf)

			err := rootCmd.Execute()
			
			if err != nil {
				t.Errorf("命令执行失败: %v", err)
			}
			
			if output.String() != tt.expectedOut {
				t.Errorf("输出不匹配\n期望: %q\n实际: %q", tt.expectedOut, output.String())
			}
		})
	}
}
			
			var outBuf bytes.Buffer
			rootCmd.SetOut(&outBuf)
			rootCmd.SetErr(&outBuf)

			err = rootCmd.Execute()
			
			output := outBuf.String()
			
			if tt.expectError {
				if err == nil {
					t.Errorf("期望错误但未发生")
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但发生了: %v", err)
				}
				if output != tt.expectedOut {
					t.Errorf("输出不匹配\n期望: %q\n实际: %q", tt.expectedOut, output)
				}
			}
		})
	}
}

// TestCombinedFlagsAndArgs 测试标志和参数的组合
func TestCombinedFlagsAndArgs(t *testing.T) {
	var output strings.Builder
	
	cmd := &cobra.Command{
		Use:   "process",
		Short: "处理输入",
		RunE: func(cmd *cobra.Command, args []string) error {
			prefix, _ := cmd.Flags().GetString("prefix")
			suffix, _ := cmd.Flags().GetString("suffix")
			repeat, _ := cmd.Flags().GetInt("repeat")
			
			for _, arg := range args {
				result := prefix + arg + suffix
				for i := 0; i < repeat; i++ {
					output.WriteString(result)
					if i < repeat-1 {
						output.WriteString(" ")
					}
				}
				output.WriteString("\n")
			}
			
			return nil
		},
	}
	
	cmd.Flags().String("prefix", "", "前缀")
	cmd.Flags().String("suffix", "", "后缀")
	cmd.Flags().Int("repeat", 1, "重复次数")

	tests := []struct {
		name        string
		args        []string
		expectedOut string
	}{
		{
			name:        "基本处理",
			args:        []string{"process", "hello", "world"},
			expectedOut: "hello\nworld\n",
		},
		{
			name:        "添加前缀后缀",
			args:        []string{"process", "--prefix", "[", "--suffix", "]", "hello", "world"},
			expectedOut: "[hello]\n[world]\n",
		},
		{
			name:        "重复输出",
			args:        []string{"process", "--repeat", "3", "test"},
			expectedOut: "test test test\n",
		},
		{
			name:        "所有选项组合",
			args:        []string{"process", "--prefix", ">", "--suffix", "<", "--repeat", "2", "a", "b"},
			expectedOut: ">a< >a<\n>b< >b<\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 重置输出
			output.Reset()
			
			rootCmd := &cobra.Command{Use: "test"}
			rootCmd.AddCommand(cmd)
			
			rootCmd.SetArgs(tt.args)
			
			var outBuf bytes.Buffer
			rootCmd.SetOut(&outBuf)
			rootCmd.SetErr(&outBuf)

			err := rootCmd.Execute()
			
			if err != nil {
				t.Errorf("命令执行失败: %v", err)
			}
			
			if output.String() != tt.expectedOut {
				t.Errorf("输出不匹配\n期望: %q\n实际: %q", tt.expectedOut, output.String())
			}
		})
	}
}
