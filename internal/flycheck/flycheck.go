// Package flycheck provides on-the-fly syntax checking for code files,
// inspired by Emacs flycheck. It detects language from file extension,
// finds the appropriate checker, runs it, and returns results.
//
// Starting with Go + staticcheck, the architecture supports adding
// more languages and checkers incrementally.
package flycheck

import (
	"errors"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	dsctx "gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/parse"
	"gitcode.com/dscli/dscli/internal/shell"
)

const (
	// DefaultTimeout is the default time limit for a checker run.
	DefaultTimeout = 15 * time.Second
)

// Checker defines a syntax checker for a specific language.
// Each language can have multiple checkers (e.g. go: staticcheck, go vet, errcheck).
type Checker struct {
	Name        string // e.g. "go-staticcheck"
	Command     string // e.g. "staticcheck"
	InstallHint string // Human-readable install instruction
	// BuildArgs builds command-line arguments for checking the given file.
	// filename is project-relative, project root is taken from context.ProjectRoot.
	BuildArgs func(filename string) []string
}

// goStaticcheck runs staticcheck on a Go package.
var goStaticcheck = Checker{
	Name:    "go-staticcheck",
	Command: "staticcheck",
	InstallHint: `请安装 staticcheck:
  go install honnef.co/go/tools/cmd/staticcheck@latest`,
	BuildArgs: func(filename string) []string {
		// Build absolute package path using context.ProjectRoot,
		// since shell.SimpleExecute may run in a different working directory.
		pkgDir := filepath.Dir(filename)
		if pkgDir == "." {
			return []string{"-tests", dsctx.ProjectRoot}
		}
		return []string{"-tests", filepath.Join(dsctx.ProjectRoot, pkgDir)}
	},
}

// Registry maps language identifiers (from parse.GuessLanguage) to their checkers.
// Add entries here when supporting new languages.
var Registry = map[string][]Checker{
	"go": {goStaticcheck},
}

// Flycheck runs syntax checkers on a file and returns any issues found.
//
// Uses context.ProjectRoot as the working directory for running checkers.
//
// Returns:
//   - result: checker output containing issues (empty if no issues)
//   - suggestion: install hints or fix suggestions (only when err != nil)
//   - err: system error (checker not found, timeout, etc.), nil if check succeeded
//
// If the language is not supported, returns ("", "", nil) — silently skipped.
func Flycheck(ctx context.Context, filename string) (result, suggestion string, err error) {
	lang := parse.GuessLanguage(filename)
	checkers, ok := Registry[lang]
	if !ok || len(checkers) == 0 {
		return "", "", nil
	}

	var results []string
	for _, checker := range checkers {
		checkerCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
		output, runErr := runChecker(checkerCtx, checker, filename)
		cancel()

		if runErr != nil {
			// Checker not installed → return install hint
			if isNotFoundError(runErr) {
				return "", checker.InstallHint, fmt.Errorf("%s 未安装", checker.Command)
			}
			// Timeout → return timeout suggestion
			if checkerCtx.Err() == context.DeadlineExceeded {
				return "", fmt.Sprintf("%s 检查超时 (%v)，建议简化代码或稍后再试",
					checker.Name, DefaultTimeout), runErr
			}
			// Other errors → skip this checker
			continue
		}

		output = strings.TrimSpace(output)
		if output != "" {
			results = append(results, formatCheckerOutput(checker.Name, output))
		}
	}

	if len(results) > 0 {
		result = strings.Join(results, "\n")
	}

	return
}

// runChecker executes a single checker and returns its stdout (issues).
// Uses shell.SimpleExecuteSeparate to bypass the allow-list check for lint tools
// and properly distinguish stdout (issues) from stderr (warnings/errors).
func runChecker(ctx context.Context, checker Checker, filename string) (string, error) {
	// Pre-check: is the command installed?
	if _, err := exec.LookPath(checker.Command); err != nil {
		return "", &exec.Error{Name: checker.Command, Err: exec.ErrNotFound}
	}

	args := checker.BuildArgs(filename)
	var cmdStr strings.Builder
	cmdStr.WriteString(checker.Command)
	for _, arg := range args {
		cmdStr.WriteString(" " + arg)
	}

	stdout, stderr, runErr := shell.SimpleExecuteSeparate(ctx, cmdStr.String())

	if runErr != nil {
		// staticcheck exits with 1 when issues are found, writing them to stdout.
		// If stdout is non-empty, treat as successful issue detection.
		stdout = strings.TrimSpace(stdout)
		if stdout != "" {
			return stdout, nil
		}
		// No stdout: real error. Return stderr if available, otherwise the error.
		if stderr != "" {
			return "", fmt.Errorf("%s: %s", checker.Name, strings.TrimSpace(stderr))
		}
		return "", runErr
	}

	return strings.TrimSpace(stdout), nil
}

