package shellblock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	dsctx "github.com/dscli/dscli/internal/context"

	"github.com/nanjj/clog"
)

// maxOutputBytes caps the merged output captured from one script (the
// design's ~256KB); the file still records that a truncation happened.
const maxOutputBytes = 256 * 1024

// drainGrace bounds the post-kill drain: after the process group is killed
// the pipe closes at once, but a writer that escaped the group (setsid and
// friends) could hold it - the round must still return.
const drainGrace = 2 * time.Second

// scriptNameRE matches the generated script files; the next number is the
// highest existing one plus one (the manual experiment left script1..64).
var scriptNameRE = regexp.MustCompile(`^script([0-9]+)[.]sh$`)

// RunResult describes one executed block and the files it produced.
type RunResult struct {
	// Number is the N of scriptN.sh / scriptN.txt.
	Number int
	// ScriptPath and OutputPath are the files written under the project
	// root.
	ScriptPath string
	OutputPath string

	// Attachments lists files an `#!attach` block asked for: existing
	// regular files in the order listed, resolved against dir (absolute
	// paths stay as they are) and uploaded by the shell loop alongside
	// OutputPath. Nil for bash and python rounds.
	Attachments []string
	// Output is the merged stdout+stderr, truncated at maxOutputBytes.
	Output string
	// ExitCode is the process exit code, -1 when killed.
	ExitCode int
	// TimedOut marks a round killed by the timeout.
	TimedOut bool
	// Truncated marks output cut at maxOutputBytes.
	Truncated bool
	// Timeout is the effective timeout used for the run.
	Timeout time.Duration
	// Duration is the wall-clock run time.
	Duration time.Duration
}

// scriptMode selects how Run executes one block; the first line of the
// script body carries the marker.
type scriptMode int

const (
	// modeBash runs the script with bash: the protocol's original form and
	// the fallback for every script without a recognized marker.
	modeBash scriptMode = iota
	// modePython runs the script with the python interpreter (a `#!` line
	// ending in python / python3).
	modePython
	// modeAttach attaches files instead of executing: every line after the
	// marker names one (attachResult).
	modeAttach
)

// String implements fmt.Stringer; the value labels the backfill header.
func (m scriptMode) String() string {
	switch m {
	case modePython:
		return "python"
	case modeAttach:
		return "attach"
	default:
		return "bash"
	}
}

// detectScriptMode reads the mode marker from the first line of the script.
// Leading and trailing whitespace is tolerated - the model is asked to emit
// the marker exactly, but a stray space must not silently turn a python
// script into a bash one (bash would garble it). The marker must be a `#!`
// line, so a plain first line that merely ends in "python" stays bash.
func detectScriptMode(script string) scriptMode {
	first := script
	if i := strings.IndexByte(first, '\n'); i >= 0 {
		first = first[:i]
	}
	first = strings.TrimSpace(first)
	switch {
	case first == "#!attach":
		return modeAttach
	case strings.HasPrefix(first, "#!") &&
		(strings.HasSuffix(first, "python") || strings.HasSuffix(first, "python3")):
		return modePython
	default:
		return modeBash
	}
}

// defaultDir is where a block with no explicit dir lands: the project's
// scratch directory under .dscli/tmp (created on demand). The old default
// was ".", which made the landing spot depend on wherever dscli happened to
// run.
func defaultDir() string {
	return filepath.Join(dsctx.ProjectRoot, ".dscli", "tmp")
}

// pythonInterpreter locates the python binary for modePython: python3 first
// (the name current systems ship), python as the fallback. The interpreter
// is resolved before the script file is written, so a missing python fails
// the round without leaving artifacts behind.
func pythonInterpreter() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("shellblock: python interpreter not found (tried python3 and python)")
}

