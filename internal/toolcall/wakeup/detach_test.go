//go:build !windows

package wakeup

import (
	"os"
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
	marker := filepath.Join(t.TempDir(), "sid")
	// The template's %s carries the (escaped) project path; reuse it as
	// the marker file path.  The command records its own sid+tty on the
	// first line and the caller's sid on the second, then exits.
	runDisplayCommand(
		`{ ps -o sid=,tty= -p $$; ps -o sid= -p $PPID; } > "%s"`, marker,
	)

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("display command did not create marker file within 5s")
		}
		time.Sleep(20 * time.Millisecond)
	}

	data, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		t.Fatalf("ps output %q: want <child sid tty> and <caller sid> lines", data)
	}

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
