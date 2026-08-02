//go:build !windows

package wakeup

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunDisplayCommandDetaches verifies that the display command starts
// in its own session with no controlling terminal, so it survives the
// caller's exit: terminal-close SIGHUP only reaches the caller's
// process group, never a detached child.
func TestRunDisplayCommandDetaches(t *testing.T) {
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("ps not available")
	}
	marker := filepath.Join(t.TempDir(), "sid")
	// The template's project path ($1) is reused as the marker file.
	// The command records its own sid+tty on the first line and the
	// caller's sid on the second, then exits.
	runDisplayCommand(
		`{ ps -o sid=,tty= -p $$; ps -o sid= -p $PPID; } > "$1"`, marker,
	)

	// The child opens the marker immediately on redirection but writes
	// output afterwards; poll for complete content, not just existence.
	deadline := time.Now().Add(5 * time.Second)
	var data []byte
	for {
		d, err := os.ReadFile(marker)
		if err == nil && len(strings.Split(strings.TrimSpace(string(d)), "\n")) >= 2 {
			data = d
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("display command did not produce complete output within 5s (last read: %q)", d)
		}
		time.Sleep(20 * time.Millisecond)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	child := strings.Fields(lines[0])
	if len(child) < 2 {
		t.Fatalf("child line %q: want <sid> <tty>", lines[0])
	}
	callerSid := strings.Fields(lines[1])
	if len(callerSid) < 1 {
		t.Fatalf("caller line %q: want <sid>", lines[1])
	}

	if got := child[0]; got == callerSid[0] {
		t.Errorf("child sid = %s, caller sid = %s: child still in caller session", got, callerSid[0])
	}
	// No controlling terminal: GNU ps prints "?", macOS prints "??".
	if tty := child[1]; tty != "?" && tty != "??" {
		t.Errorf("child controlling tty = %q, want ? (none)", tty)
	}
}

// TestRunDisplayCommandNoInjection verifies the project path is passed
// as a positional argument, so shell metacharacters in the path are
// never interpreted.  The old %s interpolation executed $(...) inside a
// double-quoted template.
func TestRunDisplayCommandNoInjection(t *testing.T) {
	pwned := filepath.Join(t.TempDir(), "pwned")
	marker := filepath.Join(t.TempDir(), "out")
	evil := "$(touch " + pwned + ")" // executed by the old %s interpolation

	// The template echoes $1 verbatim into a fixed marker file; the
	// project path appears only as a positional argument.
	runDisplayCommand(`{ echo "$1"; } > "`+marker+`"`, evil)

	deadline := time.Now().Add(5 * time.Second)
	var data []byte
	for {
		d, err := os.ReadFile(marker)
		if err == nil && len(d) > 0 {
			data = d
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("marker file not populated within 5s (last read: %q)", d)
		}
		time.Sleep(20 * time.Millisecond)
	}

	if got := strings.TrimSpace(string(data)); got != evil {
		t.Errorf("project path was mangled: got %q, want %q", got, evil)
	}
	if _, err := os.Stat(pwned); err == nil {
		t.Fatal("shell metacharacters in project path were executed")
	}
}
