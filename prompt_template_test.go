package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPromptTemplate(t *testing.T) {
	ctx := context.Background()

	// 测试模板生成
	prompt := GetTemplateSystemPrompt(ctx)
	if prompt == "" {
		t.Fatal("生成的提示词为空")
	}

	// 检查是否包含关键信息
	expectedParts := []string{
		"你是一个专业的编程助手",
		"当前日期：",
		"环境信息",
		"项目信息",
		"Git状态",
		"文件操作权限",
		"版权信息",
		"你的工作流程",
		"重要原则",
	}

	for _, part := range expectedParts {
		if !strings.Contains(prompt, part) {
			t.Errorf("提示词缺少关键部分: %s", part)
		}
	}

	// 检查日期是否正确
	currentDate := time.Now().Format("2006年01月02日")
	if !strings.Contains(prompt, currentDate) {
		t.Errorf("提示词中的日期不正确，期望包含: %s", currentDate)
	}

	// 测试模板管理器
	template := NewPromptTemplate(ctx)
	generated := template.GeneratePrompt()
	if generated != prompt {
		t.Error("模板管理器生成的提示词与直接调用函数不一致")
	}

	// 测试模板渲染
	tmplStr := `测试模板: {{.CurrentDate}} - {{.ProjectName}}`
	template2 := &PromptTemplate{
		config: &SystemPromptConfig{
			CurrentDate: "2026年03月06日",
			ProjectName: "测试项目",
		},
	}

	result := template2.generateWithTemplate(tmplStr)
	expected := "测试模板: 2026年03月06日 - 测试项目"
	if result != expected {
		t.Errorf("模板渲染错误，期望: %s, 实际: %s", expected, result)
	}
}

func TestSystemPromptConfig_HasGitChanges(t *testing.T) {
	tests := []struct {
		name      string
		gitStatus string
		expected  bool
	}{
		{"有变更", "有3个文件变更", true},
		{"工作区干净", "工作区干净", false},
		{"空状态", "", false},
		{"其他状态", "正在合并", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &SystemPromptConfig{
				GitStatus: tt.gitStatus,
			}
			result := config.HasGitChanges()
			if result != tt.expected {
				t.Errorf("HasGitChanges() = %v, 期望 %v (状态: %s)", result, tt.expected, tt.gitStatus)
			}
		})
	}
}

func TestDeepseekReasonerTemplate(t *testing.T) {
	// 临时切换模型ID测试
	originalModelID := ModelID
	defer func() { ModelID = originalModelID }()

	ModelID = DeepseekReasoner
	ctx := context.Background()

	prompt := GetTemplateSystemPrompt(ctx)
	if prompt == "" {
		t.Fatal("生成的Reasoner提示词为空")
	}

	// 检查Reasoner特定内容
	expectedParts := []string{
		"你是编程领域一个深入思考者",
		"思考环境",
		"你的工作流程",
		"思考原则",
		"全面地理解问题",
		"深入地思考问题",
		"给出深刻地洞察",
	}

	for _, part := range expectedParts {
		if !strings.Contains(prompt, part) {
			t.Errorf("Reasoner提示词缺少关键部分: %s", part)
		}
	}

	// 不应该包含Chat特定内容
	unexpectedParts := []string{
		"文件操作权限",
		"Git状态",
		"配置目录",
	}

	for _, part := range unexpectedParts {
		if strings.Contains(prompt, part) {
			t.Errorf("Reasoner提示词不应该包含: %s", part)
		}
	}
}

func TestTemplateConditionals(t *testing.T) {
	// 测试模板中的条件逻辑
	config := &SystemPromptConfig{
		GitUserName:  "测试用户",
		GitUserEmail: "test@example.com",
		GitBranch:    "main",
		GitStatus:    "有1个文件变更",
	}

	template := &PromptTemplate{config: config}
	prompt := template.GeneratePrompt()

	// 检查条件内容是否正确渲染
	if !strings.Contains(prompt, "测试用户 <test@example.com>") {
		t.Error("模板没有正确渲染Git用户信息", prompt)
	}

	if !strings.Contains(prompt, "分支：main") {
		t.Error("模板没有正确渲染Git分支信息")
	}

	if !strings.Contains(prompt, "状态：有1个文件变更") {
		t.Error("模板没有正确渲染Git状态信息")
	}

	// 测试没有Git信息的情况
	config2 := &SystemPromptConfig{
		GitUserName:  "",
		GitUserEmail: "",
		GitBranch:    "",
		GitStatus:    "",
	}

	template2 := &PromptTemplate{config: config2}
	prompt2 := template2.GeneratePrompt()

	// 不应该包含空的Git信息行
	if strings.Contains(prompt2, "用户： <") {
		t.Error("模板不应该渲染空的Git用户信息")
	}

	if strings.Contains(prompt2, "分支：") && !strings.Contains(prompt2, "分支：main") {
		t.Error("模板不应该渲染空的Git分支信息")
	}
}
