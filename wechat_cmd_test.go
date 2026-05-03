package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestWechatCommand 简单粗暴的测试：验证wechat命令及其所有子命令都存在
func TestWechatCommand(t *testing.T) {
	// 执行 dscli wechat --help 命令
	cmd := exec.Command("go", "run", ".", "wechat", "--help")
	outputBytes, err := cmd.CombinedOutput()
	output := string(outputBytes)

	if err != nil {
		// 如果命令不存在，这是测试失败
		t.Fatalf("执行命令失败: %v\n输出: %s", err, output)
	}

	// 验证wechat命令的基本信息
	if !strings.Contains(output, "Usage:") {
		t.Error("命令输出缺少Usage信息")
	}

	if !strings.Contains(output, "wechat") {
		t.Error("命令输出缺少'wechat'关键词")
	}

	// 检查所有必须的子命令
	requiredSubcommands := []string{
		"login",
		"logout",
		"status",
		"messages",
		"message",
		"send",
		"reply",
		"mark-read",
		"friends",
		"groups",
		"config",
	}

	missingCommands := []string{}
	for _, cmd := range requiredSubcommands {
		if !strings.Contains(output, cmd) {
			missingCommands = append(missingCommands, cmd)
		}
	}

	if len(missingCommands) > 0 {
		t.Errorf("缺少子命令: %v", missingCommands)
	}

	// 检查帮助信息中是否有基本描述
	if !strings.Contains(output, "微信AI工具接口") {
		t.Error("命令描述不完整")
	}

	// 输出成功信息
	if len(missingCommands) == 0 {
		t.Logf("✅ wechat命令测试通过，找到所有%d个子命令", len(requiredSubcommands))
		// 只输出前几行帮助信息
		lines := strings.Split(output, "\n")
		if len(lines) > 10 {
			t.Logf("命令输出预览:\n%s", strings.Join(lines[:10], "\n"))
		} else {
			t.Logf("命令输出:\n%s", output)
		}
	}
}