// Run executes one extracted block: the script is written verbatim to the
// next scriptN.sh under dir (mode 0600).
//
// The first script line selects the mode (detectScriptMode): bash (the
// default, also `#!/bin/bash`), python (a `#!` line ending in python /
// python3), or an `#!attach` file list. bash and python scripts run with
// their interpreter (working directory dir) in their own process group, and
// the merged stdout+stderr is written to scriptN.txt (mode 0600) with the
// backfill wrapper; a timeout kills the whole process group, so
// grandchildren (compilers, make's children) cannot outlive the round. An
// `#!attach` block runs nothing: every listed path that exists as a regular
// file is resolved against dir and appended to RunResult.Attachments, and
// one status line per path lands in the output.
//
// Run refuses a script that matches the shared destructive-command
// interception (Blocked) - a fail-closed backstop behind the loop's
// model-facing refusal.
//
// dir is the working directory (the project root in the WebChat channel);
// "" means PROJECT_ROOT/.dscli/tmp, created on demand.
func Run(ctx context.Context, dir, script string, timeout time.Duration) (*RunResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "shellblock.Run")
	defer span.Finish()
	if strings.TrimSpace(script) == "" {
		return nil, fmt.Errorf("shellblock: empty script")
	}
	// Fail-closed backstop: the loop refuses a blocked script with a
	// model-facing warning before calling Run, but the runner itself must
	// never execute a destructive match either (see Blocked).
	if detail, blocked := Blocked(script); blocked {
		return nil, fmt.Errorf("shellblock: script blocked (destructive command %q)", detail)
	}
	if dir == "" {
		dir = defaultDir()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, fmt.Errorf("shellblock: create default dir: %w", err)
		}
	}
	mode := detectScriptMode(script)
	interp := "bash"
	if mode == modePython {
		var err error
		interp, err = pythonInterpreter()
		if err != nil {
			return nil, err
		}
	}
	n, err := nextScriptNumber(dir)
	if err != nil {
		return nil, err
	}
	scriptText := script
	if !strings.HasSuffix(scriptText, "\n") {
		scriptText += "\n"
	}
	scriptPath := filepath.Join(dir, fmt.Sprintf("script%d.sh", n))
	if err := os.WriteFile(scriptPath, []byte(scriptText), 0o600); err != nil {
		return nil, fmt.Errorf("shellblock: write script: %w", err)
	}
	var execRes *scriptResult
	if mode == modeAttach {
		execRes = attachResult(dir, scriptText)
	} else {
		execRes, err = runScript(ctx, dir, scriptPath, interp, timeout)
		if err != nil {
			return nil, err
		}
	}
	res := &RunResult{
		Number:      n,
		ScriptPath:  scriptPath,
		OutputPath:  filepath.Join(dir, fmt.Sprintf("script%d.txt", n)),
		Attachments: execRes.attachments,
		Output:      execRes.output,
		ExitCode:    execRes.exitCode,
		TimedOut:    execRes.timedOut,
		Truncated:   execRes.truncated,
		Timeout:     execRes.timeout,
		Duration:    execRes.duration,
	}
	if err := os.WriteFile(res.OutputPath, []byte(FormatBackfill(n, scriptText, res)), 0o600); err != nil {
		return nil, fmt.Errorf("shellblock: write output backfill: %w", err)
	}
	return res, nil
}

// scriptResult is the raw outcome of one block execution: a bash or python
// invocation, or an `#!attach` scan.
type scriptResult struct {
	output      string
	attachments []string
	exitCode    int
	timedOut    bool
	truncated   bool
	timeout     time.Duration
	duration    time.Duration
}

