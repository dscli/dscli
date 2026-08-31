package dsml

import "testing"

// TestIsDSMLToolCallEndBadgeLabelClose is the regression for the badge-LABEL
// close variant: chat.deepseek.com renders the tool_calls wrapper as a special
// UI badge and persists the rendered form, with the tag NAME replaced by a
// human-readable badge label. Observed 2026-08 for a QA round: the reply ended
// with the site's full-width pipe badge marker plus the "evaluation" label
// (U+2011 non-breaking hyphen). The label is not a DSML tag name, so the
// known-name junk sweeper never touched it. normalizeDSMLText must rewrite the
// pipe-marked residue to the canonical close so the gate and parser see it.
func TestIsDSMLToolCallEndBadgeLabelClose(t *testing.T) {
	lt, gt := "<", ">"
	ff := string(rune(0xFF5C))
	badgeClose := lt + "/" + ff + "\u2011evaluation" + gt
	text := lt + "tool_calls" + gt + "\n" +
		lt + "invoke name=\"shell\"" + gt + "\n" +
		lt + "parameter name=\"script\" string=\"true\"" + gt +
		"ls internal/toolcall/ask/" +
		lt + "/parameter" + gt + "\n" +
		lt + "/invoke" + gt + "\n" + badgeClose

	if !IsDSMLToolCallEnd(text) {
		t.Error("IsDSMLToolCallEnd = false, want true for badge-label close tag")
	}
	if !IsDSMLToolCallReply(text) {
		t.Error("IsDSMLToolCallReply = false, want true")
	}
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "shell" {
		t.Fatalf("calls = %d (%+v), want one shell call", len(calls), calls)
	}
	if got := StripDSMLToolCalls(text); got != "" {
		t.Errorf("StripDSMLToolCalls = %q, want empty (no leftover residue)", got)
	}
	// End-to-end through the unified entry: the call parses and executes; the
	// markup is non-strict (badge label), so OK=false but execution is never
	// blocked.
	msg := ParseDSMLMessage("", text)
	if msg.OK {
		t.Error("ParseDSMLMessage OK = true, want false (non-strict badge-label close)")
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "shell" {
		t.Fatalf("msg.ToolCalls = %+v, want one shell call", msg.ToolCalls)
	}
}
