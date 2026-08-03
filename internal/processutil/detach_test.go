//go:build !windows

package processutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestRunCommandBackgroundDetaches verifies that the display command starts
// in its own session with no controlling terminal, so it survives the
// caller's exit: terminal-close SIGHUP only reaches the caller's
// process group, never a detached child.
func TestRunCommandBackgroundDetaches(t *testing.T) {
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("ps not available")
	}
	// project must be a real directory now — it becomes cmd.Dir.  The
	// marker file lives outside it; the script writes to the marker's
	// absolute path passed as $1 (test-controlled, no metacharacters).
	project := t.TempDir()
	marker := filepath.Join(t.TempDir(), "sid")
	script := filepath.Join(t.TempDir(), "sid.sh")
	// The command records its own sid+tty on the first line and the
	// caller's sid on the second, then exits.
	body := "#!/bin/sh\n{ ps -o sid=,tty= -p $$; ps -o sid= -p $PPID; } > \"$1\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := RunCommandBackground(project, script, marker); err != nil {
		t.Fatal(err)
	}

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

// TestRunCommandBackgroundNoInjection verifies the project path never enters
// the command's argv: it travels only as cmd.Dir.  Under the old sh -c
// template it was passed as $1, and before that %s interpolation executed
// $(...) inside a double-quoted template.  With exec.Command there is no
// shell layer at all — the test proves the malicious path is used
// verbatim as the working directory and produces no side effects.
func TestRunCommandBackgroundNoInjection(t *testing.T) {
	base := t.TempDir()
	pwned := filepath.Join(base, "pwned")
	evil := filepath.Join(base, "$(touch pwned)") // executed by the old %s interpolation
	if err := os.Mkdir(evil, 0o755); err != nil {
		t.Fatalf("mkdir malicious project dir: %v", err)
	}
	out := filepath.Join(t.TempDir(), "cwd.txt")
	script := filepath.Join(t.TempDir(), "capture-cwd.sh")
	// The script records its working directory (cmd.Dir = evil) into a
	// fixed marker file; the evil path appears nowhere in the command.
	if err := os.WriteFile(script, []byte("#!/bin/sh\npwd > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := RunCommandBackground(evil, script, out); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var data []byte
	for {
		d, err := os.ReadFile(out)
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
		t.Errorf("command cwd = %q, want %q (project path used verbatim)", got, evil)
	}
	// `touch pwned` would run in the shell's cwd (= cmd.Dir, the evil
	// directory); check both the project dir and its parent.
	for _, path := range []string{filepath.Join(evil, "pwned"), pwned} {
		if _, err := os.Stat(path); err == nil {
			t.Fatalf("shell metacharacters in project path were executed (%s created)", path)
		}
	}
}

// TestRunCommandBackgroundUsesProjectDir verifies the display command runs
// with the target project as its working directory — that is the handoff
// channel to the Emacs side (emacsclient -e evaluates with
// default-directory following the client's cwd), replacing the old
// handoff file.  A shell `pwd` is captured to prove the cwd.
func TestRunCommandBackgroundUsesProjectDir(t *testing.T) {
	project := t.TempDir()
	out := filepath.Join(t.TempDir(), "cwd.txt")
	script := filepath.Join(t.TempDir(), "capture-cwd.sh")
	// The script records its own working directory into the marker file
	// passed as $1.  RunCommandBackground executes it directly (no shell
	// wrapper), so the captured cwd proves cmd.Dir == project.
	if err := os.WriteFile(script, []byte("#!/bin/sh\npwd > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := RunCommandBackground(project, script, out); err != nil {
		t.Fatal(err)
	}

	// The command starts async children; poll for the capture file.
	deadline := time.Now().Add(2 * time.Second)
	var data []byte
	for {
		var err error
		data, err = os.ReadFile(out)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("capture file %s not written: %v", out, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := strings.TrimSpace(string(data)); got != project {
		t.Errorf("command cwd = %q, want %q", got, project)
	}
}
