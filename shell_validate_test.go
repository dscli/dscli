package main

import (
	"slices"
	"testing"
)

// TestCheckDangerousCommands 测试危险命令检测
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
			wantErr: true,
		},
		{
			name:    "危险命令_fork_bomb",
			script:  ":(){ :|:& };:",
			wantErr: true,
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
			name:    "大小写变体",
			script:  "RM -RF /",
			wantErr: true,
		},
		{
			name:    "部分匹配",
			script:  "echo 'rm -rf / is dangerous'",
			wantErr: true,
		},
		{
			name:    "多行危险命令",
			script:  "#!/bin/bash\nrm -rf /\necho done",
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
			wantErr: true,
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
			name:    "危险删除项目外",
			script:  "rm -rf ../other",
			wantErr: false, // 注意：当前实现不会检测这个
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

func TestParseCommands(t *testing.T) {
	commands, err := parseCommands(`./dscli chat`)
	if err != nil {
		t.Fatal(err, commands)
	}
	if !slices.Contains(commands, "./dscli") {
		t.Fatal(commands)
	}
}
