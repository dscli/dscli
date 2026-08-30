package dsml

import (
	"context"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/toolcall"
)

// registerDocProbeTools registers minimal ToolDefs so the doc builder has
// tool definitions to generate entries from (this package's test env does
// not blank-import alltools; production does).
func registerDocProbeTools(t *testing.T, defs ...toolcall.ToolDef) {
	t.Helper()
	for _, def := range defs {
		def := def
		if err := toolcall.RegisterTool(def); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { toolcall.UnregisterTool(def.Name) })
	}
}

func docShellDef() toolcall.ToolDef {
	return toolcall.ToolDef{
		Name:        "shell",
		Description: "Run a shell script.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script":  map[string]any{"type": "string", "description": "Shell script content."},
				"summary": map[string]any{"type": "string", "description": "Brief summary."},
				"timeout": map[string]any{"type": "integer", "description": "Timeout in seconds."},
			},
			"required":             []string{"script", "summary"},
			"additionalProperties": false,
		},
		Handler: func(_ context.Context, _ toolcall.ToolArgs) (string, string, error) { return "", "", nil },
	}
}

func docReadFileDef() toolcall.ToolDef {
	return toolcall.ToolDef{
		Name:        "read_file",
		Description: "Read a file or line range.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":       map[string]any{"type": "string", "description": "File path."},
				"start_line": map[string]any{"type": "integer", "description": "Start line."},
				"end_line":   map[string]any{"type": "integer", "description": "End line."},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Handler: func(_ context.Context, _ toolcall.ToolArgs) (string, string, error) { return "", "", nil },
	}
}

// TestBuildDSMLToolDocDefaults 验证无角色配置时的默认行为与 DefaultFor
// 一致：dev 得到全部工具（从注册表生成的条目），
// expert/review/test 得到空文档（模板整段消失）。
func TestBuildDSMLToolDocDefaults(t *testing.T) {
	registerDocProbeTools(t, docShellDef(), docReadFileDef())
	tests := []struct {
		role      string
		wantTools bool
	}{
		{"dev", true},
		{"test", false},
		{"expert", false},
		{"review", false},
	}
	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			doc := BuildDSMLToolDoc(t.Context(), tt.role)
			if doc.Intro == "" && doc.Schemas == "" {
				if tt.wantTools {
					t.Fatalf("%s: expected a DSML tool doc, got empty", tt.role)
				}
				return
			}
			if !tt.wantTools {
				t.Fatalf("%s: expected no DSML tool doc, got:\n%s", tt.role, doc.Intro)
			}
			if doc.Intro == "" || doc.Schemas == "" {
				t.Fatalf("%s: Intro and Schemas must both be set (Intro=%q, Schemas=%q)", tt.role, doc.Intro, doc.Schemas)
			}
		})
	}
}

// TestBuildDSMLToolDocContent 验证生成的文档包含 V4 对齐的关键要素：
// 可用工具标题、骨架示例、string= 编码规则、参数说明、JSON schemas
// 与 "strictly follow" 闭合句。条目一律从注册的 ToolDef 生成，所以
// 模型看到的名字/参数就是本地工具的原生 schema（shell 而非 exec_command）。
func TestBuildDSMLToolDocContent(t *testing.T) {
	registerDocProbeTools(t, docShellDef(), docReadFileDef())
	doc := BuildDSMLToolDoc(t.Context(), "dev")
	for _, want := range []string{
		"## 🛠️ Available Tools:",
		"`shell`",
		"`read_file`",
		"<tool_calls>",
		"<invoke name=\"shell\">",
		"<parameter name=\"script\" string=\"true\">",
		"<invoke name=\"read_file\">",
		"String parameters should be specified as is and set `string=\"true\"`",
		"- `shell`: `script`",
		"- `read_file`:",
		"`path` (string, required) — File path.",
		"### Available Tool Schemas",
		"\"name\": \"shell\"",
		"\"name\": \"read_file\"",
		"You MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.",
	} {
		if !strings.Contains(doc.Intro, want) && !strings.Contains(doc.Schemas, want) {
			t.Errorf("doc missing %q\nIntro:\n%s\nSchemas:\n%s", want, doc.Intro, doc.Schemas)
		}
	}
	// exec_command 不在文档中出现：文档只从注册的 ToolDef 生成，任何
	// 未注册/旧拼写都不会被广告，避免误导模型生成无法执行的调用。
	if strings.Contains(doc.Intro, "exec_command") || strings.Contains(doc.Schemas, "exec_command") {
		t.Errorf("doc must not advertise the unknown exec_command spelling\nIntro:\n%s", doc.Intro)
	}
	if strings.Contains(doc.Intro, "{{") || strings.Contains(doc.Schemas, "{{") {
		t.Error("doc leaks template placeholders")
	}
}

