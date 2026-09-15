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

// MaxAttachFiles caps one `#!attach` block's file list: the WebChat upload
// batch holds 50 files (WebUploadMaxFiles in internal/lp) and the round's
// scriptN.txt rides in the same batch, so 49 fit. The cap turns a would-be
// fatal batch rejection into a per-path status line. Exported so the lp
// layer can pin the batch invariant with a cross-package test.
const MaxAttachFiles = 49

// drainGrace bounds the post-kill drain: after the process group is killed
// the pipe closes at once, but a writer that escaped the group (setsid and
// friends) could hold it - the round must still return.
const drainGrace = 2 * time.Second

// scriptNameRE matches the generated artifacts: the scripts (scriptN.sh /
// scriptN.py / scriptN.lst, by mode) and their scriptN.txt backfill. The
// next number is the highest existing one plus one (the manual experiment
// left script1..64 under the old project-root landing spot).
var scriptNameRE = regexp.MustCompile(`^script([0-9]+)[.](sh|py|lst|txt)$`)

// RunResult describes one executed block and the files it produced.
type RunResult struct {
	// Number is the N of scriptN.* (the script, by mode) / scriptN.txt.
	Number int
	// ScriptPath and OutputPath are the files written under the project's
	// spool directory (PROJECT_ROOT/.dscli/tmp).
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
	// Timeout is the effective timeout used for the run; zero for `#!attach`
	// rounds (nothing runs).
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
	// naming python; see detectScriptMode).
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

// pythonNameRE matches a python interpreter name with an optional version
// suffix: python, python3, python3.11, python2.7. A custom name that merely
// contains "python" (micropython, custompython) does not match - only the
// name itself may carry the version.
var pythonNameRE = regexp.MustCompile(`^python([0-9]+(\.[0-9]+)*)?$`)

// firstLine returns the first line of s, or s itself when it has none.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// attachMarker reports whether line is the `#!attach` marker and returns
// the optional first path it carries (`#!attach f.png` -> "f.png"). The
// path is accepted on the marker line so a model slip cannot fall back to
// bash and execute the rest of the list as commands. detectScriptMode and
// attachResult share this parse so the mode decision and the body
// extraction cannot drift.
func attachMarker(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), "#!attach")
	if !ok || (rest != "" && rest[0] != ' ' && rest[0] != '\t') {
		return "", false
	}
	return strings.TrimSpace(rest), true
}

// detectScriptMode reads the mode marker from the first line of the script.
// Leading and trailing whitespace is tolerated - the model is asked to emit
// the marker exactly, but a stray space must not silently turn a python
// script into a bash one (bash would garble it). The marker must be a `#!`
// line, so a plain first line that merely ends in "python" stays bash.
func detectScriptMode(script string) scriptMode {
	first := firstLine(script)
	if _, ok := attachMarker(first); ok {
		return modeAttach
	}
	if pythonNameRE.MatchString(shebangInterpreter(first)) {
		return modePython
	}
	return modeBash
}

// Mode returns the script's execution mode label: "bash", "python", or
// "attach" (see detectScriptMode). Callers that show a round's tool to a
// human use it to pick the mode's icon.
func Mode(script string) string {
	return detectScriptMode(script).String()
}

