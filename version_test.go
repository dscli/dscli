package main

import (
	"bytes"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/context"
)

func TestBoolToString(t *testing.T) {
	tests := []struct {
		name  string // 测试用例描述
		input bool   // 输入参数
		want  string // 期望输出
	}{
		{"TrueTo启用", true, "启用"},
		{"FalseTo禁用", false, "禁用"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := boolToString(tt.input)
			if got != tt.want {
				t.Errorf("boolToString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestVersionCommandOutput(t *testing.T) {
	// 保存原始输出写入器和变量
	originalWriter := outputWriter
	originalVersion := Version
	originalBuild := Build
	originalConfigDir := ConfigDir
	originalMode := mode
	originalVerbose := verbose
	originalColorEnabled := colorEnabled
	originalShowTimestamp := showTimestamp
	originalModelChat := ModelDeepseekChat
	originalModelReasoner := ModelDeepseekReasoner

	// 设置测试环境
	defer func() {
		SetOutputWriter(originalWriter)
		Version = originalVersion
		Build = originalBuild
		ConfigDir = originalConfigDir
		mode = originalMode
		verbose = originalVerbose
		colorEnabled = originalColorEnabled
		showTimestamp = originalShowTimestamp
		ModelDeepseekChat = originalModelChat
		ModelDeepseekReasoner = originalModelReasoner
	}()

	// 设置测试值
	Version = "1.0.0-test"
	Build = "test-build-123"
	ConfigDir = "/tmp/.dscli-test"
	mode = "markdown"
	verbose = true
	colorEnabled = true
	showTimestamp = false
	ModelDeepseekChat = "deepseek-chat-test"
	ModelDeepseekReasoner = "deepseek-reasoner-test"

	// 设置测试输出缓冲区
	var buf bytes.Buffer
	SetOutputWriter(&buf)
	ctx := t.Context()

	ctx = context.WithValue(ctx, context.ProjectRootKey, context.GetProjectRoot())
	// 执行version命令
	versionRunE(ctx)

	output := buf.String()

	// 验证输出包含关键信息
	testCases := []struct {
		name     string
		contains string
	}{
		{"标题", "dscli 版本信息"},
		{"基本信息章节", "基本信息"},
		{"版本号", "版本"},
		{"构建信息", "test-build-123"},
		{"运行时信息章节", "运行时信息"},
		{"Go版本", "Go 版本"},
		{"操作系统", "操作系统"},
		{"处理器架构", "处理器架构"},
		{"编译器", "编译器"},
		{"配置信息章节", "配置信息"},
		{"配置目录", "配置目录"},
		{"项目根目录", "项目根目录"},
		{"输出模式", "输出模式"},
		{"详细输出", "详细输出"},
		{"颜色输出", "颜色输出"},
		{"时间戳显示", "时间戳显示"},
		{"模型配置章节", "模型配置"},
		{"聊天模型", "聊天模型"},
		{"推理模型", "推理模型"},
	}

	for _, tc := range testCases {
		if !strings.Contains(output, tc.contains) {
			t.Errorf("version命令输出缺少: %s", tc.name)
		}
	}

	// 验证特定值
	if !strings.Contains(output, "1.0.0-test") {
		t.Error("version命令输出缺少测试版本号")
	}

	if !strings.Contains(output, "deepseek-chat-test") {
		t.Error("version命令输出缺少测试聊天模型")
	}

	if !strings.Contains(output, "deepseek-reasoner-test") {
		t.Error("version命令输出缺少测试推理模型")
	}
}

func TestVersionCommandWithoutBuild(t *testing.T) {
	// 保存原始输出写入器和变量
	originalWriter := outputWriter
	originalVersion := Version
	originalBuild := Build

	// 恢复原始值
	defer func() {
		SetOutputWriter(originalWriter)
		Version = originalVersion
		Build = originalBuild
	}()

	// 设置测试值（无构建信息）
	Version = "1.0.0-test"
	Build = ""

	// 设置测试输出缓冲区
	var buf bytes.Buffer
	SetOutputWriter(&buf)
	ctx := context.WithValue(t.Context(), context.ProjectRootKey, context.GetProjectRoot())
	// 执行version命令
	versionRunE(ctx)

	output := buf.String()

	// 验证版本信息存在
	if !strings.Contains(output, "1.0.0-test") {
		t.Error("version命令输出缺少测试版本号")
	}

	// 验证程序没有崩溃（这是主要测试点）
	// 空构建信息应该被正确处理
}

func TestVersionCommandIntegration(t *testing.T) {
	// 这是一个集成测试，验证命令可以正常执行
	// 保存原始输出写入器
	originalWriter := outputWriter
	defer SetOutputWriter(originalWriter)

	// 设置测试输出缓冲区
	var buf bytes.Buffer
	SetOutputWriter(&buf)
	ctx := context.WithValue(t.Context(), context.ProjectRootKey, context.GetProjectRoot())

	// 执行命令
	versionRunE(ctx)

	output := buf.String()

	// 验证输出不为空
	if len(output) == 0 {
		t.Error("version命令没有输出")
	}

	// 验证输出包含预期的章节
	requiredSections := []string{
		"dscli 版本信息",
		"基本信息",
		"运行时信息",
		"配置信息",
		"模型配置",
	}

	for _, section := range requiredSections {
		if !strings.Contains(output, section) {
			t.Errorf("输出缺少章节: %s", section)
		}
	}
}
