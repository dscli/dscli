package dsml

import (
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

// xml wrapper helpers are built at runtime so this file stays transportable
// through DSML tool calls (literal brackets in a write_file content would be
// misread as markup and truncate the payload - the same rule
// dsml_strayclose_test.go follows).

func invokeBlock(name, params string) string {
	return "<invoke name=\"" + name + "\">" + params + "</invoke>"
}

func paramX(name, value, stringAttr string) string {
	if stringAttr == "" {
		return "<parameter name=\"" + name + "\">" + value + "</parameter>"
	}
	return "<parameter name=\"" + name + "\" string=\"" + stringAttr + "\">" + value + "</parameter>"
}

func toolCallsBlock(inner string) string {
	return "<tool_calls>" + inner + "</tool_calls>"
}

func TestParseDSMLMessageStrictOK(t *testing.T) {
	// Strictly conforming call: OK=true, ToolCalls present, Content stripped.
	inner := invokeBlock("shell", paramX("script", "echo hi", "true"))
	text := "thinking done\n" + toolCallsBlock(inner)
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) == 0 {
		t.Fatalf("expected ToolCalls, got none")
	}
	if !msg.OK {
		t.Errorf("OK = false, want true (strictly conforming call)")
	}
	if strings.Contains(msg.Content, "invoke") {
		t.Errorf("Content must be stripped of tool-call markup, got:\n%s", msg.Content)
	}
}

func TestParseDSMLMessageJustificationViolation(t *testing.T) {
	// The decorative justification parameter: OK=false, call still parsed,
	// Content kept verbatim (fallback judgement).
	inner := invokeBlock("shell", paramX("script", "echo hi", "true")+paramX("justification", "why", "true"))
	text := toolCallsBlock(inner)
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) == 0 {
		t.Fatalf("expected ToolCalls, got none")
	}
	if msg.OK {
		t.Errorf("OK = true, want false (justification present)")
	}
	if !strings.Contains(msg.Content, "justification") {
		t.Errorf("Content must keep the original text when OK=false, got:\n%s", msg.Content)
	}
}

func TestParseDSMLMessageStrayCloseViolation(t *testing.T) {
	// Extra </invoke> after a complete call: OK=false, call still parsed.
	inner := invokeBlock("shell", paramX("script", "echo hi", "true"))
	text := toolCallsBlock(inner) + "</invoke>"
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) == 0 {
		t.Fatalf("expected ToolCalls, got none")
	}
	if msg.OK {
		t.Errorf("OK = true, want false (stray close tag)")
	}
}

func TestParseDSMLMessageMissingStringViolation(t *testing.T) {
	// <parameter> without the string attribute: OK=false, call still parsed.
	inner := invokeBlock("shell", paramX("script", "echo hi", ""))
	text := toolCallsBlock(inner)
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) == 0 {
		t.Fatalf("expected ToolCalls, got none")
	}
	if msg.OK {
		t.Errorf("OK = true, want false (missing string attribute)")
	}
}

func TestParseDSMLMessageTruncated(t *testing.T) {
	// Cut-off emission: no ToolCalls; SuspectedDSMLToolCalls must agree.
	text := "<tool_calls>" + "<invoke name=\"shell\">" + paramX("script", "echo hi", "true")
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("expected no ToolCalls for truncated emission, got %d", len(msg.ToolCalls))
	}
	if msg.OK {
		t.Errorf("OK = true, want false (truncated emission)")
	}
	if !SuspectedDSMLToolCalls(text) {
		t.Errorf("SuspectedDSMLToolCalls = false, want true (truncated call)")
	}
}

func TestParseDSMLMessagePlainProse(t *testing.T) {
	text := "The repository is clean and tests are green."
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("expected no ToolCalls, got %d", len(msg.ToolCalls))
	}
	if !msg.OK {
		// no calls = no violations = OK=true (clean final answer)
		t.Errorf("OK = false, want true for a plain reply")
	}
	if SuspectedDSMLToolCalls(text) {
		t.Errorf("SuspectedDSMLToolCalls = true, want false (plain prose)")
	}
}