// isNotFoundError returns true if the error indicates the command was not found
// in PATH (OS returned ENOENT / "executable file not found").
func isNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	// Use errors.As to handle wrapped errors.
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return execErr.Err == exec.ErrNotFound
	}
	return false
}

// ---------------------------------------------------------------------------
// Issue severity classification
// ---------------------------------------------------------------------------

// IssueSeverity classifies the severity of a checker issue.
type IssueSeverity int

const (
	SevError   IssueSeverity = iota // ❌ compile/syntax error
	SevWarning                      // ⚠️ lint warning
	SevSuggestion                   // 💡 improvement suggestion
)

// String returns the emoji prefix for the severity.
func (s IssueSeverity) String() string {
	switch s {
	case SevError:
		return "❌"
	case SevWarning:
		return "⚠️"
	case SevSuggestion:
		return "💡"
	default:
		return "⁉️"
	}
}

// classifyIssueLine determines the severity of a single issue line.
// staticcheck output format: file:line:col: message
// Compile errors end with "(compile)". Warnings have diagnostic codes
// like U1000, SAxxxx. Suggestions have QFxxxx codes.
//
// Classification rules (priority order):
//  1. "(compile)" suffix → ❌ compile error
//  2. "syntax error" keyword → ❌ compile error
//  3. "QF" in code → 💡 suggestion
//  4. Everything else → ⚠️ warning
func classifyIssueLine(line string) IssueSeverity {
	// Compile errors: staticcheck appends "(compile)" to type-checker errors
	if strings.Contains(line, "(compile)") {
		return SevError
	}
	// Syntax errors reported by the parser (may appear without "(compile)" suffix)
	if strings.Contains(line, "syntax error") {
		return SevError
	}
	// Quickfix suggestions (QFxxxx codes from staticcheck)
	if strings.Contains(line, "QF") {
		return SevSuggestion
	}
	// Default: warning
	return SevWarning
}

// ClassifiedIssue holds a single checker issue with its severity.
type ClassifiedIssue struct {
	Severity IssueSeverity
	Line     string
}

// ClassifyIssues splits raw checker output into classified issues.
func ClassifyIssues(raw string) []ClassifiedIssue {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	issues := make([]ClassifiedIssue, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		issues = append(issues, ClassifiedIssue{
			Severity: classifyIssueLine(line),
			Line:     line,
		})
	}
	return issues
}

// IssueStats summarizes classified issues.
type IssueStats struct {
	Errors      int
	Warnings    int
	Suggestions int
}

// CountStats computes stats from classified issues.
func CountStats(issues []ClassifiedIssue) IssueStats {
	var s IssueStats
	for _, iss := range issues {
		switch iss.Severity {
		case SevError:
			s.Errors++
		case SevWarning:
			s.Warnings++
		case SevSuggestion:
			s.Suggestions++
		}
	}
	return s
}

// formatCheckerOutput formats checker output with severity-aware emoji prefixes,
// stats summary, and code block wrapping for LLM readability.
func formatCheckerOutput(checkerName, output string) string {
	issues := ClassifyIssues(output)
	if len(issues) == 0 {
		return ""
	}

	stats := CountStats(issues)

	var b strings.Builder

	// Header with stats and emoji
	b.WriteString("🔍 **")
	b.WriteString(checkerName)
	b.WriteString(" 检查结果：** ")

	// Build stat parts
	var parts []string
	if stats.Errors > 0 {
		parts = append(parts, fmt.Sprintf("❌ %d 个编译错误", stats.Errors))
	}
	if stats.Warnings > 0 {
		parts = append(parts, fmt.Sprintf("⚠️ %d 个警告", stats.Warnings))
	}
	if stats.Suggestions > 0 {
		parts = append(parts, fmt.Sprintf("💡 %d 个建议", stats.Suggestions))
	}

	// Checker health badge — prominent when there are compile errors
	if stats.Errors > 0 {
		b.WriteString("**🔥 有编译错误，必须立即修复！** ")
	}

	if len(parts) > 0 {
		b.WriteString(strings.Join(parts, " / "))
	}

	b.WriteString("\n```\n")

	// Output each issue with severity prefix
	for _, iss := range issues {
		b.WriteString(iss.Severity.String())
		b.WriteString(" ")
		b.WriteString(iss.Line)
		b.WriteString("\n")
	}
	b.WriteString("```")

	return b.String()
}

