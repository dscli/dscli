// Package flycheck provides on-the-fly syntax checking for code files,
// inspired by Emacs flycheck. It detects language from file extension,
// finds the appropriate checker, runs it, and returns results.
//
// Starting with Go + staticcheck, the architecture supports adding
// more languages and checkers incrementally.
package flycheck

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/parse"
	"gitcode.com/dscli/dscli/internal/shell"
)

const (
	// DefaultTimeout is the default time limit for a checker run.
	DefaultTimeout = 15 * time.Second
)

// Checker defines a syntax checker for a specific language.
// Each language can have multiple checkers (e.g. go: staticcheck, go vet; python: ruff, flake8).
type Checker struct {
	Name        string // e.g. "go-staticcheck"
	Command     string // e.g. "staticcheck"
	InstallHint string // Human-readable install instruction
	// BuildArgs builds command-line arguments for checking the given file.
	// filename is project-relative, project root is taken from context.ProjectRoot.
	BuildArgs func(filename string) []string
	// BuildDirArgs builds command-line arguments for checking a directory.
	// If nil, directory checking falls back to BuildArgs (may not work for all checkers).
	BuildDirArgs func(dir string) []string
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
			return []string{"-tests", context.ProjectRoot}
		}
		return []string{"-tests", filepath.Join(context.ProjectRoot, pkgDir)}
	},
	BuildDirArgs: func(dir string) []string {
		return []string{"-tests", filepath.Join(context.ProjectRoot, dir)}
	},
}

// pythonRuff runs ruff, a fast Python linter, on Python files/directories.
// Uses concise output format for clean single-line issue reporting.
var pythonRuff = Checker{
	Name:    "python-ruff",
	Command: "ruff",
	InstallHint: `请安装 ruff（快速 Python linter）:
  pip install ruff
  或: brew install ruff`,
	BuildArgs: func(filename string) []string {
		return []string{"check", "--output-format=concise", filepath.Join(context.ProjectRoot, filename)}
	},
	BuildDirArgs: func(dir string) []string {
		return []string{"check", "--output-format=concise", filepath.Join(context.ProjectRoot, dir)}
	},
}

// Registry maps language identifiers (from parse.GuessLanguage) to their checkers.
// Add entries here when supporting new languages.
var Registry = map[string][]Checker{
	"go":     {goStaticcheck},
	"python": {pythonRuff},
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

	return result, suggestion, err
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
	if execErr, ok := errors.AsType[*exec.Error](err); ok {
		return execErr.Err == exec.ErrNotFound
	}
	return false
}

// IssueSeverity classifies the severity of a checker issue.
type IssueSeverity int

