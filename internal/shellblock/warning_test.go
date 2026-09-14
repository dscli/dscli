package shellblock

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestMalformedWarning(t *testing.T) {
	got := MalformedWarning("the `<shell>` line is missing", false)
	for _, want := range []string{
		"malformed `<shell>` block",
		"the `<shell>` line is missing",
		"1. `<shell>` must be the leading line of the shell block and it fills the whole line,",
		"2. `</shell>` must be the ending line of the shell block and it fills the whole line,",
		"3. `<script>` must be the leading line of the script block and it fills the whole line,",
		"4. `</script>` must be the ending line of the script block and it fills the whole line.",
		"<summary>say hello</summary>",
		"Re-send the whole block exactly in this shape.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("MalformedWarning misses %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Do not use the DSML shape") {
		t.Errorf("MalformedWarning must not carry the DSML note when the reply has no DSML shapes:\n%s", got)
	}
	withDSML := MalformedWarning("issue", true)
	if !strings.Contains(withDSML, "Do not use the DSML shape") {
		t.Errorf("MalformedWarning with dsmlShape misses the note:\n%s", withDSML)
	}
}

func TestNoBlockWarning(t *testing.T) {
	got := NoBlockWarning(false)
	if !strings.Contains(got, "neither contains a `<shell>` block nor reads as a final report") {
		t.Errorf("NoBlockWarning = %q", got)
	}
	if strings.Contains(got, "Do not use the DSML shape") {
		t.Errorf("NoBlockWarning must not carry the DSML note:\n%s", got)
	}
	if !strings.Contains(NoBlockWarning(true), "Do not use the DSML shape") {
		t.Error("NoBlockWarning with dsmlShape misses the note")
	}
}

func TestDSMLShapeWarning(t *testing.T) {
	got := DSMLShapeWarning()
	for _, want := range []string{
		"DSML tool-call shape",
		"NOT executed",
		"Use ONE `<shell>` block instead",
		"Example:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("DSMLShapeWarning misses %q:\n%s", want, got)
		}
	}
}

func TestBlockedWarning(t *testing.T) {
	got := BlockedWarning("sudo")
	for _, want := range []string{
		"NOT executed",
		"`sudo`",
		"rejected outright",
		"Rewrite the script without them and re-send the block.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("BlockedWarning misses %q:\n%s", want, got)
		}
	}
	long := BlockedWarning(strings.Repeat("汉", 200))
	if !strings.Contains(long, "...") {
		t.Errorf("BlockedWarning must truncate a long detail:\n%s", long)
	}
	if !utf8.ValidString(long) {
		t.Error("BlockedWarning truncation must not split a multi-byte rune")
	}
}

// TestBlockedSkipsAttachBlocks locks the mode exemption: an `#!attach` path
// list executes nothing, so shell-text patterns must not false-refuse it
// (and must not burn the loop's consecutive-warning slots).
func TestBlockedSkipsAttachBlocks(t *testing.T) {
	for _, script := range []string{
		"#!attach\nreboot-notes.md\n",
		"#!attach\nwget-report.txt\nnc.md\n",
		"#!attach\nsudo rm.sh\n",
	} {
		if detail, blocked := Blocked(script); blocked {
			t.Errorf("Blocked(%q) = %q, want the attach exemption", script, detail)
		}
	}
	if _, blocked := Blocked("sudo rm -rf /"); !blocked {
		t.Error("Blocked must still refuse a destructive bash script")
	}
}

// TestNoPythonWarning pins the fallback contract for a host without python.
func TestNoPythonWarning(t *testing.T) {
	got := NoPythonWarning()
	for _, want := range []string{"NOT executed", "no matching python interpreter", "bash"} {
		if !strings.Contains(got, want) {
			t.Errorf("NoPythonWarning misses %q:\n%s", want, got)
		}
	}
}

func TestBuildToolDoc(t *testing.T) {
	doc := BuildToolDoc()
	for _, want := range []string{
		"## 🛠️ Available Tools: `shell`",
		"**Usage: run a bash or python script, or attach files**",
		"1. `<shell>` must be the leading line of the shell block and it fills the whole line,",
		"4. `</script>` must be the ending line of the script block and it fills the whole line.",
		"**Script forms**",
		pythonExample,
		attachExample,
		"**Iterate inside the script, not across rounds**",
		"set -euo pipefail",
		"exit status is",
		"rejected outright",
		"`<summary>`: <= 40 chars",
		"`<timeout>`: default 120s",
		"**Finishing**: when the task is complete, write the final report as plain",
	} {
		if !strings.Contains(doc, want) {
			t.Errorf("BuildToolDoc misses %q", want)
		}
	}
}
