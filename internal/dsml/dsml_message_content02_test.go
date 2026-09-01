package dsml

import (
	_ "embed"
	"testing"
)

//go:embed "testdata/content02.md"
var content02 string

//go:embed "testdata/reasoning02.md"
var reasoning02 string

func TestMessageContent02(t *testing.T) {
	message := ParseDSMLMessage(reasoning02, content02)
	if message.OK || len(message.ToolCalls) != 1 {
		t.Fatal(message.OK, len(message.ToolCalls))
	}
	if message.ToolCalls[0].Function.Name != "shell" {
		t.Fatal(message.ToolCalls)
	}
	if message.Content != content02 {
		t.Fatal(message.Content, content02)
	}

	if message.ReasoningContent != reasoning02 {
		t.Fatal(message.ReasoningContent)
	}

	if SuspectedDSMLToolCalls(content02) {
		t.Error("SuspectedDSMLToolCalls = true, want false (quoted/referenced examples)")
	}
}
