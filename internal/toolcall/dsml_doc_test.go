package toolcall

import (
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/session"
)

// TestDSMLDocForSpec 验证 spec → DSML 工具条目的过滤语义：白名单外的
// 名字会被丢弃（执行器会拒绝它们，注册只会误导模型），shell 与
// exec_command 是同义入口。
func TestDSMLDocForSpec(t *testing.T) {
	tests := []struct {
		name string
		spec string
		want []string // dsmlName 列表，按显示顺序
	}{
		{"all registers the whole whitelist", "all", []string{"exec_command", "read_file", "apply_patch"}},
		{"empty means nothing", "", nil},
		{"none spelling also means nothing", "none", nil},
		{"shell and read_file", "shell,read_file", []string{"exec_command", "read_file"}},
		{"exec_command synonym", "exec_command", []string{"exec_command"}},
		{"shell synonym", "shell", []string{"exec_command"}},
		{"read_file only", "read_file", []string{"read_file"}},
		{"apply_patch only", "apply_patch", []string{"apply_patch"}},
		{"mixed order follows whitelist order", "apply_patch,shell", []string{"exec_command", "apply_patch"}},
		{"non-whitelisted names are dropped", "read_file,sql,vision_file_read", []string{"read_file"}},
		{"whitelisted plus unknown keeps whitelisted", "shell,read_file,mcp_foo", []string{"exec_command", "read_file"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dsmlDocForSpec(tt.spec)
			if len(got) != len(tt.want) {
				t.Fatalf("dsmlDocForSpec(%q) = %d entries %v, want %d %v", tt.spec, len(got), entryNames(got), len(tt.want), tt.want)
			}
			for i, e := range got {
				if e.dsmlName != tt.want[i] {
					t.Errorf("entry[%d] = %q, want %q", i, e.dsmlName, tt.want[i])
				}
			}
		})
	}
}

func entryNames(entries []dsmlDocTool) []string {
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.dsmlName)
	}
	return names
}

// TestBuildDSMLToolDocDefaults 验证无角色配置时的默认行为与 DefaultFor
// 一致：dev/test 得到全部白名单工具，expert/review 得到空文档（模板
// 整段消失）。
func TestBuildDSMLToolDocDefaults(t *testing.T) {
	tests := []struct {
		role      string
		wantTools bool
	}{
		{"dev", true},
		{"test", true},
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
// 与 "strictly follow" 闭合句。
func TestBuildDSMLToolDocContent(t *testing.T) {
	doc := BuildDSMLToolDoc(t.Context(), "dev")
	for _, want := range []string{
		"## 🛠️ Available Tools: `exec_command`, `read_file`, `apply_patch`",
		"<tool_calls>",
		"<invoke name=\"read_file\">",
		"<parameter name=\"cmd\" string=\"true\">",
		"String parameters should be specified as is and set `string=\"true\"`",
		"- `exec_command`:",
		"- `read_file`:",
		"- `apply_patch`:",
		"### Available Tool Schemas",
		"\"name\": \"exec_command\"",
		"\"name\": \"read_file\"",
		"\"name\": \"apply_patch\"",
		"You MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.",
	} {
		if !strings.Contains(doc.Intro, want) && !strings.Contains(doc.Schemas, want) {
			t.Errorf("doc missing %q\nIntro:\n%s\nSchemas:\n%s", want, doc.Intro, doc.Schemas)
		}
	}
	if strings.Contains(doc.Intro, "{{") || strings.Contains(doc.Schemas, "{{") {
		t.Error("doc leaks template placeholders")
	}
}

// TestBuildDSMLToolDocRoleConfig 验证角色配置（role_configs）驱动文档：
// 配置 shell,read_file 只注册这两个工具（exec_command 是 shell 的 DSML
// 展示名），配置为空字符串则整段消失。
func TestBuildDSMLToolDocRoleConfig(t *testing.T) {
	ctx := t.Context()
	sessionID := session.GetCurrentSessionID(ctx)

	if err := roles.UpsertRoleConfig(ctx, "review", sessionID, nil, strPtrHelper("shell,read_file"), nil); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}
	doc := BuildDSMLToolDoc(ctx, "review")
	if !strings.Contains(doc.Intro, "`exec_command`, `read_file`") || strings.Contains(doc.Intro, "apply_patch") {
		t.Errorf("review doc must register exactly exec_command+read_file:\n%s", doc.Intro)
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

func strPtrHelper(s string) *string { return &s }
