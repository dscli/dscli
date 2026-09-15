package prompt

import (
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/session"
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

// capabilitiesHeading 是四个角色模板 no-tools 分支（Capabilities 段）的
// 统一标题。模板与测试共享这一字节序列（含 VS16 变体选择符），避免某处
// 规范化 emoji 后出现静默不匹配。
const capabilitiesHeading = "## 🛠️ Capabilities"

// TestShellToolSectionScopedToWebChat verifies the `shell` tool section
// appears only on the WebChat render path: chat.deepseek.com has no native
// tool protocol, so the role prompt is the only tool registration channel
// (RenderPromptForRoleWithShellTool). dscli chat registers tools through the
// API's tools parameter (GetSystemPrompt), and RenderPromptForRole renders
// without the section (its callers - e.g. code_review's review guide - work
// from attachments).
func TestShellToolSectionScopedToWebChat(t *testing.T) {
	chatPrompt := GetSystemPrompt(t.Context())
	if strings.Contains(chatPrompt, "Available Tools: `shell`") {
		t.Errorf("chat system prompt must not contain the shell tool section:\n%s", chatPrompt)
	}

	doc := "## 🛠️ Available Tools: `shell`" + "\n\n" +
		"**Usage: run a bash or python script, or attach files**"
	for _, role := range []string{"dev", "expert", "test", "architect", "review"} {
		t.Run(role, func(t *testing.T) {
			// Without the doc: the section is absent.
			plain := RenderPromptForRole(t.Context(), role)
			if strings.Contains(plain, "Available Tools: `shell`") {
				t.Errorf("%s prompt must not contain the shell tool section without the doc:\n%s", role, plain)
			}
			if !strings.Contains(plain, capabilitiesHeading) {
				t.Errorf("%s prompt without the doc must state the Capabilities limitation", role)
			}
			// With the doc: the section appears, the Capabilities section is
			// replaced, and no placeholder leaks.
			content := RenderPromptForRoleWithShellTool(t.Context(), role, doc)
			if !strings.Contains(content, "Available Tools: `shell`") {
				t.Errorf("%s webchat prompt missing the shell tool registration:\n%s", role, content)
			}
			if !strings.Contains(content, "**Usage: run a bash or python script, or attach files**") {
				t.Errorf("%s webchat prompt missing the shell usage line", role)
			}
			if strings.Contains(content, "{{") {
				t.Errorf("%s webchat prompt leaks template placeholders", role)
			}
			if strings.Contains(content, capabilitiesHeading) {
				t.Errorf("%s prompt with the shell doc must not render the Capabilities heading", role)
			}
		})
	}
}

// TestMailCheckStepScopedToRolesWithMail verifies the dev template's "check
// unread mail" opening step follows the role's mail capability: on the chat
// path (GetSystemPrompt) a role without readmail (dev by default) gets no
// mail instruction, architect gets one (architect.md keeps its own ungated
// step), and WebChat role renders always strip it (CheckMail=false by
// design — WebChat sessions are task-scoped one-shot consultations where a
// mail probe wastes the first round).
func TestMailCheckStepScopedToRolesWithMail(t *testing.T) {
	devChatCtx := context.WithValue(t.Context(), context.CurrentRoleKey, "dev")
	devChat := GetSystemPrompt(devChatCtx)
	if strings.Contains(devChat, "Check for unread mail") {
		t.Errorf("dev chat prompt must not contain the mail-check step (no readmail):\n%s", devChat)
	}
	if !strings.Contains(devChat, "0b. **Read AGENTS.md (only when it exists)**") {
		t.Errorf("dev chat prompt must retain the AGENTS.md step:\n%s", devChat)
	}

	archChatCtx := context.WithValue(t.Context(), context.CurrentRoleKey, "architect")
	archChat := GetSystemPrompt(archChatCtx)
	if !strings.Contains(archChat, "Check for unread mail") {
		t.Errorf("architect chat prompt must keep its own ungated mail-check step:\n%s", archChat)
	}

	plain := RenderPromptForRole(t.Context(), "dev")
	assertMailCheckStripped(t, "webchat prompt (no tools)", plain)
	// Stripping step 0 must not disturb the remaining workflow steps.
	for _, want := range []string{
		"0b. **Read AGENTS.md (only when it exists)**",
		"1. **Fully understand the problem**",
		"2. **Think and analyze deeply**",
		"3. **Provide deep insights**",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("webchat prompt (no tools) missing %q:\n%s", want, plain)
		}
	}
	// Lock the documented invariant: architect.md keeps its own ungated
	// mail step, so an architect WebChat render still reads mail.
	archPlain := RenderPromptForRole(t.Context(), "architect")
	if !strings.Contains(archPlain, "Check for unread mail") {
		t.Errorf("architect WebChat prompt must keep its own mail-check step:\n%s", archPlain)
	}

	// Shell-doc fixture: only a non-empty section is needed.
	doc := "## 🛠️ Available Tools: `shell`" + "\n\n" +
		"**Usage: run a bash or python script, or attach files**"
	content := RenderPromptForRoleWithShellTool(t.Context(), "dev", doc)
	assertMailCheckStripped(t, "webchat prompt (with tools)", content)
	for _, want := range []string{
		"0b. **Read AGENTS.md (only when it exists)**",
		"1. **Fully understand the problem**",
		"2. **Think and analyze deeply**",
		"3. **Provide deep insights**",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("webchat prompt (with tools) missing %q:\n%s", want, content)
		}
	}
	if !strings.Contains(content, "## 🛠️ Available Tools: `shell`") {
		t.Errorf("webchat prompt missing the shell registration:\n%s", content)
	}
}

