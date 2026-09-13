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

	"github.com/nanjj/clog"
)

// maxOutputBytes caps the merged output captured from one script (the
// design's ~256KB); the file still records that a truncation happened.
const maxOutputBytes = 256 * 1024

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

// Run executes one extracted block: the script is written verbatim to the
// next scriptN.sh under dir, run with bash (working directory dir) with its
// own process group, and the merged stdout+stderr is written to scriptN.txt
// with the backfill wrapper. A timeout kills the whole process group, so
// grandchildren (compilers, make's children) cannot outlive the round.
//
// dir is the project root; "" means the current directory.
func Run(ctx context.Context, dir, script string, timeout time.Duration) (*RunResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "shellblock.Run")
	defer span.Finish()
	if strings.TrimSpace(script) == "" {
		return nil, fmt.Errorf("shellblock: empty script")
	}
	if dir == "" {
		dir = "."
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
	if err := os.WriteFile(scriptPath, []byte(scriptText), 0o644); err != nil {
		return nil, fmt.Errorf("shellblock: write script: %w", err)
	}
	exec, err := runScript(ctx, dir, scriptPath, timeout)
	if err != nil {
		return nil, err
	}
	res := &RunResult{
		Number:     n,
		ScriptPath: scriptPath,
		OutputPath: filepath.Join(dir, fmt.Sprintf("script%d.txt", n)),
		Output:     exec.output,
		ExitCode:   exec.exitCode,
		TimedOut:   exec.timedOut,
		Truncated:  exec.truncated,
		Timeout:    exec.timeout,
		Duration:   exec.duration,
	}
	if err := os.WriteFile(res.OutputPath, []byte(FormatBackfill(n, scriptText, res)), 0o644); err != nil {
		return nil, fmt.Errorf("shellblock: write output backfill: %w", err)
	}
	return res, nil
}

// scriptResult is the raw outcome of one bash invocation.
type scriptResult struct {
	output    string
	exitCode  int
	timedOut  bool
	truncated bool
	timeout   time.Duration
	duration  time.Duration
}

// runScript runs scriptPath with bash, merging stdout and stderr through a
// single pipe (so the interleaving is the kernel's, like `bash s.sh 2>&1`).
func runScript(ctx context.Context, dir, scriptPath string, timeout time.Duration) (*scriptResult, error) {
	timeout = clampTimeout(timeout)
	start := time.Now()
	cmd := exec.Command("bash", scriptPath)
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
		return nil, fmt.Errorf("shellblock: start bash (is bash installed?): %w", err)
	}
	// The child holds the only writer; the reader sees EOF when it exits.
	_ = pw.Close()
	outCh := make(chan readResult, 1)
	go readMerged(pr, outCh)
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()

	var waitErr error
	var timedOut bool
	select {
	case waitErr = <-waitCh:
	case <-time.After(timeout):
		timedOut = true
		killProcessGroup(cmd)
		waitErr = <-waitCh
	case <-ctx.Done():
		killProcessGroup(cmd)
		<-waitCh
		<-outCh
		return nil, ctx.Err()
	}
	read := <-outCh
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
// (1 when none exist).
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
// real command line, the script verbatim in a fenced block, the merged
// output, and the exit status. The shape follows the manual-verification
// backfill (the dscli-shell harness), with the real command line and an
// explicit exit code added.
func FormatBackfill(n int, scriptText string, res *RunResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "bash script%d.sh:\n```xml\n", n)
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
