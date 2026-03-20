package shell

import (
	"strings"
	"testing"
	"time"
)

func TestSimpleExecute(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name     string
		script   string
		wantErr  bool
		contains string
	}{
		{
			name:     "简单echo命令",
			script:   "echo 'Hello, World!'",
			wantErr:  false,
			contains: "Hello, World!",
		},
		{
			name:     "变量赋值",
			script:   "name='Test'\necho \"Hello, $name\"",
			wantErr:  false,
			contains: "Hello, Test",
		},
		{
			name:     "算术运算",
			script:   "echo $((2 + 3 * 4))",
			wantErr:  false,
			contains: "14",
		},
		{
			name:     "命令成功",
			script:   "true",
			wantErr:  false,
			contains: "",
		},
		{
			name:     "命令失败",
			script:   "false",
			wantErr:  true,
			contains: "",
		},
		{
			name:     "语法错误",
			script:   "echo 'unclosed quote",
			wantErr:  true,
			contains: "语法解析失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := SimpleExecute(ctx, tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望错误但得到 nil")
				}
				if tt.contains != "" && !strings.Contains(err.Error(), tt.contains) {
					t.Errorf("错误信息不包含 %q，得到: %v", tt.contains, err)
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但得到: %v", err)
				}
				if tt.contains != "" && !strings.Contains(output, tt.contains) {
					t.Errorf("输出不包含 %q，得到: %s", tt.contains, output)
				}
			}
		})
	}
}

func TestSafeExecute(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name        string
		script      string
		wantErr     bool
		errContains string
	}{
		{
			name:    "允许的命令",
			script:  "echo '安全测试'",
			wantErr: false,
		},
		{
			name:    "文件操作（沙箱内）",
			script:  "echo 'test' > test.txt && cat test.txt",
			wantErr: false,
		},
		{
			name:        "禁止的命令",
			script:      "rm -rf /",
			wantErr:     true,
			errContains: "exit status",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := SafeExecute(ctx, tt.script)

			if tt.wantErr {
				if err == nil {
					t.Errorf("期望错误但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("错误信息不包含 %q，得到: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但得到: %v", err)
					t.Errorf("输出: %s", output)
				}
			}
		})
	}
}

func TestExecutor_ExecuteWithTimeout(t *testing.T) {
	ctx := t.Context()

	// 使用无沙箱配置测试超时
	config := DefaultConfig(ctx)
	config.SandboxMode = false
	executor := NewExecutor(ctx, config)

	// 测试正常执行
	t.Run("正常执行", func(t *testing.T) {
		script := "echo '正常执行测试'"

		result, err := executor.Execute(ctx, script)
		if err != nil {
			t.Errorf("不期望错误但得到: %v", err)
		}
		if result == nil {
			t.Fatal("结果为 nil")
		}
		if result.ExitCode != 0 {
			t.Errorf("期望退出码 0 但得到: %d", result.ExitCode)
		}
		if !strings.Contains(result.Stdout, "正常执行测试") {
			t.Errorf("输出不包含期望内容，得到: %s", result.Stdout)
		}
		if result.Duration == 0 {
			t.Error("执行时间应该大于 0")
		}
	})
}

func TestSandboxConfig(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name        string
		config      *Config
		script      string
		expectError bool
	}{
		{
			name: "严格沙箱-允许的命令",
			config: &Config{
				WorkingDir:  ".",
				Timeout:     5 * time.Second,
				StrictMode:  true,
				SandboxMode: true,
				SandboxConfig: &SandboxConfig{
					AllowedCommands: []string{"echo", "cat"},
					AllowedPaths:    []string{"."},
				},
			},
			script:      "echo '测试'",
			expectError: false,
		},
		{
			name: "严格沙箱-禁止的命令",
			config: &Config{
				WorkingDir:  ".",
				Timeout:     5 * time.Second,
				StrictMode:  true,
				SandboxMode: true,
				SandboxConfig: &SandboxConfig{
					AllowedCommands: []string{"echo"},
					AllowedPaths:    []string{"."},
				},
			},
			script:      "ls",
			expectError: true,
		},
		{
			name: "无沙箱模式",
			config: &Config{
				WorkingDir:  ".",
				Timeout:     5 * time.Second,
				StrictMode:  true,
				SandboxMode: false,
			},
			script:      "echo '无沙箱测试'",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx = t.Context()
			executor := NewExecutor(ctx, tt.config)
			result, err := executor.Execute(ctx, tt.script)

			if tt.expectError {
				if err == nil && result.Err == nil {
					t.Errorf("期望错误但执行成功")
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但得到: %v", err)
				}
				if result != nil && result.Err != nil {
					t.Errorf("不期望执行错误但得到: %v", result.Err)
				}
			}
		})
	}
}

func TestEnvironmentFiltering(t *testing.T) {
	ctx := t.Context()

	config := DefaultConfig(ctx)
	config.SandboxMode = true
	config.SandboxConfig = &SandboxConfig{
		AllowedEnvVars: []string{"TEST_VAR", "PATH"},
	}

	// 设置测试环境变量
	config.EnvVars = []string{
		"TEST_VAR=test_value",
		"PATH=/usr/bin:/bin",
		"SENSITIVE_VAR=secret",
	}

	executor := NewExecutor(ctx, config)
	script := "echo $TEST_VAR; echo $SENSITIVE_VAR"

	result, err := executor.Execute(ctx, script)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}

	// 检查输出
	output := result.Stdout
	if !strings.Contains(output, "test_value") {
		t.Errorf("期望包含 TEST_VAR 但得到: %s", output)
	}
	if strings.Contains(output, "secret") {
		t.Errorf("不期望包含 SENSITIVE_VAR 但得到: %s", output)
	}
}

func BenchmarkSimpleExecute(b *testing.B) {
	ctx := b.Context()
	script := "echo '性能测试'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SimpleExecute(ctx, script)
		if err != nil {
			b.Fatalf("执行失败: %v", err)
		}
	}
}

func BenchmarkSafeExecute(b *testing.B) {
	ctx := b.Context()
	script := "echo '安全性能测试'"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := SafeExecute(ctx, script)
		if err != nil {
			b.Fatalf("执行失败: %v", err)
		}
	}
}
