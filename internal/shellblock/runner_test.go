package shellblock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dsctx "github.com/dscli/dscli/internal/context"
)

// isolateProjectRoot pins dsctx.ProjectRoot to a fresh test-owned directory
// (restored on cleanup) and returns it: the round artifacts then land under
// <root>/.dscli/tmp instead of this repository's spool directory.
func isolateProjectRoot(t *testing.T) string {
	t.Helper()
	orig := dsctx.ProjectRoot
	root := t.TempDir()
	dsctx.ProjectRoot = root
	t.Cleanup(func() { dsctx.ProjectRoot = orig })
	return root
}

// spoolDirIn returns the artifact directory Run uses under a test root.
func spoolDirIn(root string) string {
	return filepath.Join(root, ".dscli", "tmp")
}

func TestRunWritesAndExecutes(t *testing.T) {
	root := isolateProjectRoot(t)
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
	// The artifacts land in the project's spool directory, never in the
	// script's working directory.
	if want := filepath.Join(spoolDirIn(root), "script1.sh"); res.ScriptPath != want {
		t.Errorf("script path = %q, want %q", res.ScriptPath, want)
	}
	if want := filepath.Join(spoolDirIn(root), "script1.txt"); res.OutputPath != want {
		t.Errorf("output path = %q, want %q", res.OutputPath, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "script1.sh")); !os.IsNotExist(err) {
		t.Errorf("script file must not land in the working dir: %v", err)
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
	root := isolateProjectRoot(t)
	spool := spoolDirIn(root)
	if err := os.MkdirAll(spool, 0o700); err != nil {
		t.Fatal(err)
	}
	// The scripts' mode extensions all count, as do the .txt backfills; a
	// lookalike name (.bak, notes) does not.
	for _, name := range []string{
		"script3.sh", "script3.txt", "script7.sh", "script4.py", "script9.lst",
		"script11.sh.bak", "notes.txt",
	} {
		if err := os.WriteFile(filepath.Join(spool, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(t.Context(), t.TempDir(), "echo x", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Number != 10 {
		t.Errorf("number = %d, want 10 (max existing 9 + 1)", res.Number)
	}
}

func TestRunMergedOutput(t *testing.T) {
	isolateProjectRoot(t)
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
	isolateProjectRoot(t)
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
	isolateProjectRoot(t)
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
	isolateProjectRoot(t)
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
	isolateProjectRoot(t)
	if _, err := Run(t.Context(), t.TempDir(), "   \n", 30*time.Second); err == nil {
		t.Error("Run with an empty script must fail")
	}
}

// TestDetectScriptMode pins the first-line marker table, tolerance rules
// included: a stray space must never flip the interpreter.
func TestDetectScriptMode(t *testing.T) {
	tests := []struct {
		name   string
		script string
		want   scriptMode
	}{
		{name: "no shebang defaults to bash", script: "echo x\ntrue", want: modeBash},
		{name: "bash shebang", script: "#!/bin/bash\necho x", want: modeBash},
		{name: "sh shebang stays bash", script: "#!/bin/sh\necho x", want: modeBash},
		{name: "plain python word is not a marker", script: "echo python\ntrue", want: modeBash},
		{name: "python shebang", script: "#!/usr/bin/python\nprint(1)", want: modePython},
		{name: "env python shebang", script: "#!/usr/bin/env python\nprint(1)", want: modePython},
		{name: "python3 shebang", script: "#!/usr/bin/env python3\nprint(1)", want: modePython},
		{name: "indented python shebang", script: "  #!/usr/bin/python\nprint(1)", want: modePython},
		{name: "python shebang with trailing space", script: "#!/usr/bin/python \nprint(1)", want: modePython},
		{name: "versioned python shebang", script: "#!/usr/bin/env python3.11\nprint(1)", want: modePython},
		{name: "python2 shebang", script: "#!/usr/bin/python2.7\nprint(1)", want: modePython},
		{name: "micro python is not a marker", script: "#!/usr/bin/env micropython\nprint(1)", want: modeBash},
		{name: "custom python path is not a marker", script: "#!/opt/custompython\nprint(1)", want: modeBash},
		{name: "degenerate python name stays bash", script: "#!/usr/bin/python3..11\nprint(1)", want: modeBash},
		{name: "attach marker", script: "#!attach\nf.txt", want: modeAttach},
		{name: "attach marker with trailing space", script: "#!attach \nf.txt", want: modeAttach},
		{name: "attach marker alone", script: "#!attach", want: modeAttach},
		{name: "attach marker with first path", script: "#!attach a.png\nb.png", want: modeAttach},
		{name: "attach prefix word is not the marker", script: "#!attachments\nf.txt", want: modeBash},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectScriptMode(tt.script); got != tt.want {
				t.Errorf("detectScriptMode(%q) = %v, want %v", tt.script, got, tt.want)
			}
		})
	}
}

// TestPythonCandidates pins the interpreter lookup chains: a python2 name is
// never substituted with python3, a versioned python3 name is honored first,
// and the generic chain prefers python3.
func TestPythonCandidates(t *testing.T) {
	tests := []struct {
		token string
		want  []string
	}{
		{token: "python", want: []string{"python3", "python"}},
		{token: "python3", want: []string{"python3", "python"}},
		{token: "python3.11", want: []string{"python3.11", "python3", "python"}},
		{token: "python2", want: []string{"python2"}},
		{token: "python2.7", want: []string{"python2.7", "python2"}},
	}
	for _, tt := range tests {
		got := pythonCandidates(tt.token)
		if len(got) != len(tt.want) {
			t.Errorf("pythonCandidates(%q) = %v, want %v", tt.token, got, tt.want)
			continue
		}
		for i := range tt.want {
			if got[i] != tt.want[i] {
				t.Errorf("pythonCandidates(%q) = %v, want %v", tt.token, got, tt.want)
				break
			}
		}
	}
}

// TestRunPythonScript runs a real python script end to end; it skips when no
// interpreter exists.
func TestRunPythonScript(t *testing.T) {
	script := "#!/usr/bin/env python3\nimport sys\nprint(\"hello from python\")\nsys.exit(2)"
	if _, err := pythonInterpreter(script); err != nil {
		t.Skipf("no python interpreter: %v", err)
	}
	root := isolateProjectRoot(t)
	dir := t.TempDir()
	res, err := Run(t.Context(), dir, script, 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := filepath.Join(spoolDirIn(root), "script1.py"); res.ScriptPath != want {
		t.Errorf("script path = %q, want %q", res.ScriptPath, want)
	}
	if res.ExitCode != 2 {
		t.Errorf("exit code = %d, want 2", res.ExitCode)
	}
	if !strings.Contains(res.Output, "hello from python") {
		t.Errorf("output = %q, want the python print", res.Output)
	}
	if res.Attachments != nil {
		t.Errorf("attachments = %v, want nil for a python round", res.Attachments)
	}
	backfill, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatalf("read backfill: %v", err)
	}
	for _, want := range []string{"python script1.py:", "#!/usr/bin/env python3", "exit code: 2"} {
		if !strings.Contains(string(backfill), want) {
			t.Errorf("backfill misses %q:\n%s", want, backfill)
		}
	}
}

// TestRunAttachBlock pins the attach semantics: one status line per path,
// attachments resolved against dir in listed order, exit code 0, and the
// usual scriptN.lst / scriptN.txt pair for audit consistency.
func TestRunAttachBlock(t *testing.T) {
	root := isolateProjectRoot(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sub", "b.bin"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	script := "#!attach\na.txt\nsub/b.bin\nmissing.txt\nsub\n"
	res, err := Run(t.Context(), dir, script, 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Errorf("exit code = %d, want 0", res.ExitCode)
	}
	wantOutput := "a.txt attached\nsub/b.bin attached\nmissing.txt not exist\nsub not a regular file\n"
	if res.Output != wantOutput {
		t.Errorf("output = %q, want %q", res.Output, wantOutput)
	}
	wantAttachments := []string{filepath.Join(dir, "a.txt"), filepath.Join(dir, "sub", "b.bin")}
	if len(res.Attachments) != len(wantAttachments) {
		t.Fatalf("attachments = %v, want %v", res.Attachments, wantAttachments)
	}
	for i, want := range wantAttachments {
		if res.Attachments[i] != want {
			t.Errorf("attachment[%d] = %q, want %q", i, res.Attachments[i], want)
		}
	}
	if want := filepath.Join(spoolDirIn(root), "script1.lst"); res.ScriptPath != want {
		t.Errorf("script path = %q, want %q (an attach list is a .lst)", res.ScriptPath, want)
	}
	scriptFile, err := os.ReadFile(res.ScriptPath)
	if err != nil {
		t.Fatalf("read script file: %v", err)
	}
	if string(scriptFile) != script {
		t.Errorf("script file = %q, want %q", scriptFile, script)
	}
	backfill, err := os.ReadFile(res.OutputPath)
	if err != nil {
		t.Fatalf("read backfill: %v", err)
	}
	for _, want := range []string{"attach script1.lst:", "#!attach", "missing.txt not exist"} {
		if !strings.Contains(string(backfill), want) {
			t.Errorf("backfill misses %q:\n%s", want, backfill)
		}
	}
}

// TestRunAttachAbsolutePath: an absolute path bypasses dir resolution and is
// attached as-is.
func TestRunAttachAbsolutePath(t *testing.T) {
	isolateProjectRoot(t)
	abs := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(abs, []byte("png"), 0o600); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	res, err := Run(t.Context(), dir, "#!attach\n"+abs+"\n", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Attachments) != 1 || res.Attachments[0] != abs {
		t.Errorf("attachments = %v, want [%s]", res.Attachments, abs)
	}
	if res.Output != abs+" attached\n" {
		t.Errorf("output = %q, want %q", res.Output, abs+" attached\n")
	}
}

// TestRunAttachMarkerLinePath: a path on the marker line itself
// (`#!attach a.txt`) counts as the first entry; it must not fall back to
// bash and execute the list.
func TestRunAttachMarkerLinePath(t *testing.T) {
	isolateProjectRoot(t)
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	res, err := Run(t.Context(), dir, "#!attach a.txt\nb.txt\n", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantOutput := "a.txt attached\nb.txt attached\n"
	if res.Output != wantOutput {
		t.Errorf("output = %q, want %q", res.Output, wantOutput)
	}
	if len(res.Attachments) != 2 {
		t.Errorf("attachments = %v, want 2 entries", res.Attachments)
	}
}

// TestRunAttachLimit: a list past MaxAttachFiles reports the over-cap files
// instead of letting the upload batch fail fatally. Only files that would be
// ATTACHED count toward the cap: a missing path and a directory past the cap
// still report their own status.
func TestRunAttachLimit(t *testing.T) {
	isolateProjectRoot(t)
	dir := t.TempDir()
	var script strings.Builder
	script.WriteString("#!attach\n")
	for i := 0; i < MaxAttachFiles; i++ {
		name := fmt.Sprintf("f%02d.txt", i)
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&script, "%s\n", name)
	}
	script.WriteString("missing.txt\n")
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	script.WriteString("sub\n")
	over := fmt.Sprintf("f%02d.txt", MaxAttachFiles)
	if err := os.WriteFile(filepath.Join(dir, over), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&script, "%s\n", over)

	res, err := Run(t.Context(), dir, script.String(), 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.Attachments) != MaxAttachFiles {
		t.Errorf("attachments = %d, want the %d-file cap", len(res.Attachments), MaxAttachFiles)
	}
	for _, want := range []string{
		"missing.txt not exist\n",
		"sub not a regular file\n",
		fmt.Sprintf("%s not attached (attachment limit %d reached)\n", over, MaxAttachFiles),
	} {
		if !strings.Contains(res.Output, want) {
			t.Errorf("output misses %q:\n%s", want, res.Output)
		}
	}
}

// TestRunDefaultDirs: an empty dir runs the script with the project root as
// its working directory, while the artifacts land under
// PROJECT_ROOT/.dscli/tmp (created on demand) - never in the working
// directory.
func TestRunDefaultDirs(t *testing.T) {
	root := isolateProjectRoot(t)

	res, err := Run(t.Context(), "", "#!/bin/bash\ntouch exec_probe.txt\necho hi", 30*time.Second)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	wantScript := filepath.Join(spoolDirIn(root), "script1.sh")
	if res.ScriptPath != wantScript {
		t.Errorf("script path = %q, want %q", res.ScriptPath, wantScript)
	}
	if _, err := os.Stat(wantScript); err != nil {
		t.Errorf("script file not created: %v", err)
	}
	// The script watched the project root: its relative write landed there,
	// not in the artifact directory.
	if _, err := os.Stat(filepath.Join(root, "exec_probe.txt")); err != nil {
		t.Errorf("script cwd must be the project root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(spoolDirIn(root), "exec_probe.txt")); !os.IsNotExist(err) {
		t.Errorf("the working-dir probe must not land in the spool: %v", err)
	}
	if !strings.Contains(res.Output, "hi") {
		t.Errorf("output = %q, want the echo", res.Output)
	}
}

// TestFormatBackfillModeLabels: the header carries the detected mode and the
// mode's file extension (.sh / .py / .lst).
func TestFormatBackfillModeLabels(t *testing.T) {
	if got := FormatBackfill(2, "#!/usr/bin/python\nprint(1)\n", &RunResult{ExitCode: 0}); !strings.Contains(got, "python script2.py:") {
		t.Errorf("python backfill misses the mode header:\n%s", got)
	}
	if got := FormatBackfill(3, "#!attach\nfoo.txt\n", &RunResult{ExitCode: 0}); !strings.Contains(got, "attach script3.lst:") {
		t.Errorf("attach backfill misses the mode header:\n%s", got)
	}
	if got := FormatBackfill(4, "echo x\n", &RunResult{ExitCode: 0}); !strings.Contains(got, "bash script4.sh:") {
		t.Errorf("bash backfill misses the mode header:\n%s", got)
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

// TestRunBackgroundChildDoesNotOutliveTimeout pins the drain fix: a script
// that leaves a background child holding the stdout/stderr pipe must still
// return at the timeout, not at the child's exit.
func TestRunBackgroundChildDoesNotOutliveTimeout(t *testing.T) {
	isolateProjectRoot(t)
	dir := t.TempDir()
	start := time.Now()
	res, err := Run(t.Context(), dir, "sleep 30 &", 300*time.Millisecond)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !res.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run took %v, want a prompt return despite the background child", elapsed)
	}
}

// TestRunRefusesBlockedScript locks the fail-closed backstop: the runner
// itself never executes a destructive match.
func TestRunRefusesBlockedScript(t *testing.T) {
	isolateProjectRoot(t)
	dir := t.TempDir()
	_, err := Run(t.Context(), dir, "sudo rm -rf /", 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("err = %v, want the fail-closed blocked refusal", err)
	}
}

// TestRunContextCancellation: a cancelled context kills the group and
// returns promptly with context.Canceled.
func TestRunContextCancellation(t *testing.T) {
	isolateProjectRoot(t)
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	go func() {
		time.Sleep(200 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	_, err := Run(ctx, dir, "sleep 30", time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("Run took %v, want a prompt cancel", elapsed)
	}
}

// TestRunEscapedWriterDoesNotStall: a writer that escaped the process group
// (setsid) holds the pipe, but the bounded post-kill drain still returns
// (and closes the read end so nothing leaks across rounds).
func TestRunEscapedWriterDoesNotStall(t *testing.T) {
	isolateProjectRoot(t)
	if _, err := exec.LookPath("setsid"); err != nil {
		t.Skip("setsid not available")
	}
	dir := t.TempDir()
	start := time.Now()
	res, err := Run(t.Context(), dir, "setsid sleep 30 & echo $! > escapee.pid", 300*time.Millisecond)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if pidData, readErr := os.ReadFile(filepath.Join(dir, "escapee.pid")); readErr == nil {
		if pid := strings.TrimSpace(string(pidData)); pid != "" {
			t.Cleanup(func() { _ = exec.Command("kill", "-9", pid).Run() })
		}
	}
	if !res.TimedOut {
		t.Error("TimedOut = false, want true")
	}
	if elapsed := time.Since(start); elapsed > 10*time.Second {
		t.Errorf("Run took %v, want a bounded return despite the escaped writer", elapsed)
	}
}