func TestParseDSMLMessageReasoningFallback(t *testing.T) {
	// No call in content, one in reasoning: ToolCalls come from reasoning,
	// reasoning is stripped on OK=true, content untouched.
	inner := invokeBlock("shell", paramX("script", "echo hi", "true"))
	reasoning := "I should check the repo.\n" + toolCallsBlock(inner)
	content := "Here is my answer."
	msg := ParseDSMLMessage(reasoning, content)
	if len(msg.ToolCalls) == 0 {
		t.Fatalf("expected ToolCalls from reasoning, got none")
	}
	if !msg.OK {
		t.Errorf("OK = false, want true (strictly conforming call in reasoning)")
	}
	if strings.Contains(msg.ReasoningContent, "invoke") {
		t.Errorf("ReasoningContent must be stripped, got:\n%s", msg.ReasoningContent)
	}
	if msg.Content != content {
		t.Errorf("Content modified (%q), want %q", msg.Content, content)
	}
}

func TestParseDSMLMessageReasoningQuotedOnly(t *testing.T) {
	// Reasoning carries only a fenced quoted example: no executable call,
	// so OK=true and ReasoningContent must stay verbatim (not stripped).
	inner := invokeBlock("shell", paramX("script", "echo hi", "true"))
	reasoning := "Example in thinking:\n```xml\n" + toolCallsBlock(inner) + "\n```"
	content := "Here is my answer."
	msg := ParseDSMLMessage(reasoning, content)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("expected no ToolCalls, got %d", len(msg.ToolCalls))
	}
	if !msg.OK {
		t.Errorf("OK = false, want true (quoted example only)")
	}
	if msg.Content != content {
		t.Errorf("Content modified (%q), want %q", msg.Content, content)
	}
	if msg.ReasoningContent != reasoning {
		t.Errorf("ReasoningContent must stay verbatim, got:\n%s", msg.ReasoningContent)
	}
}

func TestParseDSMLMessageContentPriority(t *testing.T) {
	// Calls in both: content wins, reasoning untouched.
	innerC := invokeBlock("shell", paramX("script", "echo content", "true"))
	innerR := invokeBlock("read_file", paramX("path", "/tmp/reason", "true"))
	content := toolCallsBlock(innerC)
	reasoning := "draft " + toolCallsBlock(innerR)
	msg := ParseDSMLMessage(reasoning, content)
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 ToolCall (content source), got %d", len(msg.ToolCalls))
	}
	if !strings.Contains(msg.ToolCalls[0].Function.Name, "shell") {
		t.Errorf("ToolCall name = %q, want shell (content priority)", msg.ToolCalls[0].Function.Name)
	}
	if !strings.Contains(msg.ReasoningContent, "read_file") {
		t.Errorf("reasoning must stay untouched when content carries calls")
	}
}

func TestSuspectedDSMLToolCallsQuotedExample(t *testing.T) {
	// A fenced-code quote of an invoke example is NOT suspected - it is a
	// reference, not an attempted call.
	inner := invokeBlock("shell", paramX("script", "echo hi", "true"))
	text := "Here is how to call:\n```xml\n" + toolCallsBlock(inner) + "\n```"
	if SuspectedDSMLToolCalls(text) {
		t.Errorf("SuspectedDSMLToolCalls = true, want false (quoted example)")
	}
}

