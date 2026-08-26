package ask

import (
	"context"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
	// Blank import registers the shell tool. It must live in a package that
	// has no "registry must be empty" assumption (internal/toolcall's own
	// TestRegisterToolAndGetAllTools would fail) and where importing the
	// shell package creates no import cycle (shell -> toolcall only).
	_ "github.com/dscli/dscli/internal/toolcall/shell"
)

// TestExecuteDSMLToolCalls runs a real harmless shell command through the
// framework end to end: parse DSML -> normalize -> HandleToolCall -> feedback.
func TestExecuteDSMLToolCalls(t *testing.T) {
	calls, err := toolcall.ParseDSMLToolCalls(`<invoke name="exec_command">
<parameter name="cmd" string="true">printf 'hello dsml'</parameter>
<parameter name="timeout" string="false">5000</parameter>
</invoke>`)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	outputs := toolcall.ExecuteDSMLToolCalls(context.Background(), calls)
	if len(outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(outputs))
	}
	if !strings.Contains(outputs[0], "hello dsml") {
		t.Errorf("output missing command stdout:\n%s", outputs[0])
	}
}

func TestExecuteDSMLToolCallsUnsupported(t *testing.T) {
	// A non-whitelisted name (a quoted example, not an executable call) is
	// skipped silently: no execution, no "unsupported tool" feedback block
	// for the expert to argue with.
	text := `<invoke name="write_file">
<parameter name="path" string="true">/tmp/x</parameter>
</invoke>`
	calls, err := toolcall.ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	outputs := toolcall.ExecuteDSMLToolCalls(context.Background(), calls)
	if len(outputs) != 0 {
		t.Errorf("outputs = %v, want none (silently skipped)", outputs)
	}
}
