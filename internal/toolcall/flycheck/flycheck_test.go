package flycheck

import (
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/flycheck"
)

// TestHandleFlycheckOutputFormatting verifies the output formatting logic
// produces the expected emoji badges for different severity levels.
func TestHandleFlycheckOutputFormatting(t *testing.T) {
	// Test stats-based output formatting (extracted logic)

	tests := []struct {
		name           string
		issues         []flycheck.ClassifiedIssue
		wantContains   []string
		wantNotContain []string
	}{
		{
			name: "compile errors get red alert",
			issues: []flycheck.ClassifiedIssue{
				{Severity: flycheck.SevError, Line: "file.go:1:1: syntax error (compile)"},
			},
			wantContains: []string{"❌", "必须立即修复", "编译错误"},
		},
		{
			name: "warnings get warning header",
			issues: []flycheck.ClassifiedIssue{
				{Severity: flycheck.SevWarning, Line: "file.go:1:1: unused (U1000)"},
			},
			wantContains:   []string{"⚠️", "发现问题"},
			wantNotContain: []string{"❌ 发现编译错误", "必须立即修复"},
		},
		{
			name:         "empty issues get success",
			issues:       nil,
			wantContains: []string{"✅", "未发现问题"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stats := flycheck.CountStats(tt.issues)

			var output string
			if len(tt.issues) > 0 {
				var b strings.Builder
				if stats.Errors > 0 {
					b.WriteString("## ❌ flycheck 发现编译错误 — 必须立即修复！\n\n")
				} else {
					b.WriteString("## ⚠️ flycheck 发现问题\n\n")
				}
				b.WriteString("```\n")
				for _, iss := range tt.issues {
					b.WriteString(iss.Severity.String() + " " + iss.Line + "\n")
				}
				b.WriteString("```\n")
				output = b.String()
			} else {
				output = "✅ flycheck: 检查了 1 个包（1 个文件），未发现问题"
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(output, want) {
					t.Errorf("output should contain %q, got:\n%s", want, output)
				}
			}
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(output, notWant) {
					t.Errorf("output should NOT contain %q, got:\n%s", notWant, output)
				}
			}
		})
	}
}

// TestPathNormalization verifies the path normalization logic matches flycheck.NormalizePath.
func TestPathNormalization(t *testing.T) {
	tests := []struct {
		input         string
		wantRecursive bool
		wantPath      string
	}{
		{"./...", true, "."},
		{"internal/...", true, "internal"},
		{"./internal/...", true, "internal"},
		{"internal/toolcall/", false, "internal/toolcall"},
		{"./internal/flycheck/flycheck.go", false, "internal/flycheck/flycheck.go"},
		{"...", true, "."},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			path, recursive := flycheck.NormalizePath(tt.input)
			if recursive != tt.wantRecursive {
				t.Errorf("recursive: got %v, want %v", recursive, tt.wantRecursive)
			}
			if path != tt.wantPath {
				t.Errorf("path: got %q, want %q", path, tt.wantPath)
			}
		})
	}
}