// ---------------------------------------------------------------------------
// Directory-level checking (used by toolcall handler)
// ---------------------------------------------------------------------------

// FlycheckDir runs go checkers on a project-relative directory.
// Like Flycheck but operates on a directory rather than deriving from a filename.
// FlycheckDir runs go checkers on a project-relative directory.
// Like Flycheck but operates on a directory rather than deriving from a filename.
//
// Returns:
//   - result: formatted checker output (empty if no issues)
//   - rawIssues: classified issues for stats aggregation (empty if no issues)
//   - suggestion: install hints or fix suggestions (only when err != nil)
//   - err: system error (checker not found, timeout, etc.)
func FlycheckDir(ctx context.Context, dir string) (result string, rawIssues []ClassifiedIssue, suggestion string, err error) {
	checkers, ok := Registry["go"]
	if !ok || len(checkers) == 0 {
		return "", nil, "", nil
	}

	var results []string
	var allIssues []ClassifiedIssue
	for _, checker := range checkers {
		checkerCtx, cancel := context.WithTimeout(ctx, DefaultTimeout)
		output, runErr := runCheckerOnDir(checkerCtx, checker, dir)
		cancel()

		if runErr != nil {
			if isNotFoundError(runErr) {
				return "", nil, checker.InstallHint, fmt.Errorf("%s 未安装", checker.Command)
			}
			if checkerCtx.Err() == context.DeadlineExceeded {
				return "", nil, fmt.Sprintf("%s 检查超时 (%v)，建议简化代码或稍后再试",
					checker.Name, DefaultTimeout), runErr
			}
			continue
		}

		output = strings.TrimSpace(output)
		if output != "" {
			results = append(results, formatCheckerOutput(checker.Name, output))
			allIssues = append(allIssues, ClassifyIssues(output)...)
		}
	}

	if len(results) > 0 {
		result = strings.Join(results, "\n")
	}
	rawIssues = allIssues

	return
}

// runCheckerOnDir is like runChecker but takes a project-relative directory
// path directly instead of deriving it from a filename.
func runCheckerOnDir(ctx context.Context, checker Checker, dir string) (string, error) {
	if _, err := exec.LookPath(checker.Command); err != nil {
		return "", &exec.Error{Name: checker.Command, Err: exec.ErrNotFound}
	}

	absDir := filepath.Join(dsctx.ProjectRoot, dir)
	args := []string{"-tests", absDir}

	var cmdStr strings.Builder
	cmdStr.WriteString(checker.Command)
	for _, arg := range args {
		cmdStr.WriteString(" ")
		cmdStr.WriteString(arg)
	}

	stdout, stderr, runErr := shell.SimpleExecuteSeparate(ctx, cmdStr.String())

	if runErr != nil {
		stdout = strings.TrimSpace(stdout)
		if stdout != "" {
			return stdout, nil
		}
		if stderr != "" {
			return "", fmt.Errorf("%s: %s", checker.Name, strings.TrimSpace(stderr))
		}
		return "", runErr
	}

	return strings.TrimSpace(stdout), nil
}

// FindGoPackages 返回给定目录下所有含有 .go 文件的子目录（相对路径）。
// 如果 recursive 为 true，递归查找所有子目录；否则只查找直接子目录。
// 同时也会检查 baseDir 本身是否含有 .go 文件。
func FindGoPackages(baseDir string, recursive bool) []string {
	var pkgs []string

	absBase := filepath.Join(dsctx.ProjectRoot, baseDir)

	// Check baseDir itself
	if hasGoFiles(absBase) {
		pkgs = append(pkgs, baseDir)
	}

	// Walk subdirectories
	entries, err := os.ReadDir(absBase)
	if err != nil {
		return pkgs
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		subDir := filepath.Join(absBase, entry.Name())
		if recursive {
			// Recursive walk
			filepath.Walk(subDir, func(path string, info os.FileInfo, err error) error {
				if err != nil || !info.IsDir() {
					return nil
				}
				if hasGoFiles(path) {
					rel, _ := filepath.Rel(dsctx.ProjectRoot, path)
					pkgs = append(pkgs, rel)
				}
				return nil
			})
		} else {
			if hasGoFiles(subDir) {
				rel, _ := filepath.Rel(dsctx.ProjectRoot, subDir)
				pkgs = append(pkgs, rel)
			}
		}
	}

	return pkgs
}

// hasGoFiles 检查目录是否包含 .go 文件。
func hasGoFiles(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			return true
		}
	}
	return false
}

// CountGoFiles 统计目录下的 .go 文件数量。
func CountGoFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".go") {
			n++
		}
	}
	return n
}