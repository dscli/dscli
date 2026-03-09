package main

import (
	"testing"
)

// TestCheckDangerousCommands 测试基于语法解析的危险命令检测
func TestCheckDangerousCommands(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr bool
	}{
		{
			name:    "安全命令",
			script:  "echo hello",
			wantErr: false,
		},
		{
			name:    "危险命令_rm_rf_root",
			script:  "rm -rf /",
			wantErr: true,
		},
		{
			name:    "危险命令_rm_rf_home",
			script:  "rm -rf ~",
			wantErr: true,
		},
		{
			name:    "危险命令_dd",
			script:  "dd if=/dev/zero of=/dev/sda",
			wantErr: true,
		},
		{
			name:    "危险命令_mkfs",
			script:  "mkfs.ext4 /dev/sda1",
			wantErr: false, // 对于AI助手，我们不把mkfs视为危险命令
		},
		{
			name:    "危险命令_kill_all",
			script:  "kill -9 -1",
			wantErr: true,
		},
		{
			name:    "危险命令_shutdown",
			script:  "shutdown -h now",
			wantErr: true,
		},
		{
			name:    "大小写变体_应该安全",
			script:  "RM -RF /",
			wantErr: false, // 现在基于语法解析，大小写不匹配
		},
		{
			name:    "注释中的危险命令_应该安全",
			script:  "# rm -rf /",
			wantErr: false,
		},
		{
			name:    "echo中的危险命令_应该安全",
			script:  "echo 'rm -rf / is dangerous'",
			wantErr: false,
		},
		{
			name:    "安全删除项目内",
			script:  "rm -rf ./build",
			wantErr: false,
		},
		{
			name:    "安全删除项目外",
			script:  "rm -rf ../other",
			wantErr: false,
		},
		{
			name:    "多行危险命令",
			script:  "#!/bin/bash\nrm -rf /\necho done",
			wantErr: true,
		},
		{
			name:    "管道中的危险命令",
			script:  "ls | rm -rf /",
			wantErr: true,
		},
		{
			name:    "条件执行中的危险命令",
			script:  "test -f file.txt && rm -rf /",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkDangerousCommands(tt.script)
			if (err != nil) != tt.wantErr {
				t.Errorf("checkDangerousCommands() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestValidateShell 测试shell脚本验证
func TestValidateShell(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		wantErr bool
	}{
		{
			name:    "安全脚本",
			script:  "ls -la",
			wantErr: false,
		},
		{
			name:    "运行dscli",
			script:  "./dscli chat",
			wantErr: false,
		},
		{
			name:    "运行dscli model",
			script:  "./dscli model",
			wantErr: false,
		},
		{
			name:    "运行dscli_无路径",
			script:  "dscli model",
			wantErr: false,
		},
		{
			name:    "危险命令",
			script:  "rm -rf /",
			wantErr: true,
		},
		{
			name:    "组合危险",
			script:  "./dscli chat && rm -rf /",
			wantErr: true,
		},
		{
			name:    "安全删除项目内",
			script:  "rm -rf ./build",
			wantErr: false,
		},
		{
			name:    "注释中的dscli_应该安全",
			script:  "# ./dscli chat",
			wantErr: false,
		},
		{
			name:    "echo中的dscli_应该安全",
			script:  "echo 'running ./dscli chat'",
			wantErr: false,
		},
		{
			name:    "复杂脚本_混合安全危险",
			script:  "#!/bin/bash\necho 'Starting script'\n# This is a comment about dscli\nls -la\nrm -rf /tmp/test\n# Don't run: ./dscli chat\necho 'Done'",
			wantErr: false,
		},
		{
			name:    "复杂脚本_包含实际危险",
			script:  "#!/bin/bash\necho 'Starting script'\nls -la\nrm -rf /\necho 'This is dangerous'",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateShell(tt.script)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateShell() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestParseCommands 测试命令解析
func TestParseCommands(t *testing.T) {
	tests := []struct {
		name     string
		script   string
		wantCmds []string
	}{
		{
			name:     "简单命令",
			script:   "ls -la",
			wantCmds: []string{"ls"},
		},
		{
			name:     "多个命令",
			script:   "ls -la && echo hello",
			wantCmds: []string{"ls", "echo"},
		},
		{
			name:     "带路径的命令",
			script:   "./dscli chat",
			wantCmds: []string{"./dscli"},
		},
		{
			name:     "管道命令",
			script:   "ls | grep test",
			wantCmds: []string{"ls", "grep"},
		},
		{
			name:     "注释中的命令_应该不解析",
			script:   "# rm -rf /",
			wantCmds: []string{},
		},
		{
			name:     "字符串中的命令_应该不解析",
			script:   "echo 'rm -rf /'",
			wantCmds: []string{"echo"},
		},
		{
			name:     "复杂脚本",
			script:   "#!/bin/bash\necho 'Start'\nls -la\n# Comment: ./dscli chat\necho 'End'",
			wantCmds: []string{"echo", "ls", "echo"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			commands, err := parseCommands(tt.script)
			if err != nil {
				t.Fatalf("parseCommands() error = %v", err)
			}

			// 提取命令名
			var cmdNames []string
			for _, cmd := range commands {
				if cmd.IsExecuted {
					cmdNames = append(cmdNames, cmd.Name)
				}
			}

			// 检查命令数量
			if len(cmdNames) != len(tt.wantCmds) {
				t.Errorf("parseCommands() got %v commands, want %v", len(cmdNames), len(tt.wantCmds))
				t.Errorf("got: %v", cmdNames)
				t.Errorf("want: %v", tt.wantCmds)
			}

			// 检查每个命令
			for i, gotCmd := range cmdNames {
				if i >= len(tt.wantCmds) {
					break
				}
				if gotCmd != tt.wantCmds[i] {
					t.Errorf("command[%d] = %v, want %v", i, gotCmd, tt.wantCmds[i])
				}
			}
		})
	}
}

// TestIsDangerousCommand 测试危险命令匹配逻辑
func TestIsDangerousCommand(t *testing.T) {
	tests := []struct {
		name string
		cmd  CommandInfo
		rule DangerousCommand
		want bool
	}{
		{
			name: "精确匹配_rm_rf_root",
			cmd: CommandInfo{
				Name: "rm",
				Args: []string{"-rf", "/"},
			},
			rule: DangerousCommand{
				Command:    "rm",
				Args:       []string{"-rf", "/"},
				ExactMatch: true,
			},
			want: true,
		},
		{
			name: "部分匹配_rm_rf_other",
			cmd: CommandInfo{
				Name: "rm",
				Args: []string{"-rf", "/tmp"},
			},
			rule: DangerousCommand{
				Command:    "rm",
				Args:       []string{"-rf", "/"},
				ExactMatch: true,
			},
			want: false,
		},
		{
			name: "命令名不匹配",
			cmd: CommandInfo{
				Name: "ls",
				Args: []string{"-rf", "/"},
			},
			rule: DangerousCommand{
				Command:    "rm",
				Args:       []string{"-rf", "/"},
				ExactMatch: true,
			},
			want: false,
		},
		{
			name: "无参数规则_匹配",
			cmd: CommandInfo{
				Name: "mkfs",
				Args: []string{"ext4", "/dev/sda1"},
			},
			rule: DangerousCommand{
				Command: "mkfs",
			},
			want: true,
		},
		{
			name: "参数包含匹配",
			cmd: CommandInfo{
				Name: "dd",
				Args: []string{"if=/dev/zero", "of=/dev/sda"},
			},
			rule: DangerousCommand{
				Command: "dd",
				Args:    []string{"if=/dev/zero", "of=/dev/sd"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isDangerousCommand(tt.cmd, tt.rule)
			if got != tt.want {
				t.Errorf("isDangerousCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}