// TestRenderPromptForRoleWithShellTool locks the shell-channel branch: the
// dev template renders the shell tool doc in place of the capabilities
// section, and
// a plain render never carries it.
func TestRenderPromptForRoleWithShellTool(t *testing.T) {
	doc := "## 🛠️ Available Tools: `shell`\n\n**Usage: execute bash script**"
	content := RenderPromptForRoleWithShellTool(t.Context(), "dev", doc)
	assertMailCheckStripped(t, "webchat shell prompt", content)
	for _, want := range []string{
		"0b. **Read AGENTS.md (only when it exists)**",
		"## 🛠️ Available Tools: `shell`",
		"**Usage: execute bash script**",
		"attached text file",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("shell prompt missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, capabilitiesHeading) {
		t.Errorf("shell prompt must not render the Capabilities section:\n%s", content)
	}
	plain := RenderPromptForRole(t.Context(), "dev")
	if strings.Contains(plain, "Available Tools: `shell`") {
		t.Errorf("plain dev prompt must not carry the shell section:\n%s", plain)
	}
}

// TestRoleCanReadMail verifies the chat-path mail gate: without readmail in
// the role's tool set (dev by default) RoleCanReadMail is false; architect
// (all tools) is true. Tests run without a session row, so the fallback
// (roles.DefaultFor) applies — GetCurrentSessionID returns 0 and the lookup
// misses, matching production behavior for fresh projects.
func TestRoleCanReadMail(t *testing.T) {
	if got := RoleCanReadMail(t.Context()); got {
		t.Errorf("RoleCanReadMail() with no role (defaults to dev) = true, want false")
	}
	devCtx := context.WithValue(t.Context(), context.CurrentRoleKey, "dev")
	if got := RoleCanReadMail(devCtx); got {
		t.Errorf("RoleCanReadMail(dev) = true, want false")
	}
	archCtx := context.WithValue(t.Context(), context.CurrentRoleKey, "architect")
	if !RoleCanReadMail(archCtx) {
		t.Errorf("RoleCanReadMail(architect) = false, want true (architect is all tools)")
	}
}

// TestRoleCanReadMailSpec locks the spec parsing: "all" resolves to nil
// (everything, including readmail); DevDefaultTools and "" exclude it; an
// explicit list containing readmail includes it. Whitespace is tolerated
// around the spec.
func TestRoleCanReadMailSpec(t *testing.T) {
	cases := []struct {
		spec string
		want bool
	}{
		{"all", true},
		{" all ", true},
		{roles.DevDefaultTools, false},
		{"", false},
		{"shell,readmail", true},
	}
	for _, c := range cases {
		if got := roleCanReadMailSpec(c.spec); got != c.want {
			t.Errorf("roleCanReadMailSpec(%q) = %v, want %v", c.spec, got, c.want)
		}
	}
}

// TestRoleCanReadMailRowOverride verifies the row-wins semantics end to end
// in BOTH directions, exercising the full chain through prompt.ToolsSpec →
// roles.GetRoleConfig without mocking. It runs in an isolated session scope
// (a temp project root, like withIsolatedSession in interrupt_test.go), so
// the upserted role_configs rows land under a throwaway session id and can
// never leak into tests that resolve the real project session — this is what
// keeps `go test -shuffle=on` clean.
func TestRoleCanReadMailRowOverride(t *testing.T) {
	ctx := withIsolatedSession(t)
	sessionID := session.GetCurrentSessionID(ctx)

	t.Run("tools=all flips dev to true", func(t *testing.T) {
		all := "all"
		if err := roles.UpsertRoleConfig(t.Context(), "dev", sessionID, nil, &all, nil); err != nil {
			t.Fatalf("UpsertRoleConfig: %v", err)
		}
		devCtx := context.WithValue(t.Context(), context.CurrentRoleKey, "dev")
		if !RoleCanReadMail(devCtx) {
			t.Errorf("RoleCanReadMail(dev) with stored tools=all row = false, want true (row wins over default)")
		}
	})

	t.Run("tools=none flips dev back to false", func(t *testing.T) {
		none := ""
		if err := roles.UpsertRoleConfig(t.Context(), "dev", sessionID, nil, &none, nil); err != nil {
			t.Fatalf("UpsertRoleConfig: %v", err)
		}
		devCtx := context.WithValue(t.Context(), context.CurrentRoleKey, "dev")
		if RoleCanReadMail(devCtx) {
			t.Errorf("RoleCanReadMail(dev) with stored tools=none row = true, want false (row wins in both directions)")
		}
	})
}

// assertMailCheckStripped checks the shared invariants for a WebChat-rendered
// dev prompt (CheckMail=false): the mail-check step is absent and the
// Workflow heading flows directly into 0b with exactly one blank line.
func assertMailCheckStripped(t *testing.T, label, content string) {
	t.Helper()
	if strings.Contains(content, "Check for unread mail") {
		t.Errorf("%s must not contain the mail-check step:\n%s", label, content)
	}
	if !strings.Contains(content, "## 🔄 Workflow\n\n0b.") {
		t.Errorf("%s must render Workflow followed by 0b with exactly one blank line:\n%s", label, content)
	}
	if strings.Contains(content, "## 🔄 Workflow\n\n\n") {
		t.Errorf("%s must not leave a double blank line after Workflow:\n%s", label, content)
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
	// dev, expert, review, architect 也应返回非 nil
	if tmpl := newPromptTemplate(ctx, "dev"); tmpl == nil {
		t.Error("newPromptTemplate(dev) 返回 nil")
	}
	if tmpl := newPromptTemplate(ctx, "expert"); tmpl == nil {
		t.Error("newPromptTemplate(expert) 返回 nil")
	}
	if tmpl := newPromptTemplate(ctx, "review"); tmpl == nil {
		t.Error("newPromptTemplate(review) 返回 nil")
	}
	if tmpl := newPromptTemplate(ctx, "architect"); tmpl == nil {
		t.Error("newPromptTemplate(architect) 返回 nil")
	}
}