// runScript runs scriptPath with the given interpreter, merging stdout and
// stderr through a single pipe (so the interleaving is the kernel's, like
// `bash s.sh 2>&1`). The timeout bounds the whole round: it guards the pipe
// drain, not just the process wait - a background child can hold the pipe
// long past the interpreter's own exit - and kills the process group; the
// post-kill drain is bounded by drainGrace so even a writer that escaped
// the group cannot stall the return.
func runScript(ctx context.Context, dir, scriptPath, interp string, timeout time.Duration) (*scriptResult, error) {
	timeout = clampTimeout(timeout)
	start := time.Now()
	cmd := exec.Command(interp, scriptPath)
	cmd.Dir = dir
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("shellblock: pipe: %w", err)
	}
	cmd.Stdout = pw
	cmd.Stderr = pw
	prepareProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		_ = pw.Close()
		_ = pr.Close()
		return nil, fmt.Errorf("shellblock: start %s (is it installed?): %w", interp, err)
	}
	// The child holds the only writer; the reader sees EOF when it exits.
	_ = pw.Close()
	outCh := make(chan readResult, 1)
	go readMerged(pr, outCh)
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	var (
		read     readResult
		waitErr  error
		gotRead  bool
		timedOut bool
	)
	deadline := time.Now().Add(timeout)
	// Phase 1: wait for the pipe to close. It closes only after EVERY
	// writer is gone - bash AND any background child that inherited it.
	// Bash exiting alone is not enough: a child (`sleep 300 &`) can hold
	// the pipe long past its parent's exit, so the timeout must guard the
	// drain, not just the process wait.
	select {
	case read = <-outCh:
		gotRead = true
	case <-time.After(time.Until(deadline)):
		timedOut = true
		killProcessGroup(cmd)
	case <-ctx.Done():
		killProcessGroup(cmd)
		select {
		case <-outCh:
		case <-time.After(drainGrace):
			// A writer escaped the process group and still holds the pipe;
			// release the read end so nothing accumulates across rounds.
			_ = pr.Close()
		}
		select {
		case <-waitCh:
		case <-time.After(drainGrace):
		}
		return nil, ctx.Err()
	}
	// Phase 2: reap bash. Normally instantaneous once the pipe closed; a
	// script that closed its stdio early but kept running stays bounded by
	// the same deadline.
	if !timedOut {
		select {
		case waitErr = <-waitCh:
		case <-time.After(time.Until(deadline)):
			timedOut = true
			killProcessGroup(cmd)
		}
	}
	// Phase 3: after a kill, collect what the drain produced without
	// blocking on a writer that escaped the process group; the reader
	// goroutine ends by itself when the pipe finally closes and its send
	// lands in the buffered channel. If the grace expires, close the read
	// end so the still-blocked reader (and its FD) cannot accumulate across
	// rounds; an in-flight read returns ErrClosed and the goroutine exits.
	if timedOut {
		select {
		case waitErr = <-waitCh:
		case <-time.After(drainGrace):
		}
		if !gotRead {
			select {
			case read = <-outCh:
			case <-time.After(drainGrace):
				_ = pr.Close()
			}
		}
	}
	res := &scriptResult{
		output:    read.data,
		exitCode:  exitCodeFrom(waitErr),
		timedOut:  timedOut,
		truncated: read.truncated,
		timeout:   timeout,
		duration:  time.Since(start),
	}
	if timedOut {
		res.exitCode = -1
	}
	return res, nil
}

// attachResult builds the result of an `#!attach` block without running
// anything. Every non-empty line after the marker names a file; the path is
// resolved against dir (absolute paths stay as they are) and gets exactly
// one output line:
//
//	<path> attached           - existing regular file, appended to Attachments
//	<path> not exist          - nothing at the path
//	<path> not a regular file - a directory or special file, skipped
//
// The last case is a guard, not protocol: a directory in the upload batch
// would fail the whole attachment send, so one bad path must not poison it.
// The output echoes the path as listed (not the resolved one) so the model
// can correct its own line; a list with no usable lines produces an empty
// output.
func attachResult(dir, scriptText string) *scriptResult {
	start := time.Now()
	body := scriptText
	if i := strings.IndexByte(body, '\n'); i >= 0 {
		body = body[i+1:]
	} else {
		body = ""
	}
	res := &scriptResult{}
	var b strings.Builder
	for _, line := range strings.Split(body, "\n") {
		path := strings.TrimSpace(line)
		if path == "" {
			continue
		}
		target := path
		if !filepath.IsAbs(target) {
			target = filepath.Join(dir, target)
		}
		info, err := os.Stat(target)
		switch {
		case err != nil:
			fmt.Fprintf(&b, "%s not exist\n", path)
		case !info.Mode().IsRegular():
			fmt.Fprintf(&b, "%s not a regular file\n", path)
		default:
			res.attachments = append(res.attachments, target)
			fmt.Fprintf(&b, "%s attached\n", path)
		}
	}
	res.output = b.String()
	res.duration = time.Since(start)
	return res
}

