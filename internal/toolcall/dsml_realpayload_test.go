package toolcall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseDSMLToolCallsRealPayloads pins the two production payloads that
// triggered "exec_command missing parameter cmd" (21:10 report). Both embed
// DSML-looking text inside the cmd VALUE:
//
//   - real_sample1.txt: a Python heredoc carrying '<invoke name="a">' and
//     '<invoke name="b">' fragments. The old non-greedy <invoke> block regex
//     stopped at the value's first '</invoke>', so the block body ended before
//     any complete <parameter> and the call lost its cmd ("missing parameter
//     cmd" in practice).
//   - real_sample2.txt: a shell snippet with '</||DSML||invoke>' (normalized
//     to '</invoke>' before parsing) plus "cat <dsml_config" - it must survive
//     as literal content inside the command.
//
// The stack scan (dsmlBlockRanges) treats <parameter> bodies as opaque, so
// both calls parse with their full cmd and every parameter after it.
func TestParseDSMLToolCallsRealPayloads(t *testing.T) {
	for _, name := range []string{"real_sample1.txt", "real_sample2.txt"} {
		text, err := os.ReadFile(filepath.Join("testdata", name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		calls, err := ParseDSMLToolCalls(string(text))
		if err != nil {
			t.Fatalf("%s: ParseDSMLToolCalls: %v", name, err)
		}
		if len(calls) != 1 {
			t.Fatalf("%s: got %d calls, want 1", name, len(calls))
		}
		tool, args, err := normalizeDSMLInvoke(calls[0])
		if err != nil {
			t.Fatalf("%s: normalizeDSMLInvoke: %v", name, err)
		}
		if tool != "shell" {
			t.Fatalf("%s: tool = %q, want shell", name, tool)
		}
		script, ok := args["script"].(string)
		if !ok || strings.TrimSpace(script) == "" {
			t.Fatalf("%s: args[script] missing/empty (args=%v)", name, args)
		}
		// The full heredoc must survive: the sample marker is the last line
		// of the command.
		if !strings.Contains(script, "GOEOF") && !strings.Contains(script, "PY'") {
			t.Fatalf("%s: script looks truncated: %q...", name, script[:min(60, len(script))])
		}
		if _, ok := args["summary"].(string); !ok {
			t.Errorf("%s: summary missing (justification must survive)", name)
		}
	}
}
