package dsml

import (
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

// TestNormalizeDSMLInvokeRejectsTrailingResidue pins the 2026-09-08 code_dev
// report: a write_file call whose content value carries an unclosed code
// fence swallows the call's own </parameter> and </invoke> tags into the
// value (the fence is opaque to the structural scan, and the wrapper close
// authorizes the implicit close). The call parses, but it must NOT execute -
// the residue would be written into the target file (SKILL.md lines 89-90).
func TestNormalizeDSMLInvokeRejectsTrailingResidue(t *testing.T) {
	residue := strings.Join([]string{
		"<tool_calls>",
		`<invoke name="write_file">`,
		`<parameter name="file">SKILL.md</parameter>`,
		`<parameter name="content">`,
		"# title",
		"",
		"```bash",
		"slingshot draft add a.html",
		"</parameter>",
		"</invoke>",
		"</tool_calls>",
	}, "\n")

	calls, err := ParseDSMLToolCalls(residue)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	_, _, err = normalizeDSMLInvoke(calls[0])
	if err == nil {
		t.Fatal("normalizeDSMLInvoke accepted a value with trailing close-tag residue")
	}
	if !strings.Contains(err.Error(), "close-tag residue") {
		t.Errorf("err = %v, want close-tag residue error", err)
	}
}

// TestNormalizeDSMLInvokeKeepsClosedFenceTags: a value whose fence IS closed
// may legitimately contain DSML example tags (a doc edit). The gate must not
// touch it - the value still ends with the fence, not with the residue.
func TestNormalizeDSMLInvokeKeepsClosedFenceTags(t *testing.T) {
	clean := strings.Join([]string{
		"<tool_calls>",
		`<invoke name="write_file">`,
		`<parameter name="file">SKILL.md</parameter>`,
		`<parameter name="content">`,
		"# title",
		"",
		"```xml",
		`<invoke name="shell">`,
		"</invoke>",
		"```",
		"</parameter>",
		"</invoke>",
		"</tool_calls>",
	}, "\n")

	calls, err := ParseDSMLToolCalls(clean)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	_, args, err := normalizeDSMLInvoke(calls[0])
	if err != nil {
		t.Fatalf("normalizeDSMLInvoke rejected a closed-fence value: %v", err)
	}
	content, _ := args["content"].(string)
	if !strings.Contains(content, "</invoke>") {
		t.Errorf("content = %q, want the fenced example preserved", content)
	}
	if !strings.HasSuffix(strings.TrimSpace(content), "```") {
		t.Errorf("content = %q, want the value to end with its closing fence", content)
	}
}

// TestRejectTrailingResidue documents the gate's exact boundary: the residue
// PAIR (</parameter> + </invoke>, or </invoke> + wrapper close) is refused; a
// single trailing tag is prose/doc content and is left alone.
func TestRejectTrailingResidue(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "residue pair", value: "text\n</parameter>\n</invoke>", wantErr: true},
		{name: "residue pair with trailing newline", value: "text\n</parameter>\n</invoke>\n", wantErr: true},
		{name: "residue invoke plus wrapper", value: "text\n</invoke>\n</tool_calls>", wantErr: true},
		{name: "closed fence keeps tags", value: "```xml\n</invoke>\n```", wantErr: false},
		{name: "single trailing tag is content", value: "text\n</invoke>", wantErr: false},
		{name: "empty", value: "", wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rejectTrailingResidue(toolcall.ToolArgs{"content": tt.value})
			if gotErr := err != nil; gotErr != tt.wantErr {
				t.Errorf("rejectTrailingResidue(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
			}
		})
	}
}
