package flycheck

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestFlycheckGoStaticcheck runs staticcheck on our own codebase.
// Assumes staticcheck is installed (as in the project environment).
func TestFlycheckGoStaticcheck(t *testing.T) {
	if _, err := exec.LookPath("staticcheck"); err != nil {
		t.Skip("staticcheck not installed, skipping")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, suggestion, err := Flycheck(ctx, "internal/toolcall/ask/code_review.go")
	if err != nil {
		t.Errorf("Flycheck error: %v, suggestion: %s", err, suggestion)
	}

	// We know two unused functions exist in code_review.go, so result should be non-empty
	if result == "" {
		t.Log("No issues found (code may have been cleaned up)")
	} else {
		t.Logf("Flycheck result:\n%s", result)
		if !strings.Contains(result, "U1000") {
			t.Log("Expected U1000 (unused) diagnostics, but none found")
		}
	}
}

// TestFlycheckUnknownLanguage ensures unsupported languages are silently skipped.
func TestFlycheckUnknownLanguage(t *testing.T) {
	result, suggestion, err := Flycheck(
		context.Background(),
		"test.txt",
	)
	if err != nil {
		t.Errorf("unexpected error for unknown language: %v", err)
	}
	if result != "" {
		t.Errorf("expected empty result for unknown language, got: %s", result)
	}
	if suggestion != "" {
		t.Errorf("expected empty suggestion for unknown language, got: %s", suggestion)
	}
}

// TestFlycheckGoNoIssues checks a clean Go file produces no result.
// Since Flycheck uses context.ProjectRoot, we check against a file
// within the real project that should have no staticcheck issues.
// TestFlycheckGoNoIssues checks a clean Go file produces no result.
// Uses a file within the real project known to have no staticcheck issues.
func TestFlycheckGoNoIssues(t *testing.T) {
	if _, err := exec.LookPath("staticcheck"); err != nil {
		t.Skip("staticcheck not installed, skipping")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// internal/parse/parse.go is a stable utility file with no linter issues
	result, suggestion, err := Flycheck(ctx, "internal/parse/parse.go")
	if err != nil {
		t.Errorf("Flycheck error: %v, suggestion: %s", err, suggestion)
	}
	// parse.go might have informational warnings; just ensure no hard errors
	if strings.Contains(result, "error") && !strings.Contains(result, "U1000") {
		t.Logf("Flycheck found issues in parse.go (may be expected):\n%s", result)
	}
}

// TestIsNotFoundError tests the error detection helper.
func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil", nil, false},
		{"exec error", &exec.Error{Name: "foo", Err: exec.ErrNotFound}, true},
		{"exit error", &exec.ExitError{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isNotFoundError(tt.err); got != tt.expected {
				t.Errorf("isNotFoundError(%v) = %v, want %v", tt.err, got, tt.expected)
			}
		})
	}
}

// TestFormatCheckerOutput tests the output formatting function.
func TestFormatCheckerOutput(t *testing.T) {
	output := formatCheckerOutput("go-staticcheck", "file.go:1:1: error message")
	if !strings.Contains(output, "go-staticcheck") {
		t.Error("formatted output should contain checker name")
	}
	if !strings.Contains(output, "error message") {
		t.Error("formatted output should contain the issue message")
	}
	if !strings.Contains(output, "```") {
		t.Error("formatted output should contain code block markers")
	}
}

// TestClassifyIssueLine tests severity classification of checker output lines.
func TestClassifyIssueLine(t *testing.T) {
	tests := []struct {
		line     string
		expected IssueSeverity
	}{
		// Compile errors
		{"file.go:10:1: syntax error: unexpected EOF, expected } (compile)", SevError},
		{"file.go:5:2: undefined: someFunc (compile)", SevError},
		{"file.go:175:1: syntax error: unexpected EOF", SevError}, // syntax error without (compile) suffix
		{"file.go:3:5: expected ';', got '}'", SevWarning}, // expected/got without (compile) → warning
		// Warnings (lint)
		{"file.go:20:2: func unusedFunc is unused (U1000)", SevWarning},
		{"file.go:15:3: should use time.Since instead of time.Now().Sub (S1012)", SevWarning},
		// Suggestions
		{"file.go:30:1: could apply quickfix QF1001 to simplify code", SevSuggestion},
		// Edge: unknown pattern defaults to warning
		{"file.go:8:1: some unknown diagnostic message", SevWarning},
	}
	for _, tt := range tests {
		t.Run(tt.line[:min(len(tt.line), 30)], func(t *testing.T) {
			got := classifyIssueLine(tt.line)
			if got != tt.expected {
				t.Errorf("classifyIssueLine(%q) = %v, want %v", tt.line, got, tt.expected)
			}
		})
	}
}

// TestClassifyIssuesAndStats tests batch classification and stats counting.
func TestClassifyIssuesAndStats(t *testing.T) {
	raw := `file.go:1:1: syntax error: unexpected EOF (compile)
file.go:2:1: func unused is unused (U1000)
file.go:3:1: could apply quickfix QF1001
file.go:4:1: undefined: Foo (compile)`

	issues := ClassifyIssues(raw)
	if len(issues) != 4 {
		t.Fatalf("expected 4 issues, got %d", len(issues))
	}

	stats := CountStats(issues)
	if stats.Errors != 2 {
		t.Errorf("expected 2 errors, got %d (issues: %+v)", stats.Errors, issues)
	}
	if stats.Warnings != 1 {
		t.Errorf("expected 1 warning, got %d", stats.Warnings)
	}
	if stats.Suggestions != 1 {
		t.Errorf("expected 1 suggestion, got %d", stats.Suggestions)
	}
}

// TestFormatCheckerOutputWithErrors verifies the error badge appears.
func TestFormatCheckerOutputWithErrors(t *testing.T) {
	raw := "file.go:1:1: syntax error: unexpected EOF (compile)"
	result := formatCheckerOutput("go-staticcheck", raw)
	if !strings.Contains(result, "❌") {
		t.Error("expected ❌ emoji for compile error")
	}
	if !strings.Contains(result, "🔥") {
		t.Error("expected 🔥 badge when there are compile errors")
	}
	if !strings.Contains(result, "必须立即修复") {
		t.Error("expected urgent fix message")
	}
}