func TestInjectStrictWarning(t *testing.T) {
	plain := formatDSMLToolResult(&toolcall.ToolContent{Result: "ok"})
	warned := formatDSMLToolResult(&toolcall.ToolContent{Result: "ok", Warning: "note"})
	// An EMPTY block cannot come out of formatDSMLToolResult (it normalizes
	// to (no output)); guard the defensive pass-through with a hand-built one.
	empty := "<tool_result>{}</tool_result>"

	if got := InjectStrictWarning(nil); got != nil {
		t.Errorf("nil input: got %v, want nil", got)
	}
	if got := InjectStrictWarning([]string{}); len(got) != 0 {
		t.Errorf("empty input: got %v, want empty", got)
	}

	out := InjectStrictWarning([]string{plain, warned, empty})
	if len(out) != 3 {
		t.Fatalf("got %d outputs, want 3", len(out))
	}
	if !strings.Contains(out[0], StrictWarning) {
		t.Errorf("output[0] missing StrictWarning:\n%s", out[0])
	}
	if !strings.Contains(out[1], "note") || !strings.Contains(out[1], StrictWarning) {
		t.Errorf("output[1] must keep existing warning AND append StrictWarning:\n%s", out[1])
	}
	if out[2] != empty {
		t.Errorf("empty block must stay untouched, got:\n%s", out[2])
	}
}

func TestInjectStrictWarningUnparseable(t *testing.T) {
	weird := "<tool_result>not-json</tool_result>"
	out := InjectStrictWarning([]string{weird})
	if len(out) != 1 || out[0] != weird {
		t.Errorf("unparseable block must pass through verbatim, got %v", out)
	}
}

// brokenWrapperReply is the full corrupt emission from the field: the model
// wrote </shell> instead of </tool_calls>, then continued with prose plus a
// stray </shell>. The single shell call is complete and must parse.
func brokenWrapperReply() string {
	return "<tool_calls>\n" +
		invokeBlock("shell",
			paramX("script", "go test ./internal/dsml/ -run 'TestMessageContent01|TestParseDSMLMessage' -v 2>&1 | tail -60", "true")+
				paramX("summary", "Run DSML message tests", "true")+
				paramX("timeout", "120", "false")) +
		"\n</shell>\n\n✅ 代码审查结果\n</shell>\n✅ CodeReview 执行成功"
}

func TestParseDSMLMessageBrokenWrapperCall(t *testing.T) {
	// The wrapper is malformed (</shell> instead of </tool_calls>) and stray
	// close tags follow, but one shell call parses. The ONLY authority for
	// "is this a tool-call reply" is len(ToolCalls) != 0, so OK=false
	// (violations) while the call is kept and the content stays verbatim.
	text := brokenWrapperReply()
	msg := ParseDSMLMessage("", text)
	if msg.OK {
		t.Error("OK = true, want false (broken wrapper and stray closes)")
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(msg.ToolCalls))
	}
	if msg.ToolCalls[0].Function.Name != "shell" {
		t.Errorf("tool name = %q, want shell", msg.ToolCalls[0].Function.Name)
	}
	if msg.Content != text {
		t.Errorf("Content must stay verbatim when OK=false, got:\n%q", msg.Content)
	}
	if SuspectedDSMLToolCalls(text) {
		t.Error("SuspectedDSMLToolCalls = true, want false (calls parsed)")
	}
}

func TestParseDSMLMessageBadgeRenderedAttempt(t *testing.T) {
	// A badge-rendered emission whose invoke open collapsed into a parameter
	// tag: wrapper + named parameter + </invoke>, no <invoke name=...> open.
	// Nothing parses, but the attempt residue must be suspected and OK=false.
	badge := "<" + "tool_calls>\n" +
		"<" + "parameter name=\"read_file\">\n" +
		paramX("path", "AGENTS.md", "true") + "\n" +
		"<" + "/invoke>\n" +
		"<" + "/tool_calls>"
	msg := ParseDSMLMessage("", badge)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %d, want 0", len(msg.ToolCalls))
	}
	if msg.OK {
		t.Error("OK = true, want false (badge-rendered attempt residue)")
	}
	if !SuspectedDSMLToolCalls(badge) {
		t.Error("SuspectedDSMLToolCalls = false, want true (badge-rendered attempt)")
	}
}