// TestBuildDSMLToolDocRoleConfig 验证角色配置（role_configs）驱动文档：
// 配置 shell,read_file 只注册这两个工具（按注册名一一对应；未知名字不
// 产生条目），配置为空字符串则整段消失。
func TestBuildDSMLToolDocRoleConfig(t *testing.T) {
	ctx := t.Context()
	sessionID := session.GetCurrentSessionID(ctx)
	registerDocProbeTools(t, docShellDef(), docReadFileDef())

	if err := roles.UpsertRoleConfig(ctx, "review", sessionID, nil, strPtrHelper("shell,read_file"), nil); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}
	doc := BuildDSMLToolDoc(ctx, "review")
	if !strings.Contains(doc.Intro, "`read_file`, `shell`") || strings.Contains(doc.Intro, "apply_patch") {
		t.Errorf("review doc must register exactly read_file+shell:\n%s", doc.Intro)
	}

	if err := roles.UpsertRoleConfig(ctx, "review", sessionID, nil, strPtrHelper(""), nil); err != nil {
		t.Fatalf("UpsertRoleConfig (clear): %v", err)
	}
	if doc := BuildDSMLToolDoc(ctx, "review"); doc.Intro != "" || doc.Schemas != "" {
		t.Errorf("review doc must be empty after clearing tools:\n%s", doc.Intro)
	}

	// Restore for other tests in this package: delete the row so the
	// DefaultFor path (expert/review = none) still holds.
	if err := roles.DeleteRoleConfig(ctx, "review", sessionID); err != nil {
		t.Fatalf("DeleteRoleConfig: %v", err)
	}
}

// TestBuildDSMLToolDocGenerated: a role-configured tool without a
// hand-written DSML entry is registered from its ToolDef - `dscli role
// update --tools` is the single place that decides what the web model may
// call, with no code change needed per tool.
func TestBuildDSMLToolDocGenerated(t *testing.T) {
	ctx := t.Context()
	sessionID := session.GetCurrentSessionID(ctx)

	const probeName = "probe_tool"
	if err := toolcall.RegisterTool(toolcall.ToolDef{
		Name:        probeName,
		Description: "probe description",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "SQL query"},
				"limit": map[string]any{"type": "integer", "description": "Row cap"},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Handler: func(_ context.Context, _ toolcall.ToolArgs) (string, string, error) {
			return "", "", nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { toolcall.UnregisterTool(probeName) })

	if err := roles.UpsertRoleConfig(ctx, "review", sessionID, nil, strPtrHelper("probe_tool"), nil); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}
	t.Cleanup(func() {
		if err := roles.DeleteRoleConfig(ctx, "review", sessionID); err != nil {
			t.Fatalf("DeleteRoleConfig: %v", err)
		}
	})

	doc := BuildDSMLToolDoc(ctx, "review")
	if !strings.Contains(doc.Intro, "`probe_tool`") {
		t.Fatalf("review doc must register the generated probe_tool entry:\n%s", doc.Intro)
	}
	for _, want := range []string{
		"<invoke name=\"probe_tool\">",
		`<parameter name="query" string="true">...</parameter>`,
		`<parameter name="limit" string="false">0</parameter>`,
		"`query` (string, required) — SQL query",
		"`limit` (integer, optional) — Row cap",
		"\"name\": \"probe_tool\"",
	} {
		if !strings.Contains(doc.Intro, want) && !strings.Contains(doc.Schemas, want) {
			t.Errorf("generated doc missing %q\nIntro:\n%s\nSchemas:\n%s", want, doc.Intro, doc.Schemas)
		}
	}
}

func strPtrHelper(s string) *string { return &s }
