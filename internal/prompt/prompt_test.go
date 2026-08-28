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
// 注册通道（RenderPromptForRoleWithTools + 非空 DSMLToolDoc）；dscli
// chat 通过 API 的 tools 参数注册工具（GetSystemPrompt），模板中的
// <invoke> 示例不得泄漏进去，否则模型会误用 DSML 而非原生 tool_calls。
// RenderPromptForRole（无 doc）也不得渲染该段：工具集合由角色配置
// 驱动（toolcall.BuildDSMLToolDoc），无配置时 expert/review 无工具。
func TestDSMLToolsSectionScopedToWebChat(t *testing.T) {
	chatPrompt := GetSystemPrompt(t.Context())
	if strings.Contains(chatPrompt, "<invoke name=") || strings.Contains(chatPrompt, "Available Tools") {
		t.Errorf("chat system prompt must not contain the DSML tool section:\n%s", chatPrompt)
	}

	doc := DSMLToolDoc{
		Intro:   "## 🛠️ Available Tools: `read_file`\n\n<tool_calls>\n<invoke name=\"read_file\">\n<parameter name=\"path\" string=\"true\">AGENTS.md</parameter>\n</invoke>\n</tool_calls>",
		Schemas: "### Available Tool Schemas\n\n```json\n{\"type\":\"function\"}\n```\n\nYou MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.",
	}
	for _, role := range []string{"dev", "expert", "review", "test"} {
		t.Run(role, func(t *testing.T) {
			// Without doc: the section must be absent (role may have no tools).
			plain := RenderPromptForRole(t.Context(), role)
			if strings.Contains(plain, "<invoke name=") || strings.Contains(plain, "Available Tools") {
				t.Errorf("%s prompt must not contain the DSML tool section without doc:\n%s", role, plain)
			}
			// With doc: the section appears and no placeholder leaks.
			content := RenderPromptForRoleWithTools(t.Context(), role, doc)
			if !strings.Contains(content, "<invoke name=\"read_file\">") {
				t.Errorf("%s webchat prompt missing DSML read_file registration:\n%s", role, content)
			}
			if !strings.Contains(content, "Available Tools") {
				t.Errorf("%s webchat prompt missing Available Tools heading", role)
			}
			if strings.Contains(content, "{{") {
				t.Errorf("%s webchat prompt leaks template placeholders", role)
			}
			// All role templates render a Capabilities section ONLY without
			// tools: with tools the DSML intro IS the capability section
			// (mutually exclusive, never both — a hand-written tool list
			// would go stale against the role's actual tool config).
			if !strings.Contains(plain, "## 🛠️ Capabilities") {
				t.Errorf("%s prompt without tools must state the Capabilities limitation", role)
			}
			if strings.Contains(content, "## 🛠️ Capabilities") {
				t.Errorf("%s prompt with tools must not render the Capabilities heading (DSML intro replaces it)", role)
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
