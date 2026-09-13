package shellblock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunWritesAndExecutes(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(t.Context(), dir, "#!/bin/bash\necho hello\nexit 3", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Number != 1 {
		t.Errorf("number = %d, want 1", res.Number)
	}
	if res.ExitCode != 3 {
		t.Errorf("exit code = %d, want 3", res.ExitCode)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Errorf("output = %q, want hello", res.Output)
	}
	script, err := os.ReadFile(res.ScriptPath)
	if err != nil {
		t.Fatalf("read script file: %v", err)
	}
	if got, want := string(script), "#!/bin/bash\necho hello\nexit 3\n"; got != want {
		t.Errorf("script file = %q, want %q", got, want)
	}
	backfill, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatalf("read backfill file: %v", err)
	}
	for _, want := range []string{
		"bash script1.sh:",
		"```xml",
		"#!/bin/bash",
		"```\noutput:",
		"hello",
		"exit code: 3",
	} {
		if !strings.Contains(string(backfill), want) {
			t.Errorf("backfill misses %q:\n%s", want, backfill)
		}
	}
}

func TestRunNumbering(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"script3.sh", "script3.txt", "script7.sh", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(t.Context(), dir, "echo x", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Number != 8 {
		t.Errorf("number = %d, want 8 (max existing 7 + 1)", res.Number)
	}
}

func TestRunMergedOutput(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(t.Context(), dir, "echo out; echo err 1>&2; exit 0", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !strings.Contains(res.Output, "out") || !strings.Contains(res.Output, "err") {
		t.Errorf("merged output = %q, want both streams", res.Output)
	}
}

func TestRunTimeout(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	res, err := Run(t.Context(), dir, "sleep 10", 250*time.Millisecond)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	if res.ExitCode != -1 {
		t.Errorf("exit code = %d, want -1 (killed)", res.ExitCode)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run took %v, want a prompt kill", elapsed)
	}
	backfill, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatalf("read backfill: %v", err)
	}
	if !strings.Contains(string(backfill), "exit code: killed (timeout 250ms)") {
		t.Errorf("backfill misses the kill note:\n%s", backfill)
	}
}

// TestRunKillsProcessGroup: a script that parks background children must be
// killed as a group, not waited on - the round returns promptly.
func TestRunKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	res, err := Run(t.Context(), dir, "sleep 30 & sleep 30 & wait", 300*time.Millisecond)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run took %v, want a group kill", elapsed)
	}
}

func TestRunOutputTruncation(t *testing.T) {
	dir := t.TempDir()
	res, err := Run(t.Context(), dir, "head -c 300000 /dev/zero | tr '\\0' 'a'", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.Truncated {
		t.Error("Truncated = false, want true")
	}
	if len(res.Output) > maxOutputBytes {
		t.Errorf("output = %d bytes, want <= %d", len(res.Output), maxOutputBytes)
	}
	backfill, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatalf("read backfill: %v", err)
	}
	if !strings.Contains(string(backfill), "[output truncated at 256KB]") {
		t.Errorf("backfill misses the truncation note")
	}
}

func TestRunEmptyScript(t *testing.T) {
	if _, err := Run(t.Context(), t.TempDir(), "   \n", 30*time.Second); err == nil {
		t.Error("Run with an empty script must fail")
	}
}

func TestFormatBackfillNoOutput(t *testing.T) {
	got := FormatBackfill(65, "true\n", &RunResult{ExitCode: 0})
	for _, want := range []string{"bash script65.sh:", "```xml\ntrue\n```", "(no output)", "exit code: 0"} {
		if !strings.Contains(got, want) {
			t.Errorf("FormatBackfill misses %q:\n%s", want, got)
		}
	}
}

func TestClampTimeout(t *testing.T) {
	if got := clampTimeout(0); got != DefaultTimeout {
		t.Errorf("clampTimeout(0) = %v, want %v", got, DefaultTimeout)
	}
	if got := clampTimeout(-time.Second); got != DefaultTimeout {
		t.Errorf("clampTimeout(-1s) = %v, want %v", got, DefaultTimeout)
	}
	if got := clampTimeout(MaxTimeout + time.Second); got != MaxTimeout {
		t.Errorf("clampTimeout(over cap) = %v, want %v", got, MaxTimeout)
	}
	if got := clampTimeout(300 * time.Second); got != 300*time.Second {
		t.Errorf("clampTimeout(300s) = %v, want 300s", got)
	}
}
