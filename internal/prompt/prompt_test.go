package prompt

import (
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/context"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
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

// TestDSMLToolsSectionScopedToWebChat 验证 DSML 工具注册段只出现在
// WebChat 渲染路径：chat.deepseek.com 没有原生工具协议，角色提示词是
// 注册通道（RenderPromptForRole -> DSMLTools=true）；dscli chat 通过 API
// 的 tools 参数注册工具（GetSystemPrompt -> DSMLTools=false），模板中
// 的 <invoke> 手写示例不得泄漏进去，否则模型会误用 DSML 而非原生
// tool_calls。
func TestDSMLToolsSectionScopedToWebChat(t *testing.T) {
	chatPrompt := GetSystemPrompt(t.Context())
	if strings.Contains(chatPrompt, "<invoke name=") || strings.Contains(chatPrompt, "Available Tools") {
		t.Errorf("chat system prompt must not contain the DSML tool section:\n%s", chatPrompt)
	}

	for _, role := range []string{"dev", "expert", "review", "test"} {
		t.Run(role, func(t *testing.T) {
			content := RenderPromptForRole(t.Context(), role)
			if !strings.Contains(content, "<invoke name=\"read_file\">") {
				t.Errorf("%s webchat prompt missing DSML read_file registration:\n%s", role, content)
			}
			if !strings.Contains(content, "Available Tools") {
				t.Errorf("%s webchat prompt missing Available Tools heading", role)
			}
			if strings.Contains(content, "{{") {
				t.Errorf("%s webchat prompt leaks template placeholders", role)
			}
		})
	}
}

// TestNewPromptTemplate_NilSafety 验证未知 modelID / 角色不返回 nil
// （回退到 dev 模板）。
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