// readResult is the bounded merged output drained from the pipe.
type readResult struct {
	data      string
	truncated bool
}

// readMerged drains the merged-output pipe into a bounded buffer. It never
// stops reading - a full pipe would block the script - so bytes past the cap
// are discarded and the output is marked truncated instead.
func readMerged(pr *os.File, ch chan<- readResult) {
	var buf bytes.Buffer
	copied, _ := io.Copy(&buf, io.LimitReader(pr, maxOutputBytes))
	rest, _ := io.Copy(io.Discard, pr)
	truncated := copied == maxOutputBytes && rest > 0
	data := buf.String()
	if truncated {
		data = trimPartialRune(data)
	}
	_ = pr.Close()
	ch <- readResult{data: data, truncated: truncated}
}

// trimPartialRune drops an incomplete trailing UTF-8 sequence left by the
// byte-boundary cut (at most UTFMax bytes).
func trimPartialRune(s string) string {
	for i := 0; i < utf8.UTFMax && len(s) > 0; i++ {
		r, size := utf8.DecodeLastRuneInString(s)
		if r != utf8.RuneError || size > 1 {
			break
		}
		s = s[:len(s)-1]
	}
	return s
}

// exitCodeFrom extracts the process exit code; killed processes yield -1.
func exitCodeFrom(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// clampTimeout applies the protocol bounds: default when unset, cap at
// MaxTimeout.
func clampTimeout(d time.Duration) time.Duration {
	if d <= 0 {
		return DefaultTimeout
	}
	if d > MaxTimeout {
		return MaxTimeout
	}
	return d
}

// nextScriptNumber returns the highest existing scriptN.sh number plus one
// (1 when none exist). The shell loop runs one block per round, so a single
// writer is the working assumption; concurrent Run calls in one directory
// could pick the same N.
func nextScriptNumber(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("shellblock: scan %s: %w", dir, err)
	}
	maxN := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		m := scriptNameRE.FindStringSubmatch(entry.Name())
		if m == nil {
			continue
		}
		if n, err := strconv.Atoi(m[1]); err == nil && n > maxN {
			maxN = n
		}
	}
	return maxN + 1, nil
}

// FormatBackfill renders the scriptN.txt wrapper sent back to the model: the
// mode-prefixed command line, the script verbatim in a fenced block, the
// merged output, and the exit status. The shape follows the manual-
// verification backfill (the dscli-shell harness), with the mode and an
// explicit exit code added.
func FormatBackfill(n int, scriptText string, res *RunResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s script%d.sh:\n```xml\n", detectScriptMode(scriptText), n)
	b.WriteString(scriptText)
	b.WriteString("```\noutput:\n")
	out := res.Output
	switch {
	case strings.TrimSpace(out) == "":
		out = "(no output)\n"
	case !strings.HasSuffix(out, "\n"):
		out += "\n"
	}
	b.WriteString(out)
	if res.Truncated {
		fmt.Fprintf(&b, "[output truncated at %dKB]\n", maxOutputBytes/1024)
	}
	if res.TimedOut {
		fmt.Fprintf(&b, "exit code: killed (timeout %s)\n", res.Timeout)
	} else {
		fmt.Fprintf(&b, "exit code: %d\n", res.ExitCode)
	}
	return b.String()
}
