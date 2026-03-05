package main

import (
	"context"
	"strings"
	"testing"
)

func TestLoadPrompts(t *testing.T) {
	ctx := context.Background()

	// 设置测试环境
	originalModelID := ModelID
	originalProjectRoot := ProjectRoot
	defer func() {
		ModelID = originalModelID
		ProjectRoot = originalProjectRoot
	}()

	// 测试Deepseek Chat模型
	ModelID = DeepseekChat
	ProjectRoot = "/tmp/test-project"

	messages, err := LoadPrompts(ctx)
	if err != nil {
		t.Fatalf("LoadPrompts() failed: %v", err)
	}

	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}

	msg := messages[0]
	if msg.Role != "system" {
		t.Errorf("Expected role 'system', got %s", msg.Role)
	}

	// 检查提示词包含关键信息
	content := msg.Content
	expectedKeywords := []string{
		"专业的编程助手",
		"当前日期",
		"工作目录",
		"项目根目录",
		"Git状态",
		"文件操作权限",
		"版权信息",
		"工作流程",
	}

	for _, keyword := range expectedKeywords {
		if !strings.Contains(content, keyword) {
			t.Errorf("Prompt missing keyword: %s", keyword)
		}
	}

	// 测试Deepseek Reasoner模型
	ModelID = DeepseekReasoner
	messages, err = LoadPrompts(ctx)
	if err != nil {
		t.Fatalf("LoadPrompts() failed for Reasoner: %v", err)
	}

	reasonerContent := messages[0].Content
	if !strings.Contains(reasonerContent, "深入思考者") {
		t.Errorf("Reasoner prompt missing '深入思考者'")
	}
	if !strings.Contains(reasonerContent, "工作流程") {
		t.Errorf("Reasoner prompt missing '工作流程'")
	}
}

func TestGetSystemPrompt(t *testing.T) {
	ctx := context.Background()

	// 设置测试环境
	originalModelID := ModelID
	originalProjectRoot := ProjectRoot
	defer func() {
		ModelID = originalModelID
		ProjectRoot = originalProjectRoot
	}()

	// 测试Deepseek Chat模型
	ModelID = DeepseekChat
	ProjectRoot = "/tmp/test-project"

	prompt := GetSystemPrompt(ctx)
	if prompt == "" {
		t.Fatal("GetSystemPrompt() returned empty string")
	}

	// 检查包含日期信息
	if !strings.Contains(prompt, "当前日期：") {
		t.Error("Prompt missing date information")
	}

	// 检查包含项目信息
	if !strings.Contains(prompt, "项目根目录：") {
		t.Error("Prompt missing project root information")
	}

	// 测试Deepseek Reasoner模型
	ModelID = DeepseekReasoner
	prompt = GetSystemPrompt(ctx)
	if prompt == "" {
		t.Fatal("GetSystemPrompt() returned empty string for Reasoner")
	}

	if !strings.Contains(prompt, "深入思考者") {
		t.Error("Reasoner prompt missing '深入思考者'")
	}
}

func TestSystemPromptConfig(t *testing.T) {
	ctx := context.Background()

	// 设置测试环境
	originalProjectRoot := ProjectRoot
	originalModelID := ModelID
	defer func() {
		ProjectRoot = originalProjectRoot
		ModelID = originalModelID
	}()

	ProjectRoot = "/tmp/test-project"
	ModelID = DeepseekChat

	config := NewSystemPromptConfig(ctx)

	// 测试基础信息
	if config.CurrentDate == "" {
		t.Error("CurrentDate should not be empty")
	}

	if config.ProjectRoot != "/tmp/test-project" {
		t.Errorf("Expected ProjectRoot '/tmp/test-project', got %s", config.ProjectRoot)
	}

	if config.ConfigDir == "" {
		t.Error("ConfigDir should not be empty")
	}

	// 测试项目信息
	if config.ProjectName != "test-project" {
		t.Errorf("Expected ProjectName 'test-project', got %s", config.ProjectName)
	}

	// 测试提示词生成
	prompt := config.GeneratePrompt()
	if prompt == "" {
		t.Error("GeneratePrompt() returned empty string")
	}

	// 测试包含关键部分
	sections := []string{
		"环境信息",
		"项目信息",
		"Git状态",
		"文件操作权限",
		"版权信息",
		"工作流程",
	}

	for _, section := range sections {
		if !strings.Contains(prompt, section) {
			t.Errorf("Prompt missing section: %s", section)
		}
	}
}

// 测试不同模型的提示词差异
func TestModelSpecificPrompts(t *testing.T) {
	ctx := context.Background()

	// 设置测试环境
	originalModelID := ModelID
	originalProjectRoot := ProjectRoot
	defer func() {
		ModelID = originalModelID
		ProjectRoot = originalProjectRoot
	}()

	ProjectRoot = "/tmp/test-project"

	// 测试Chat模型
	ModelID = DeepseekChat
	chatPrompt := GetEnhancedSystemPrompt(ctx)

	if !strings.Contains(chatPrompt, "专业的编程助手") {
		t.Error("Chat prompt should contain '专业的编程助手'")
	}
	if !strings.Contains(chatPrompt, "文件操作权限") {
		t.Error("Chat prompt should contain '文件操作权限'")
	}

	// 测试Reasoner模型
	ModelID = DeepseekReasoner
	reasonerPrompt := GetEnhancedSystemPrompt(ctx)

	if !strings.Contains(reasonerPrompt, "深入思考者") {
		t.Error("Reasoner prompt should contain '深入思考者'")
	}
	if !strings.Contains(reasonerPrompt, "思考原则") {
		t.Error("Reasoner prompt should contain '思考原则'")
	}

	// 验证两个模型的提示词不同
	if chatPrompt == reasonerPrompt {
		t.Error("Chat and Reasoner prompts should be different")
	}
}