// shebangInterpreter returns the base name of the interpreter named on a
// `#!` line: the first token, or the second when the first is `env`
// (`/usr/bin/env python3` -> python3, `/usr/bin/python3.11` -> python3.11).
// It returns "" for a line that is not a shebang or names nothing.
func shebangInterpreter(line string) string {
	line = strings.TrimSpace(line)
	rest, ok := strings.CutPrefix(line, "#!")
	if !ok {
		return ""
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	name := filepath.Base(fields[0])
	if name == "env" && len(fields) > 1 {
		return filepath.Base(fields[1])
	}
	return name
}

// spoolDir is where every round's artifacts land: PROJECT_ROOT/.dscli/tmp,
// created on demand. It is deliberately separate from the script's working
// directory (Run's dir): the script file and the backfill are dscli's audit
// trail, not project content, so they must not litter the project root (or
// wherever dscli happened to run).
func spoolDir() string {
	return filepath.Join(dsctx.ProjectRoot, ".dscli", "tmp")
}

// scriptExt returns the extension that names the script file's form, so the
// file on disk reads like what it is: `.sh` for bash, `.py` for python, and
// `.lst` for an `#!attach` path list.
func scriptExt(mode scriptMode) string {
	switch mode {
	case modePython:
		return ".py"
	case modeAttach:
		return ".lst"
	default:
		return ".sh"
	}
}

// scriptFileName returns the round's script file name: scriptN.sh (bash),
// scriptN.py (python), or scriptN.lst (attach). Run names the file with it
// and FormatBackfill heads the backfill with it, so the label and the file
// on disk cannot drift.
func scriptFileName(n int, mode scriptMode) string {
	return fmt.Sprintf("script%d%s", n, scriptExt(mode))
}

// ErrNoPythonInterpreter reports a python-mode block on a host without a
// python binary. The shell loop turns it into a model-facing warning round
// (fall back to bash); every other Run error stays fatal.
var ErrNoPythonInterpreter = errors.New("shellblock: python interpreter not found")

// pythonInterpreter locates the python binary for a python-mode script: the
// shebang token is honored first (python3.11 runs python3.11 when present),
// then the generic chain python3 -> python. A python2 name is never
// substituted with python3 - when no python2 binary exists the sentinel
// sends the model back to bash instead of running the script under the
// wrong interpreter. The interpreter is resolved before the script file is
// written, so a missing python fails the round without leaving artifacts
// behind.
func pythonInterpreter(script string) (string, error) {
	candidates := pythonCandidates(shebangInterpreter(firstLine(script)))
	for _, name := range candidates {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%w (tried %s)", ErrNoPythonInterpreter, strings.Join(candidates, ", "))
}

// pythonCandidates lists the interpreter names to try for a shebang token,
// in order. The python2 family is isolated on purpose: py2 code must not be
// silently handed to a python3 interpreter.
func pythonCandidates(token string) []string {
	switch {
	case token == "python2":
		return []string{"python2"}
	case strings.HasPrefix(token, "python2."):
		return []string{token, "python2"}
	case token == "python" || token == "python3":
		return []string{"python3", "python"}
	default:
		return []string{token, "python3", "python"}
	}
}

// Run executes one extracted block: the script is written verbatim to the
// next scriptN file under the project's spool directory, PROJECT_ROOT/
// .dscli/tmp (created on demand, mode 0600), with the extension its mode
// selects (scriptN.sh for bash, scriptN.py for python, scriptN.lst for an
// `#!attach` path list).
//
// The first script line selects the mode (detectScriptMode): bash (the
// default, also `#!/bin/bash`), python (a `#!` line naming python - the
// interpreter token, with an optional version suffix), or an `#!attach`
// file list. bash and python scripts run with
// their interpreter (working directory dir) in their own process group, and
// the merged stdout+stderr is written to scriptN.txt (mode 0600, same spool
// directory) with the backfill wrapper; a timeout kills the whole process
// group, so grandchildren (compilers, make's children) cannot outlive the
// round. An `#!attach` block runs nothing: every listed path that exists as
// a regular file is resolved against dir and appended to
// RunResult.Attachments, and one status line per path lands in the output.
//
// Run refuses a script that matches the shared destructive-command
// interception (Blocked) - a fail-closed backstop behind the loop's
// model-facing refusal.
//
// dir is the script's working directory (the project root in the WebChat
// channel); "" means PROJECT_ROOT. The artifacts never land there - they
// always go to the spool directory.
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
	// dir is the script's working directory, never the artifact landing
	// spot: the files go to the spool directory below.
	if dir == "" {
		dir = dsctx.ProjectRoot
	}
	mode := detectScriptMode(script)
	interp := "bash"
	if mode == modePython {
		var err error
		interp, err = pythonInterpreter(script)
		if err != nil {
			return nil, err
		}
	}
	spool := spoolDir()
	if err := os.MkdirAll(spool, 0o700); err != nil {
		return nil, fmt.Errorf("shellblock: create spool dir %s: %w", spool, err)
	}
	n, err := nextScriptNumber(spool)
	if err != nil {
		return nil, err
	}
	scriptText := script
	if !strings.HasSuffix(scriptText, "\n") {
		scriptText += "\n"
	}
	scriptPath := filepath.Join(spool, scriptFileName(n, mode))
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
		OutputPath:  filepath.Join(spool, fmt.Sprintf("script%d.txt", n)),
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
// anything. The marker line may carry the first path; every following
// non-empty line names another. The path is resolved against dir (absolute
// paths stay as they are) and gets exactly one output line:
//
//	<path> attached           - existing regular file, appended to Attachments
//	<path> not exist          - nothing at the path
//	<path> not a regular file - a directory or special file, skipped
//	<path> not attached (attachment limit N reached) - past MaxAttachFiles
//
// The guards are not protocol: a directory in the upload batch would fail
// the whole attachment send and so would a batch over the site's limit, so
// one bad path (or an over-long list) must not poison the round. The status
// output itself is bounded by maxOutputBytes like merged script output.
// os.Stat follows symlinks, so a symlink to a regular file is attached at
// its target. The output echoes the path as listed (not the resolved one)
// so the model can correct its own line; a list with no usable lines
// produces an empty output.
func attachResult(dir, scriptText string) *scriptResult {
	start := time.Now()
	first, body, _ := strings.Cut(scriptText, "\n")
	if path, ok := attachMarker(first); ok {
		// A path on the marker line itself counts as the first entry (see
		// attachMarker); the mode decision and the body extraction share
		// the parse, so they cannot drift.
		body = path + "\n" + body
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
		case len(res.attachments) >= MaxAttachFiles:
			fmt.Fprintf(&b, "%s not attached (attachment limit %d reached)\n", path, MaxAttachFiles)
		default:
			res.attachments = append(res.attachments, target)
			fmt.Fprintf(&b, "%s attached\n", path)
		}
	}
	res.output = b.String()
	if len(res.output) > maxOutputBytes {
		// The status list is output like any script's: bound it so a
		// pathological list cannot balloon scriptN.txt (and the upload
		// batch) past the merged-output cap; FormatBackfill adds the note.
		res.output = trimPartialRune(res.output[:maxOutputBytes])
		res.truncated = true
	}
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

// nextScriptNumber returns the highest existing scriptN artifact number
// plus one (1 when none exist); the scripts (.sh / .py / .lst) and the
// scriptN.txt backfills all count, so a number is never reused. The shell
// loop runs one block per round, so a single writer is the working
// assumption; concurrent Run calls in one spool directory could pick the
// same N.
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
// mode-prefixed command line (`bash scriptN.sh:` / `python scriptN.py:` /
// `attach scriptN.lst:`), the script verbatim in a fenced block, the merged
// output, and the exit status. The shape follows the manual-
// verification backfill (the dscli-shell harness), with the mode and an
// explicit exit code added; the file name carries the mode's extension so
// it matches the file Run wrote.
func FormatBackfill(n int, scriptText string, res *RunResult) string {
	var b strings.Builder
	mode := detectScriptMode(scriptText)
	fmt.Fprintf(&b, "%s %s:\n```xml\n", mode, scriptFileName(n, mode))
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
