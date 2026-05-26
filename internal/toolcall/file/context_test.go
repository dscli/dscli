package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Tests for AppendEditContext
// =============================================================================

func TestAppendEditContext_SmallEdit(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	// 10-line file: replace line 3 with "NEW"
	orig := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	writeAndReplace(t, path, orig, 3, 3, "NEW\n")

	ctx := AppendEditContext(path, 3, 3, 1, 1)
	if ctx == "" {
		t.Fatal("expected non-empty context")
	}
	// Anchor at line 3 ("NEW"), before=2 lines, after=8 lines (but only 10 total)
	// Should show lines 1-10 (all within 36-line limit)
	if !strings.Contains(ctx, "编辑后上下文") {
		t.Error("context missing header")
	}
	if !strings.Contains(ctx, "NEW") {
		t.Error("context missing new content")
	}
}

func TestAppendEditContext_LineOffsetWarning(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	// 10-line file: replace 3 lines (3-5) with 1 line → delta = -2
	orig := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	writeAndReplace(t, path, orig, 3, 5, "NEW\n")

	ctx := AppendEditContext(path, 3, 5, 3, 1)
	if !strings.Contains(ctx, "行数变化") {
		t.Error("expected offset warning for line count change")
	}
	if !strings.Contains(ctx, "-2") || !strings.Contains(ctx, "偏移") {
		t.Error("expected offset -2 in warning")
	}
}

func TestAppendEditContext_NoOffsetWhenSameCount(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	// Replace exactly 1 line with 1 line → no offset
	orig := "line1\nline2\nline3\nline4\n"
	writeAndReplace(t, path, orig, 2, 2, "NEW\n")

	ctx := AppendEditContext(path, 2, 2, 1, 1)
	if strings.Contains(ctx, "行数变化") {
		t.Error("unexpected offset warning when line count unchanged")
	}
}

func TestAppendEditContext_DeleteLines(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	// Delete lines 3-5 (replaced with empty)
	orig := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	writeAndReplace(t, path, orig, 3, 5, "")

	ctx := AppendEditContext(path, 3, 5, 3, 0)
	if !strings.Contains(ctx, "行数变化") {
		t.Error("expected offset warning after delete")
	}
}

func TestAppendEditContext_LargeEditTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	// 50-line file: replace lines 10-48 with 40 new lines (>36, triggers truncation)
	var b strings.Builder
	for i := 1; i <= 50; i++ {
		b.WriteString("line")
		b.WriteString(strings.Repeat("x", i%10))
		b.WriteString("\n")
	}
	orig := b.String()

	newContent := strings.Repeat("NEW\n", 40)
	replaced := writeAndReplace(t, path, orig, 10, 48, newContent)

	_ = replaced
	ctx := AppendEditContext(path, 10, 48, 39, 40)
	if !strings.Contains(ctx, "行省略") {
		t.Error("expected truncation marker for large edit")
	}
}

func TestAppendEditContext_FileNotFound(t *testing.T) {
	ctx := AppendEditContext("/nonexistent/file.txt", 1, 1, 1, 1)
	if ctx != "" {
		t.Error("expected empty context for nonexistent file")
	}
}

// =============================================================================
// Tests for AppendWriteFileContext
// =============================================================================

func TestAppendWriteFileContext_SmallFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	content := "line1\nline2\nline3\n"
	os.WriteFile(path, []byte(content), 0o644)

	ctx := AppendWriteFileContext(path)
	if !strings.Contains(ctx, "写入后") {
		t.Error("context missing header")
	}
	if !strings.Contains(ctx, "line1") {
		t.Error("context missing content")
	}
}

func TestAppendWriteFileContext_LargeFileTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")

	var b strings.Builder
	for i := 1; i <= 50; i++ {
		b.WriteString("line")
		b.WriteString(strings.Repeat("x", i%10))
		b.WriteString("\n")
	}
	os.WriteFile(path, []byte(b.String()), 0o644)

	ctx := AppendWriteFileContext(path)
	if !strings.Contains(ctx, "行省略") {
		t.Error("expected truncation marker for large file")
	}
	if !strings.Contains(ctx, "首尾") {
		t.Error("expected '首尾' in header for truncated view")
	}
}

func TestAppendWriteFileContext_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(path, []byte(""), 0o644)

	ctx := AppendWriteFileContext(path)
	if !strings.Contains(ctx, "为空") {
		t.Error("expected '为空' for empty file")
	}
}

func TestAppendWriteFileContext_NotFound(t *testing.T) {
	ctx := AppendWriteFileContext("/nonexistent/file.txt")
	if ctx != "" {
		t.Error("expected empty context for nonexistent file")
	}
}

// =============================================================================
// Helpers
// =============================================================================

// writeAndReplace writes orig to path, then replaces lines [start,end] with newContent.
// Returns the resulting file content.
func writeAndReplace(t *testing.T, path, orig string, start, end int, newContent string) string {
	t.Helper()

	os.WriteFile(path, []byte(orig), 0o644)

	// Read and split
	lines, err := contextReadLines(path)
	if err != nil {
		t.Fatalf("contextReadLines: %v", err)
	}

	// Slice for replacement: lines[start-1:end] → newContent
	newLines := strings.Split(newContent, "\n")
	// Remove trailing empty from Split
	if len(newLines) > 0 && newLines[len(newLines)-1] == "" {
		newLines = newLines[:len(newLines)-1]
	}

	var result []string
	result = append(result, lines[:start-1]...)
	result = append(result, newLines...)
	if end < len(lines) {
		result = append(result, lines[end:]...)
	}

	out := strings.Join(result, "\n") + "\n"
	os.WriteFile(path, []byte(out), 0o644)
	return out
}
