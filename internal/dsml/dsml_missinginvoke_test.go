package dsml

import (
	"strings"
	"testing"
)

// TestParseDSMLMissingInvokeClose is the regression for a real code_review
// round (2026-08-29, chat.deepseek.com IndexedDB content): the expert's last
// emission closed with "</_calls>" right after the final </parameter>,
// dropping "</invoke>" entirely - the close tag of both the call and the
// wrapper collapsed into one typo'd fragment. IsDSMLToolCallEnd already
// treats the wrapper close as the "emission complete" intent signal, so the
// parser must match: complete parameters plus a wrapper close make an
// executable call, not a truncation. The old parse failure stopped the tool
// loop, returned an empty reply to code_review and forced a full re-run of
// the consultation (wasted compute).
func TestParseDSMLMissingInvokeClose(t *testing.T) {
	text := `<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">go build ./internal/toolcall/ask/ 2>&1 | head -20; echo "exit=$?"</parameter>
<parameter name="summary" string="true">Build ask package to check embed var</parameter>
<parameter name="timeout" string="false">180</parameter>
</_calls>`
	if !IsDSMLToolCallEnd(text) {
		t.Fatal("IsDSMLToolCallEnd = false, want true (wrapper close present)")
	}
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v, want success", err)
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	if calls[0].Name != "shell" {
		t.Errorf("name = %q, want shell", calls[0].Name)
	}
	if got, _ := calls[0].Args["script"].(string); got != "go build ./internal/toolcall/ask/ 2>&1 | head -20; echo \"exit=$?\"" {
		t.Errorf("script = %q", got)
	}
	if got, _ := calls[0].Args["summary"].(string); got != "Build ask package to check embed var" {
		t.Errorf("summary = %q", got)
	}
	if got, ok := calls[0].Args["timeout"].(float64); !ok || got != 180 {
		t.Errorf("timeout = %v (%T), want 180 (number)", calls[0].Args["timeout"], calls[0].Args["timeout"])
	}
	if got := StripDSMLToolCalls(text); got != "" {
		t.Errorf("StripDSMLToolCalls = %q, want empty", got)
	}
}

// TestParseDSMLMissingInvokeCloseCompleteWrapper is the same degrade with a
// correctly-spelled </tool_calls> close: only </invoke> is missing.
func TestParseDSMLMissingInvokeCloseCompleteWrapper(t *testing.T) {
	text := `<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">ls</parameter>
</tool_calls>`
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v, want success", err)
	}
	if len(calls) != 1 || calls[0].Name != "shell" {
		t.Fatalf("calls = %d (%+v), want 1 shell call", len(calls), calls)
	}
}

// TestParseDSMLMissingInvokeCloseSiblings: an earlier call closed properly
// while the LAST one dropped </invoke>. Both must parse; the implicit close
// of the second lands at its own body end, so the covered-skip in
// ParseDSMLToolCalls must not drop the sibling.
func TestParseDSMLMissingInvokeCloseSiblings(t *testing.T) {
	text := `<tool_calls>
<invoke name="read_file">
<parameter name="path" string="true">AGENTS.md</parameter>
</invoke>
<invoke name="shell">
<parameter name="script" string="true">git status --short</parameter>
</_calls>`
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v, want success", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	if calls[0].Name != "read_file" || calls[1].Name != "shell" {
		t.Errorf("names = %q, %q", calls[0].Name, calls[1].Name)
	}
	if got, _ := calls[1].Args["script"].(string); got != "git status --short" {
		t.Errorf("second script = %q", got)
	}
}

// TestParseDSMLMissingInvokeCloseBounded pins the strict boundary: the
// implicit close is authorized ONLY by a complete wrapper close plus fully
// closed parameters. No wrapper close (genuine truncation) and an unclosed
// <parameter> (the emission was really cut off mid-arguments) must still be
// rejected, exactly like TestIsDSMLToolCallCutBounded.
func TestParseDSMLMissingInvokeCloseBounded(t *testing.T) {
	noWrapper := `<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">echo hi</parameter>
`
	if IsDSMLToolCallEnd(noWrapper) || IsDSMLToolCallCut(noWrapper) {
		t.Fatal("no-close text must not pass the gate")
	}
	if _, err := ParseDSMLToolCalls(noWrapper); err == nil {
		t.Error("ParseDSMLToolCalls(no wrapper close) = nil error, want truncation error")
	}
	unclosedParam := `<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">echo hi
</_calls>`
	if !IsDSMLToolCallEnd(unclosedParam) {
		t.Fatal("unclosedParam text must pass the gate (wrapper close present)")
	}
	calls, err := ParseDSMLToolCalls(unclosedParam)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls(unclosed parameter + wrapper close) = %v, want success", err)
	}
	if len(calls) != 1 || calls[0].Name != "shell" {
		t.Fatalf("calls = %d (%+v), want 1 shell call", len(calls), calls)
	}
	if got, _ := calls[0].Args["script"].(string); !strings.HasPrefix(got, "echo hi") {
		t.Errorf("script = %q, want value running to the wrapper close", got)
	}
}

// TestParseDSMLMissingInvokeCloseMisNested pins the implicit-close boundary:
// it fires ONLY when exactly one open is left unclosed at the end. A
// mis-nested shape (a second invoke opened before the first was closed, then
// the wrapper close) leaves TWO opens - that is a genuine truncation or a
// model mishap, and the calls must never execute. The two positive cases are
// controls: the single unclosed open (with and without parameters) still
// parses, so the tightening did not regress the observed 2026-08-29 shape.
func TestParseDSMLMissingInvokeCloseMisNested(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantErr  bool
		wantName string
	}{
		{
			name:    "two unclosed opens and wrapper close",
			text:    "<tool_calls>\n<invoke name=\"a\">\n<parameter name=\"x\">v</parameter>\n<invoke name=\"b\">\n</tool_calls>",
			wantErr: true,
		},
		{
			name:    "two unclosed opens and typo close",
			text:    "<invoke name=\"a\">\n<parameter name=\"x\">v</parameter>\n<invoke name=\"b\">\n</_calls>",
			wantErr: true,
		},
		{
			name:     "single unclosed open with params and typo close",
			text:     "<invoke name=\"a\">\n<parameter name=\"x\">v</parameter>\n</_calls>",
			wantName: "a",
		},
		{
			name:     "single parameterless unclosed open and typo close",
			text:     "<invoke name=\"pwd\">\n</_calls>",
			wantName: "pwd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls, err := ParseDSMLToolCalls(tt.text)
			if tt.wantErr && err == nil {
				t.Fatal("ParseDSMLToolCalls = nil error, want truncation error")
			}
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("ParseDSMLToolCalls: %v, want success", err)
				}
				if len(calls) != 1 || calls[0].Name != tt.wantName {
					t.Fatalf("calls = %d (%+v), want 1 %q call", len(calls), calls, tt.wantName)
				}
			}
		})
	}
}
