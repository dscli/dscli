package dsml

import (
	_ "embed"
	"testing"
)

//go:embed "testdata/content01.md"
var content01 string

//go:embed "testdata/reasoning01.md"
var reasoning01 string

func TestMessageContent01(t *testing.T) {
	message := ParseDSMLMessage(reasoning01, content01)
	if !message.OK || len(message.ToolCalls) != 0 {
		t.Fatal(message.OK, len(message.ToolCalls))
	}

	if message.Content != content01 {
		t.Fatal(message.Content)
	}

	if message.ReasoningContent != reasoning01 {
		t.Fatal(message.ReasoningContent)
	}

	if SuspectedDSMLToolCalls(content01) {
		t.Error("SuspectedDSMLToolCalls = true, want false (quoted/referenced examples)")
	}
}
