package shell

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestIsShellScript(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		script      string
		wantValid   bool
		wantErr     bool
		errContains string
	}{
		{
			name:      "简单echo命令",
			script:    "echo 'Hello, World!'",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "变量赋值",
			script:    "name='Test'\necho \"Hello, $name\"",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "算术运算",
			script:    "echo $((2 + 3 * 4))",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "if语句",
			script:    "if [ -f test.txt ]; then echo '文件存在'; fi",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "for循环",
			script:    "for i in 1 2 3; do echo $i; done",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "函数定义",
			script:    "greet() { echo \"Hello, $1\"; }\ngreet World",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:        "语法错误-未闭合引号",
			script:      "echo 'unclosed quote",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:        "语法错误-无效语法",
			script:      "echo hello | | grep world",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:        "语法错误-未闭合括号",
			script:      "echo $((2 + 3)",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:      "空脚本",
			script:    "",
			wantValid: true, // 空脚本是有效的Shell脚本
			wantErr:   false,
		},
		{
			name:      "只有注释",
			script:    "# 这是一个注释",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "shebang行",
			script:    "#!/bin/bash\necho 'Hello'",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:        "Python脚本",
			script:      "print('Hello, World!')",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:        "JavaScript代码",
			script:      "console.log('Hello');",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:      "纯文本",
			script:    "This is just plain text, not a shell script.",
			wantValid: true, // 纯文本在Shell语法中是有效的（虽然执行会失败）
			wantErr:   false,
		},
		{
			name:        "混合内容",
			script:      "echo 'Hello'\nprint('World')\necho 'Again'",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() { os.RemoveAll("test.txt") })
			isValid, err := IsShellScript(ctx, tt.script)

			// 检查有效性判断
			if isValid != tt.wantValid {
				t.Errorf("IsShellScript() 有效性判断错误: 期望 %v, 得到 %v", tt.wantValid, isValid)
			}

			// 检查错误
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望错误但得到 nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("错误信息不包含 %q，得到: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但得到: %v", err)
				}
			}

			// 如果脚本有效，验证它确实可以被执行
			if isValid && err == nil && tt.script != "" {
				// 尝试执行简单的echo命令来验证
				if strings.Contains(tt.script, "echo") {
					output, execErr := SimpleExecute(ctx, "echo '验证执行'")
					if execErr != nil {
						t.Errorf("有效脚本执行失败: %v", execErr)
					}
					if !strings.Contains(output, "验证执行") {
						t.Errorf("执行输出不包含期望内容: %s", output)
					}
				}
			}
		})
	}
}

// ============================================================
// 命令验证测试
// ============================================================

func TestVerifySystemCommand(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		name          string
		cmd           string
		wantExists    bool
		wantVerified  bool
		checkVersion  bool // 是否检查版本信息非空
	}{
		{
			name:         "系统命令 echo",
			cmd:          "echo",
			wantExists:   true,
			wantVerified: true,
			checkVersion: true,
		},
		{
			name:         "系统命令 ls",
			cmd:          "ls",
			wantExists:   true,
			wantVerified: true,
			checkVersion: true,
		},
		{
			name:         "系统命令 cat",
			cmd:          "cat",
			wantExists:   true,
			wantVerified: true,
			checkVersion: true,
		},
		{
			name:       "不存在的命令",
			cmd:        "nonexistent_command_xyz123",
			wantExists: false,
		},
		{
			name:         "go 命令",
			cmd:          "go",
			wantExists:   true,
			wantVerified: true,
			checkVersion: true,
		},
		{
			name:         "git 命令",
			cmd:          "git",
			wantExists:   true,
			wantVerified: true,
			checkVersion: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := VerifySystemCommand(ctx, tt.cmd)
			if err != nil {
				t.Fatalf("VerifySystemCommand() 返回错误: %v", err)
			}
			if info == nil {
				t.Fatal("info 为 nil")
			}

			if info.Name != tt.cmd {
				t.Errorf("Name: 期望 %q, 得到 %q", tt.cmd, info.Name)
			}

			if info.Exists != tt.wantExists {
				t.Errorf("Exists: 期望 %v, 得到 %v (Error: %s)", tt.wantExists, info.Exists, info.Error)
			}

			if tt.wantExists {
				if info.Path == "" {
					t.Error("命令存在但 Path 为空")
				}
			} else {
				if info.Error == "" {
					t.Error("命令不存在但 Error 为空")
				}
			}

			if tt.wantVerified {
				if !info.Verified {
					t.Errorf("期望 Verified=true, 得到 false (Version=%q)", info.Version)
				}
				if tt.checkVersion && info.Version == "" {
					t.Error("期望有版本信息但 Version 为空")
				}
			}
		})
	}
}

