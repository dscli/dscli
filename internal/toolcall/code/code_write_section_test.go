package code

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/parse"
)

// =============================================================================
// Unit tests for writeToFile — the core write logic
// =============================================================================

func TestWriteToFile_BasicReplace(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\nline3\nline4\nline5\nline6\nline7\nline8\nline9\nline10\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	err := writeToFile(path, lines, 3, 5, "NEW", true)
	if err != nil {
		t.Fatalf("writeToFile failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := "line1\nline2\nNEW\nline6\nline7\nline8\nline9\nline10\n"
	if string(got) != want {
		t.Errorf("writeToFile result mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestWriteToFile_ReplaceFirstLine(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\nline3\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	err := writeToFile(path, lines, 1, 1, "NEW", true)
	if err != nil {
		t.Fatalf("writeToFile failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := "NEW\nline2\nline3\n"
	if string(got) != want {
		t.Errorf("writeToFile result mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestWriteToFile_ReplaceLastLine(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\nline3\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	err := writeToFile(path, lines, 3, 3, "LAST", true)
	if err != nil {
		t.Fatalf("writeToFile failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := "line1\nline2\nLAST\n"
	if string(got) != want {
		t.Errorf("writeToFile result mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestWriteToFile_ReplaceEntireFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\nline3\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	err := writeToFile(path, lines, 1, 3, "ALL NEW", true)
	if err != nil {
		t.Fatalf("writeToFile failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := "ALL NEW\n"
	if string(got) != want {
		t.Errorf("writeToFile result mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestWriteToFile_NoTrailingNewlinePreserved(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\nline3"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	err := writeToFile(path, lines, 2, 2, "NEW", false)
	if err != nil {
		t.Fatalf("writeToFile failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := "line1\nNEW\nline3"
	if string(got) != want {
		t.Errorf("writeToFile result mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestWriteToFile_MultiLineReplacement(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\nline3\nline4\nline5\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	err := writeToFile(path, lines, 2, 4, "NEW1\nNEW2", true)
	if err != nil {
		t.Fatalf("writeToFile failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := "line1\nNEW1\nNEW2\nline5\n"
	if string(got) != want {
		t.Errorf("writeToFile result mismatch:\n got: %q\nwant: %q", string(got), want)
	}
}

func TestWriteToFile_BoundsCheckStartLine(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	// startLine=0 should now return an error, not panic
	err := writeToFile(path, lines, 0, 1, "X", true)
	if err == nil {
		t.Error("expected error for startLine=0, got nil")
	}
	t.Logf("got expected error: %v", err)
}

func TestWriteToFile_BoundsCheckEndLine(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	// endLine > len(lines) should now return an error, not panic
	err := writeToFile(path, lines, 1, 100, "X", true)
	if err == nil {
		t.Error("expected error for endLine > len(lines), got nil")
	}
	t.Logf("got expected error: %v", err)
}

func TestWriteToFile_BoundsCheckEndBeforeStart(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	orig := "line1\nline2\nline3\n"
	os.WriteFile(path, []byte(orig), 0644)

	lines := splitAndStrip(orig)

	// endLine < startLine should return an error
	err := writeToFile(path, lines, 3, 1, "X", true)
	if err == nil {
		t.Error("expected error for endLine < startLine, got nil")
	}
	t.Logf("got expected error: %v", err)
}

// =============================================================================
// Tests for locateSectionRange
// =============================================================================

func TestLocateSectionRange_Lines(t *testing.T) {
	structure := &parse.FileStructure{}
	lines := []string{"a", "b", "c", "d", "e"}

	start, end, err := locateSectionRange(structure, lines, "lines:2-4")
	if err != nil {
		t.Fatalf("locateSectionRange failed: %v", err)
	}
	if start != 2 || end != 4 {
		t.Errorf("got start=%d end=%d, want start=2 end=4", start, end)
	}
}

func TestLocateSectionRange_LinesEndBeyondFile(t *testing.T) {
	structure := &parse.FileStructure{}
	lines := []string{"a", "b", "c"}

	start, end, err := locateSectionRange(structure, lines, "lines:2-100")
	if err != nil {
		t.Fatalf("locateSectionRange failed: %v", err)
	}
	if start != 2 || end != 3 {
		t.Errorf("got start=%d end=%d, want start=2 end=3 (truncated)", start, end)
	}
}

func TestLocateSectionRange_LinesStartBeyondFile(t *testing.T) {
	structure := &parse.FileStructure{}
	lines := []string{"a", "b"}

	_, _, err := locateSectionRange(structure, lines, "lines:5-10")
	if err == nil {
		t.Error("expected error for start > len(lines), got nil")
	}
}

func TestLocateSectionRange_LinesInvalidRange(t *testing.T) {
	structure := &parse.FileStructure{}
	lines := []string{"a", "b", "c"}

	_, _, err := locateSectionRange(structure, lines, "lines:5-3")
	if err == nil {
		t.Error("expected error for start > end, got nil")
	}
}

// =============================================================================
// Integration test: writeCodeSection with function selector
// =============================================================================

func TestWriteCodeSection_FunctionSelector(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	goCode := "package test\n\nfunc Foo() int {\n\tx := 1\n\ty := 2\n\treturn x + y\n}\n\nfunc Bar() string {\n\treturn \"hello\"\n}\n"
	os.WriteFile(path, []byte(goCode), 0644)

	ctx := context.Background()

	_, err := writeCodeSection(ctx, path, "function:Foo",
		"func Foo() int {\n\treturn 42\n}")
	if err != nil {
		t.Fatalf("writeCodeSection failed: %v", err)
	}

	got, _ := os.ReadFile(path)
	expected := "package test\n\nfunc Foo() int {\n\treturn 42\n}\n\nfunc Bar() string {\n\treturn \"hello\"\n}\n"
	if string(got) != expected {
		t.Errorf("writeCodeSection result mismatch:\n got: %q\nwant: %q", string(got), expected)
	}
}

func TestWriteCodeSection_FunctionEndLineDoesNotOvershoot(t *testing.T) {
	// Verifies that replacing a function only affects the function itself,
	// not content before or after it.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	goCode := `package test

// Before comment
func TestFoo(t *testing.T) {
	tmpDir := t.TempDir()
	t.Log("setup")
	for i := 0; i < 10; i++ {
		t.Run("sub", func(t *testing.T) {
			t.Log(i)
		})
	}
}

// After comment
func TestBar(t *testing.T) {
	t.Log("bar")
}
`
	os.WriteFile(path, []byte(goCode), 0644)

	ctx := context.Background()

	_, err := writeCodeSection(ctx, path, "function:TestFoo",
		"func TestFoo(t *testing.T) {\n\tt.Skip(\"replaced\")\n}")
	if err != nil {
		t.Fatalf("writeCodeSection failed: %v", err)
	}

	got := string(mustReadFile(path))

	if !strings.Contains(got, `t.Skip("replaced")`) {
		t.Error("function body was not replaced")
	}
	if strings.Contains(got, `t.Log("setup")`) {
		t.Error("old function body lines remain after replacement")
	}
	if !strings.Contains(got, "// Before comment") {
		t.Error("content before function was lost")
	}
	if !strings.Contains(got, "// After comment") {
		t.Error("content after function was lost")
	}
	if !strings.Contains(got, "func TestBar") {
		t.Error("function after TestFoo was lost")
	}
	if strings.Count(got, "package test") != 1 {
		t.Errorf("package declaration appears %d times (expected 1) — possible duplication", strings.Count(got, "package test"))
	}
}

func TestWriteCodeSection_LinesNoDuplication(t *testing.T) {
	// Verify lines: selector doesn't duplicate content.
	// Use a valid Go file to avoid parse errors.
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	goCode := `package test

import "fmt"

func main() {
	fmt.Println("line4")
	fmt.Println("line5")
	fmt.Println("line6")
}
`
	os.WriteFile(path, []byte(goCode), 0644)

	ctx := context.Background()

	// Replace lines 5-7 (the three fmt.Println lines)
	_, err := writeCodeSection(ctx, path, "lines:5-7",
		"\tfmt.Println(\"new4\")\n\tfmt.Println(\"new5\")\n\tfmt.Println(\"new6\")")
	if err != nil {
		t.Fatalf("writeCodeSection failed: %v", err)
	}

	got := string(mustReadFile(path))

	// Verify old lines are gone
	if strings.Contains(got, `"line4"`) {
		t.Error("old line4 still present")
	}

	// Verify new lines are present
	if !strings.Contains(got, `"new4"`) {
		t.Error("new line4 missing")
	}

	// Check no duplicate package lines
	if strings.Count(got, "package test") != 1 {
		t.Errorf("package appears %d times (expected 1)", strings.Count(got, "package test"))
	}
}

func TestWriteCodeSection_TrailingNewlinePreserved(t *testing.T) {
	// After a function is replaced, the file should still end with \n
	// (if it originally did).
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "test.go")

	goCode := "package test\n\nfunc Foo() int {\n\treturn 1\n}\n"
	os.WriteFile(path, []byte(goCode), 0644)

	ctx := context.Background()

	_, err := writeCodeSection(ctx, path, "function:Foo",
		"func Foo() int {\n\treturn 42\n}")
	if err != nil {
		t.Fatalf("writeCodeSection failed: %v", err)
	}

	got := string(mustReadFile(path))
	if !strings.HasSuffix(got, "\n") {
		t.Error("file lost trailing newline after write")
	}
}

// =============================================================================
// Helpers
// =============================================================================

func splitAndStrip(s string) []string {
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func mustReadFile(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return data
}