const (
	SevError      IssueSeverity = iota // ❌ compile/syntax error
	SevWarning                         // ⚠️ lint warning
	SevSuggestion                      // 💡 improvement suggestion
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
//
// classifyIssueLine determines the severity of a single issue line.
//
// Supports two checker output formats:
//
// 1. staticcheck (Go): file.go:line:col: message (code)
//   - "(compile)" suffix → ❌ compile error
//   - "syntax error" keyword → ❌ compile error
//   - "QF" code → 💡 suggestion
//   - default → ⚠️ warning
//
// 2. ruff (Python): path.py:line:col: RULECODE message
//   - E/F rule codes → ❌ error (pycodestyle errors / pyflakes undefined names)
//   - W rule codes → ⚠️ warning
//   - I/UP/SIM/C4 rule codes → 💡 suggestion (isort, pyupgrade, simplify, comprehensions)
//   - default → ⚠️ warning
func classifyIssueLine(line string) IssueSeverity {
	// === staticcheck patterns ===

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

	// === ruff patterns: extract rule code from "path:line:col: CODE msg" ===
	if code := extractRuffCode(line); code != "" {
		// E: pycodestyle errors (e.g., E501 line too long)
		// F: pyflakes errors (e.g., F401 unused import, F841 unused variable, F821 undefined name)
		if strings.HasPrefix(code, "E") || strings.HasPrefix(code, "F") {
			return SevError
		}
		// W: pycodestyle warnings (e.g., W291 trailing whitespace)
		if strings.HasPrefix(code, "W") {
			return SevWarning
		}
		// Suggestions: I (isort), UP (pyupgrade), SIM (flake8-simplify), C4 (flake8-comprehensions)
		if strings.HasPrefix(code, "I") || strings.HasPrefix(code, "UP") ||
			strings.HasPrefix(code, "SIM") || strings.HasPrefix(code, "C4") {
			return SevSuggestion
		}
		// Other ruff rules (N, D, RUF, PL, etc.) → warning
		return SevWarning
	}

	// Default: warning
	return SevWarning
}

// extractRuffCode extracts a ruff rule code from a diagnostic line.
// Ruff output format: path.py:line:col: CODE message
// Returns empty string if the line doesn't match ruff format.
func extractRuffCode(line string) string {
	// Find the third colon (after file:line:col)
	parts := strings.SplitN(line, ":", 4)
	if len(parts) < 4 {
		return ""
	}
	// parts[3] starts with " CODE message"
	rest := strings.TrimSpace(parts[3])
	// Extract the first word (rule code)
	code, _, found := strings.Cut(rest, " ")
	if !found {
		return ""
	}
	if len(code) >= 2 && len(code) <= 8 {
		allUpper := true
		for _, ch := range code {
			if !((ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')) {
				allUpper = false
				break
			}
		}
		if allUpper {
			// Must contain at least one letter - not purely numeric
			hasLetter := false
			for _, ch := range code {
				if ch >= 'A' && ch <= 'Z' {
					hasLetter = true
					break
				}
			}
			if hasLetter {
				return code
			}
		}
	}
	return ""
}

// ClassifiedIssue holds a single checker issue with its severity.
type ClassifiedIssue struct {
	Severity IssueSeverity
	Line     string
}

// ClassifyIssues splits raw checker output into classified issues,
// filtering out summary/boilerplate lines (e.g. ruff's "Found N errors").
func ClassifyIssues(raw string) []ClassifiedIssue {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	issues := make([]ClassifiedIssue, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		// Skip non-diagnostic summary lines from various checkers
		if isSummaryLine(line) {
			continue
		}
		issues = append(issues, ClassifiedIssue{
			Severity: classifyIssueLine(line),
			Line:     line,
		})
	}
	return issues
}

// isSummaryLine returns true if the line is a checker summary (not a diagnostic).
// Filters out lines like "Found 3 errors.", "[*] 2 fixable...", "No errors found.", etc.
func isSummaryLine(line string) bool {
	// ruff summary patterns
	if strings.HasPrefix(line, "Found ") || strings.HasPrefix(line, "[*] ") {
		return true
	}
	if strings.HasPrefix(line, "No errors") {
		return true
	}
	// staticcheck sometimes emits empty package-level messages
	if line == "-" {
		return true
	}
	return false
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

// FlycheckDir runs checkers on a project-relative directory for the given language.
//
// Returns:
//   - result: formatted checker output (empty if no issues)
//   - rawIssues: classified issues for stats aggregation (empty if no issues)
//   - suggestion: install hints or fix suggestions (only when err != nil)
//   - err: system error (checker not found, timeout, etc.)
func FlycheckDir(ctx context.Context, lang, dir string) (result string, rawIssues []ClassifiedIssue, suggestion string, err error) {
	checkers, ok := Registry[lang]
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

	return result, rawIssues, suggestion, err
}

// runCheckerOnDir is like runChecker but takes a project-relative directory
// path directly instead of deriving it from a filename.
// Uses checker.BuildDirArgs if available; otherwise falls back to BuildArgs.
func runCheckerOnDir(ctx context.Context, checker Checker, dir string) (string, error) {
	if _, err := exec.LookPath(checker.Command); err != nil {
		return "", &exec.Error{Name: checker.Command, Err: exec.ErrNotFound}
	}

	// Use BuildDirArgs if defined, otherwise fall back to BuildArgs with dir path.
	var args []string
	if checker.BuildDirArgs != nil {
		args = checker.BuildDirArgs(dir)
	} else {
		args = checker.BuildArgs(dir)
	}

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

	absBase := filepath.Join(context.ProjectRoot, baseDir)

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
					rel, _ := filepath.Rel(context.ProjectRoot, path)
					pkgs = append(pkgs, rel)
				}
				return nil
			})
		} else {
			if hasGoFiles(subDir) {
				rel, _ := filepath.Rel(context.ProjectRoot, subDir)
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

// FindPyFiles 返回给定目录下所有 .py 文件的相对路径。
// 如果 recursive 为 true，递归查找所有子目录；否则只查找目录本身。
func FindPyFiles(baseDir string, recursive bool) []string {
	absBase := filepath.Join(context.ProjectRoot, baseDir)
	var files []string

	if recursive {
		filepath.Walk(absBase, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if !info.IsDir() && strings.HasSuffix(info.Name(), ".py") {
				rel, _ := filepath.Rel(context.ProjectRoot, path)
				files = append(files, rel)
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(absBase)
		if err != nil {
			return files
		}
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
				files = append(files, filepath.Join(baseDir, e.Name()))
			}
		}
	}

	return files
}

// CountPyFiles 统计目录下的 .py 文件数量（递归）。
func CountPyFiles(dir string) int {
	absDir := filepath.Join(context.ProjectRoot, dir)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".py") {
			n++
		}
	}
	return n
}
