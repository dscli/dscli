package dsml

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

// registerExecShellForTest registers a local shell tool that really runs the
// script via sh. It deliberately does NOT blank-import the production shell
// package: that would register "shell" globally for every test in this
// package, colliding with the probe tools the parse/doc tests register.
func registerExecShellForTest(t *testing.T) {
	t.Helper()
	if err := toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "shell",
		Description: "Run a shell script.",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{"type": "string", "description": "Shell script content."},
			},
			"required": []string{"script"},
		},
		Handler: func(_ context.Context, args toolcall.ToolArgs) (string, string, error) {
			script, _ := args["script"].(string)
			out, err := exec.Command("sh", "-c", script).CombinedOutput()
			if err != nil {
				return "", "", err
			}
			return strings.TrimSpace(string(out)), "", nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { toolcall.UnregisterTool("shell") })
}

// TestExecuteDSMLToolCalls runs a real harmless shell command through the
// framework end to end: parse DSML -> normalize -> HandleToolCall -> feedback.
func TestExecuteDSMLToolCalls(t *testing.T) {
	registerExecShellForTest(t)
	calls, err := ParseDSMLToolCalls(`<invoke name="shell">
<parameter name="script" string="true">printf 'hello dsml'</parameter>
<parameter name="timeout" string="false">5000</parameter>
</invoke>`)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	outputs := ExecuteDSMLToolCalls(context.Background(), calls)
	if len(outputs) != 1 {
		t.Fatalf("got %d outputs, want 1", len(outputs))
	}
	if !strings.Contains(outputs[0], "hello dsml") {
		t.Errorf("output missing command stdout:\n%s", outputs[0])
	}
}

func TestExecuteDSMLToolCallsUnsupported(t *testing.T) {
	// An unregistered name (a quoted example, not an executable call) is
	// skipped silently: no execution, no "unknown tool" feedback block
	// for the expert to argue with.
	text := `<invoke name="write_file">
<parameter name="path" string="true">/tmp/x</parameter>
</invoke>`
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	outputs := ExecuteDSMLToolCalls(context.Background(), calls)
	if len(outputs) != 0 {
		t.Errorf("outputs = %v, want none (silently skipped)", outputs)
	}
}
