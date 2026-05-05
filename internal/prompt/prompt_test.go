package prompt

import (
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/context"
)

func TestGetEnhancedSystemPrompt(t *testing.T) {
	tests := []struct {
		name        string
		modelID     int64
		role        string
		contains    string
		notcontains string
	}{
		{
			"deepseek-chat",
			context.DeepseekChat,
			"dev",
			"Professional Programming Assistant",
			"system_prompt",
		},
		{
			"deepseek-reasoner",
			context.DeepseekReasoner,
			"expert",
			"Programming Domain Expert",
			"system_prompt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			ctx = context.WithValue(ctx, context.CurrentModelIDKey, tt.modelID)
			ctx = context.WithValue(ctx, context.CurrentRoleKey, tt.role)
			content := GetSystemPrompt(ctx)
			if !strings.Contains(content, tt.contains) {
				t.Fatal(content, tt.contains)
			}
			if strings.Contains(content, tt.notcontains) {
				t.Fatal(content, tt.notcontains)
			}
		})
	}
}

// TestLoadPrompts 验证 LoadPrompts 返回正确的系统消息结构
func TestLoadPrompts(t *testing.T) {
	ctx := t.Context()
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)

	msgs, err := LoadPrompts(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("LoadPrompts 返回空消息列表")
	}
	if msgs[0].Role != "system" {
		t.Errorf("第一条消息 role = %q, 期望 system", msgs[0].Role)
	}
	if msgs[0].Content == "" {
		t.Error("系统提示词内容为空")
	}
	// 核心身份标识必须在
	if !strings.Contains(msgs[0].Content, "Professional Programming Assistant") {
		t.Error("系统提示词缺少身份标识")
	}
	// 不应包含模板占位符 leak
	if strings.Contains(msgs[0].Content, "{{.") {
		t.Error("系统提示词包含未渲染的模板占位符")
	}
}

// TestNewPromptTemplate_NilSafety 验证未知 modelID 不返回 nil
// TestNewPromptTemplate_NilSafety 验证未知角色不返回 nil
// TestNewPromptTemplate_NilSafety 验证未知 modelID 不返回 nil
// TestNewPromptTemplate_NilSafety 验证未知角色不返回 nil
func TestNewPromptTemplate_NilSafety(t *testing.T) {
	ctx := context.Background()
	invalidRoles := []string{"invalid", "unknown", ""}
	for _, role := range invalidRoles {
		tmpl := newPromptTemplate(ctx, role)
		if tmpl == nil {
			t.Errorf("newPromptTemplate(%q) 返回 nil，期望非 nil", role)
		}
	}
	// dev, expert, review 也应返回非 nil
	if tmpl := newPromptTemplate(ctx, "dev"); tmpl == nil {
		t.Error("newPromptTemplate(dev) 返回 nil")
	}
	if tmpl := newPromptTemplate(ctx, "expert"); tmpl == nil {
		t.Error("newPromptTemplate(expert) 返回 nil")
	}
	if tmpl := newPromptTemplate(ctx, "review"); tmpl == nil {
		t.Error("newPromptTemplate(review) 返回 nil")
	}
}
