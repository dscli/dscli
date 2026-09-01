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
