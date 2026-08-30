package dsml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestParseDSMLToolCallsRealPayloads pins the two production payloads that
// historically triggered "missing parameter cmd" (21:10 report); they
// carry the pre-rename spelling (exec_command + cmd + justification). Both
// embed DSML-looking text inside the cmd VALUE:
//
//   - real_sample1.txt: a Python heredoc carrying '<invoke name="a">' and
//     '<invoke name="b">' fragments. The old non-greedy <invoke> block regex
//     stopped at the value's first '</invoke>', so the block body ended
//     before any complete <parameter> and the call lost its cmd ("missing
//     parameter cmd" in practice).
//   - real_sample2.txt: a shell snippet with '</||DSML||invoke>' (normalized
//     to '</invoke>' before parsing) plus "cat <dsml_config" - it must survive
//     as literal content inside the command.
//
// The stack scan (dsmlBlockRanges) treats <parameter> bodies as opaque, so
// both calls parse with their full cmd and every parameter after it.
// normalizeDSMLInvoke is a verbatim passthrough here (name and args keep
// their raw keys; only the decorative justification is stripped) - the
// executor's role allow-set decides executability, not the DSML layer.
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
		// No mapping anymore: the DSML layer passes the raw spelling
		// through; the role allow-set is the only gate.
		if tool != calls[0].Name {
			t.Fatalf("%s: tool = %q, want passthrough %q", name, tool, calls[0].Name)
		}
		cmd, ok := args["cmd"].(string)
		if !ok || strings.TrimSpace(cmd) == "" {
			t.Fatalf("%s: args[cmd] missing/empty (args=%v)", name, args)
		}
		// The full heredoc must survive: the sample marker is the last line
		// of the command.
		if !strings.Contains(cmd, "GOEOF") && !strings.Contains(cmd, "PY'") {
			t.Fatalf("%s: cmd looks truncated: %q...", name, cmd[:min(60, len(cmd))])
		}
		if _, ok := args["justification"]; ok {
			t.Errorf("%s: justification must be stripped, got %v", name, args)
		}
	}
}