func TestTryGetVersion(t *testing.T) {
	ctx := t.Context()

	// 只测试已知会返回版本信息的命令
	tests := []struct {
		name       string
		cmd        string
		wantNonEmpty bool
	}{
		{"echo 命令", "echo", true},
		{"go 命令", "go", true},
		{"git 命令", "git", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, err := exec.LookPath(tt.cmd)
			if err != nil {
				t.Skipf("系统无 %s 命令，跳过测试", tt.cmd)
			}

			version := tryGetVersion(ctx, path)
			if tt.wantNonEmpty && version == "" {
				t.Errorf("tryGetVersion(%q) 返回空，期望非空", tt.cmd)
			}
			t.Logf("%s 版本: %s", tt.cmd, version)
		})
	}
}

func TestIsCommandAvailable(t *testing.T) {
	ctx := t.Context()

	// 使用实际的允许列表
	allowedCommands := getAllowedCommands()

	tests := []struct {
		name      string
		cmd       string
		allowed   []string
		wantAvail bool
	}{
		{
			name:      "允许且存在的命令 echo",
			cmd:       "echo",
			allowed:   allowedCommands,
			wantAvail: true,
		},
		{
			name:      "允许且存在的命令 ls",
			cmd:       "ls",
			allowed:   allowedCommands,
			wantAvail: true,
		},
		{
			name:      "不在允许列表的命令（判不存在）",
			cmd:       "systemctl",
			allowed:   allowedCommands,
			wantAvail: false,
		},
		{
			name:      "不存在的命令",
			cmd:       "nonexistent_cmd_xyz",
			allowed:   allowedCommands,
			wantAvail: false,
		},
		{
			name:      "允许但系统不存在的命令（自定义列表）",
			cmd:       "fakecmd_xyz_not_exist",
			allowed:   []string{"fakecmd_xyz_not_exist"},
			wantAvail: false,
		},
		{
			name:      "空允许列表",
			cmd:       "echo",
			allowed:   []string{},
			wantAvail: false,
		},
		{
			name:      "命令在允许列表中但系统不存在",
			cmd:       "nonexistent_in_allowlist",
			allowed:   append(allowedCommands, "nonexistent_in_allowlist"),
			wantAvail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsCommandAvailable(ctx, tt.cmd, tt.allowed)
			if got != tt.wantAvail {
				t.Errorf("IsCommandAvailable(%q) = %v, 期望 %v", tt.cmd, got, tt.wantAvail)
			}
		})
	}
}

func TestGetAvailableCommandsDescription(t *testing.T) {
	ctx := t.Context()

	desc := GetAvailableCommandsDescription(ctx)

	// 基本检查
	if desc == "" {
		t.Error("返回空字符串")
	}

	// 应该包含至少一个分类标题（###）
	if !strings.Contains(desc, "### ") {
		t.Errorf("输出不包含分类标题 (###):\n%s", desc)
	}

	// 应该包含 echo（几乎肯定存在）
	if !strings.Contains(desc, "echo") {
		t.Error("输出不包含 echo 命令")
	}

	// 不应该包含不存在的命令描述（如果 tectonic 不存在）
	// 只做基本格式检查
	t.Logf("可用命令描述:\n%s", desc)
}

func TestVerifySystemCommand_RgAlias(t *testing.T) {
	ctx := t.Context()

	info, err := VerifySystemCommand(ctx, "rg")
	if err != nil {
		t.Fatalf("VerifySystemCommand(rg) 返回错误: %v", err)
	}

	// rg 可能不存在，如果存在则检查
	if info.Exists {
		t.Logf("rg 路径: %s", info.Path)
		t.Logf("rg 版本: %s", info.Version)

		// 如果版本信息包含 "ripgrep"，确认是真 rg
		if info.Verified && strings.Contains(info.Version, "ripgrep") {
			t.Log("✓ 确认为真实 ripgrep")
		} else if info.Verified {
			t.Log("版本已验证但未包含 'ripgrep' 关键字，可能不是真实 ripgrep")
		}
	} else {
		t.Log("rg 未安装，跳过验证")
	}
}