func TestParseDSMLMessageEmptyWrapperNotSuspected(t *testing.T) {
	text := "<" + "tool_calls>" + "<" + "/tool_calls>"
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %d, want 0", len(msg.ToolCalls))
	}
	if !msg.OK {
		t.Error("OK = false, want true (empty wrapper is not an attempt)")
	}
	if SuspectedDSMLToolCalls(text) {
		t.Error("SuspectedDSMLToolCalls = true, want false (empty wrapper)")
	}
}

func TestParseDSMLMessageInlineCodeQuoteNotSuspected(t *testing.T) {
	inner := "<" + "tool_calls>" +
		paramX("x", "y", "true") +
		"<" + "/invoke>" +
		"<" + "/tool_calls>"
	text := "`" + inner + "`"
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %d, want 0", len(msg.ToolCalls))
	}
	if !msg.OK {
		t.Error("OK = false, want true (inline code quote)")
	}
	if SuspectedDSMLToolCalls(text) {
		t.Error("SuspectedDSMLToolCalls = true, want false (inline code quote)")
	}
}

func TestParseDSMLMessageFencedCodeQuoteNotSuspected(t *testing.T) {
	inner := "<" + "tool_calls>" +
		paramX("x", "y", "true") +
		"<" + "/invoke>" +
		"<" + "/tool_calls>"
	text := "Here is how:\n```xml\n" + inner + "\n```"
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %d, want 0", len(msg.ToolCalls))
	}
	if !msg.OK {
		t.Error("OK = false, want true (fenced code quote)")
	}
	if SuspectedDSMLToolCalls(text) {
		t.Error("SuspectedDSMLToolCalls = true, want false (fenced code quote)")
	}
}

func TestParseDSMLMessageProseWrapperMentionNotSuspected(t *testing.T) {
	text := "Remember to wrap tool calls in tool_calls."
	msg := ParseDSMLMessage("", text)
	if len(msg.ToolCalls) != 0 {
		t.Fatalf("ToolCalls = %d, want 0", len(msg.ToolCalls))
	}
	if !msg.OK {
		t.Error("OK = false, want true (plain prose)")
	}
	if SuspectedDSMLToolCalls(text) {
		t.Error("SuspectedDSMLToolCalls = true, want false (prose wrapper mention)")
	}
}

// TestHasUnquotedAttemptShapesBranches pins the close-only and _calls-twin
// variants of the attempt-shape detector, plus the fenced-code exemption.
func TestHasUnquotedAttemptShapesBranches(t *testing.T) {
	closeOnly := "<" + "tool_calls>\n" +
		"<" + "/invoke>\n" +
		"<" + "/tool_calls>"

	callsTwin := "<" + "_calls>\n" +
		"<" + "parameter name=\"read_file\">\n" +
		"<" + "/invoke>\n" +
		"<" + "/_calls>"

	fenced := "Here is how:\n```xml\n" +
		"<" + "tool_calls>\n" +
		"<" + "/invoke>\n" +
		"<" + "/tool_calls>\n" +
		"```"

	cases := []struct {
		name        string
		text        string
		wantOK      bool
		wantSuspect bool
	}{
		{"close-only", closeOnly, false, true},
		{"_calls twin", callsTwin, false, true},
		{"fenced close-only", fenced, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msg := ParseDSMLMessage("", c.text)
			if len(msg.ToolCalls) != 0 {
				t.Fatalf("ToolCalls = %d, want 0", len(msg.ToolCalls))
			}
			if msg.OK != c.wantOK {
				t.Errorf("OK = %v, want %v", msg.OK, c.wantOK)
			}
			if SuspectedDSMLToolCalls(c.text) != c.wantSuspect {
				t.Errorf("SuspectedDSMLToolCalls = %v, want %v", SuspectedDSMLToolCalls(c.text), c.wantSuspect)
			}
		})
	}
}
