package dsml

import (
	_ "embed"
	"testing"
)

//go:embed "testdata/content02.md"
var content02 string

//go:embed "testdata/reasoning02.md"
var reasoning02 string

// TestMessageContent02 replays a real field round: the model wrote the
// shell close tag (</shell>) instead of the tool_calls close, then kept
// going with prose plus a stray </shell>. One shell call parses, so OK must
// be false (broken wrapper + stray closes) while the content stays verbatim.
func TestMessageContent02(t *testing.T) {
	message := ParseDSMLMessage(reasoning02, content02)
	if message.OK || len(message.ToolCalls) != 1 {
		t.Fatalf("OK=%v ToolCalls=%d, want OK=false and 1 call", message.OK, len(message.ToolCalls))
	}
	if message.ToolCalls[0].Function.Name != "shell" {
		t.Fatalf("tool name = %q, want shell", message.ToolCalls[0].Function.Name)
	}
	if message.Content != content02 {
		t.Errorf("Content modified when OK=false: got %d bytes, want the original %d bytes verbatim", len(message.Content), len(content02))
	}

	if message.ReasoningContent != reasoning02 {
		t.Errorf("ReasoningContent modified: got %d bytes, want the original %d bytes verbatim", len(message.ReasoningContent), len(reasoning02))
	}

	if SuspectedDSMLToolCalls(content02) {
		t.Error("SuspectedDSMLToolCalls = true, want false (calls parsed)")
	}
}

//go:embed "testdata/content03.md"
var content03 string

//go:embed "testdata/reasoning03.md"
var reasoning03 string

func TestMessageContent03(t *testing.T) {
	message := ParseDSMLMessage(reasoning03, content03)
	if message.OK || len(message.ToolCalls) != 0 {
		t.Fatalf("OK=%v ToolCalls=%d, want OK=false and 0 call", message.OK, len(message.ToolCalls))
	}
	if !SuspectedDSMLToolCalls(content03) {
		t.Error("SuspectedDSMLToolCalls = false, want true (badge-rendered attempt residue)")
	}
	if message.Content != content03 {
		t.Errorf("Content modified: got %d bytes, want the original %d bytes verbatim", len(message.Content), len(content03))
	}
	if message.ReasoningContent != reasoning03 {
		t.Errorf("ReasoningContent modified: got %d bytes, want the original %d bytes verbatim", len(message.ReasoningContent), len(reasoning03))
	}
}
