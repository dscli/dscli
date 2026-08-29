package toolcall

import "testing"

// TestIsDSMLToolCallReplyBareInvoke is the regression for a real code_dev
// round (2026-08-29): the developer model replied with a BARE <invoke> block
// - no <tool_calls> wrapper, no wrapper close tag at all - so
// IsDSMLToolCallEnd and IsDSMLToolCallCut were both false and the reply was
// returned as a final answer instead of executing. The stored shape is
// exactly the parser's own structural language: complete <invoke> with
// complete parameters and nothing else in the reply (the site renders it
// with two-space indentation, which is cosmetic). IsPureDSMLToolCalls is
// the third intent signal: parseable complete calls plus empty stripped
// text IS an instruction, quoted examples never leave empty stripped text.
func TestIsDSMLToolCallReplyBareInvoke(t *testing.T) {
	bare := "<invoke name=\"read_file\">\n<parameter name=\"path\" string=\"true\">AGENTS.md</parameter>\n</invoke>"
	indented := "  <invoke name=\"read_file\">\n  <parameter name=\"path\" string=\"true\">AGENTS.md</parameter>\n  </invoke>"
	for name, text := range map[string]string{"bare": bare, "indented": indented} {
		t.Run(name, func(t *testing.T) {
			if IsDSMLToolCallEnd(text) {
				t.Error("IsDSMLToolCallEnd = true, want false (no wrapper close)")
			}
			if IsDSMLToolCallCut(text) {
				t.Error("IsDSMLToolCallCut = true, want false (nothing cut off)")
			}
			if !IsPureDSMLToolCalls(text) {
				t.Error("IsPureDSMLToolCalls = false, want true (bare complete calls)")
			}
			if !IsDSMLToolCallReply(text) {
				t.Error("IsDSMLToolCallReply = false, want true")
			}
			calls, err := ParseDSMLToolCalls(text)
			if err != nil {
				t.Fatalf("ParseDSMLToolCalls: %v", err)
			}
			if len(calls) != 1 || calls[0].Name != "read_file" {
				t.Fatalf("calls = %+v, want one read_file", calls)
			}
			if got, _ := calls[0].Args["path"].(string); got != "AGENTS.md" {
				t.Errorf("path = %q, want AGENTS.md", got)
			}
			if got := StripDSMLToolCalls(text); got != "" {
				t.Errorf("StripDSMLToolCalls = %q, want empty", got)
			}
		})
	}
}

// TestIsDSMLToolCallReplyBareInvokeSiblings: two bare calls back to back
// (no wrapper) are both parsed and both execute - the loop runs the full
// set, not just the first one.
func TestIsDSMLToolCallReplyBareInvokeSiblings(t *testing.T) {
	text := "<invoke name=\"read_file\">\n<parameter name=\"path\" string=\"true\">AGENTS.md</parameter>\n</invoke>\n<invoke name=\"shell\">\n<parameter name=\"script\" string=\"true\">git status --short</parameter>\n</invoke>"
	if !IsDSMLToolCallReply(text) {
		t.Error("IsDSMLToolCallReply = false, want true")
	}
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 2 || calls[0].Name != "read_file" || calls[1].Name != "shell" {
		t.Fatalf("calls = %+v, want read_file + shell", calls)
	}
}

// TestIsDSMLToolCallReplyBareInvokeBounded pins what must NOT be a tool-call
// reply: quoted examples (fence, inline code, prose with context) and
// truncated emissions. The three-signal union must never turn a citation
// into an executed command.
func TestIsDSMLToolCallReplyBareInvokeBounded(t *testing.T) {
	fenced := "example:\n```\n<invoke name=\"read_file\">\n<parameter name=\"path\" string=\"true\">AGENTS.md</parameter>\n</invoke>\n```\n"
	inlineCode := "markdown like `<invoke name=\"read_file\">` means read a file"
	prose := "see this call: <invoke name=\"read_file\"><parameter name=\"path\" string=\"true\">AGENTS.md</parameter></invoke>, then continue."
	missingInvokeClose := "<invoke name=\"read_file\">\n<parameter name=\"path\" string=\"true\">AGENTS.md</parameter>"
	missingParamClose := "<invoke name=\"read_file\">\n<parameter name=\"path\" string=\"true\">AGENTS.md"
	for name, text := range map[string]string{
		"fenced":               fenced,
		"inline-code":          inlineCode,
		"prose":                prose,
		"missing-invoke-close": missingInvokeClose,
		"missing-param-close":  missingParamClose,
	} {
		if IsDSMLToolCallReply(text) {
			t.Errorf("%s: IsDSMLToolCallReply = true, want false", name)
		}
	}
}

// TestIsDSMLToolCallReplyEndCutSignalsPreserved: replies that already
// qualified via the wrapper-close signals still do; the union is additive,
// not a replacement.
func TestIsDSMLToolCallReplyEndCutSignalsPreserved(t *testing.T) {
	withClose := "<tool_calls>\n<invoke name=\"shell\">\n<parameter name=\"script\" string=\"true\">ls</parameter>\n</invoke>\n</tool_calls>"
	withTypoClose := "<tool_calls>\n<invoke name=\"shell\">\n<parameter name=\"script\" string=\"true\">ls</parameter>\n</invoke>\n</_calls>"
	withCut := "<tool_calls>\n<invoke name=\"shell\">\n<parameter name=\"script\" string=\"true\">ls</parameter>\n</invoke>\n</"
	for name, text := range map[string]string{
		"end":        withClose,
		"typo-close": withTypoClose,
		"cut":        withCut,
	} {
		if !IsDSMLToolCallReply(text) {
			t.Errorf("%s: IsDSMLToolCallReply = false, want true (signal preserved)", name)
		}
	}
}
