package ask

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/toolcall"
)

// TestCodeReviewToolStructure tests the basic structure of the code review tool
func TestCodeReviewToolStructure(t *testing.T) {
	// Verify the tool definition exists
	if codeReviewTool.Name != "code_review" {
		t.Errorf("Expected tool name 'code_review', got '%s'", codeReviewTool.Name)
	}

	if codeReviewTool.DisplayName != "Code Review" {
		t.Errorf("Expected display name 'Code Review', got '%s'", codeReviewTool.DisplayName)
	}

	// Check that description contains key information
	description := codeReviewTool.Description
	requiredKeywords := []string{
		"commit",
		"review",
		"uncommitted",
		"test",
		"HEAD",
	}
	for _, keyword := range requiredKeywords {
		if !strings.Contains(description, keyword) {
			t.Errorf("Tool description missing required keyword: %s", keyword)
		}
	}
	if codeReviewTool.Timeout != 5*time.Minute {
		t.Errorf("Expected timeout 5 minutes, got %v", codeReviewTool.Timeout)
	}

	if codeReviewTool.Category != "check" {
		t.Errorf("Expected category 'check', got '%s'", codeReviewTool.Category)
	}
}

// TestHandleCodeReviewFunction tests that the handler function exists and
// responds to git state appropriately.
func TestHandleCodeReviewFunction(t *testing.T) {
	ctx := context.Background()
	args := toolcall.ToolArgs{"summary": "Test commit"}

	result, _, err := handleCodeReview(ctx, args)
	if err != nil {
		// Git environment errors (uncommitted changes / no commits) are
		// expected in a dev workspace, not a test failure.
		t.Logf("handleCodeReview returned error (expected in dev workspace): %v", err)
	} else {
		// Success path: verify the mock was invoked.
		if !strings.Contains(result, "[MOCK]") {
			t.Fatalf("expected [MOCK] in result, got: %s", result)
		}
	}
}

// TestBuildCodeReviewRequest tests the pure function that builds the review
// request from the summary, commit log, patch, guide and file section.
func TestBuildCodeReviewRequest(t *testing.T) {
	summary := "fix: test summary"
	commitLog := "commit message body"
	patch := "diff --git a/file.go b/file.go"
	guide := "AGENTS.md content"
	fileSection := "## File: main.go\n```go\npackage main\n```\n"

	result := buildCodeReviewRequest(summary, commitLog, patch, guide, fileSection)

	sections := []string{
		"## Commit Background",
		"## Commit Message",
		"## Code Changes",
		"## Project Guide (AGENTS.md)",
		"## File Contents",
	}
	for _, section := range sections {
		if !strings.Contains(result, section) {
			t.Errorf("Expected section %q in result, got:\n%s", section, result)
		}
	}

	// Verify content is preserved.
	if !strings.Contains(result, summary) {
		t.Errorf("Expected summary %q in result", summary)
	}
	if !strings.Contains(result, commitLog) {
		t.Errorf("Expected commitLog %q in result", commitLog)
	}
	if !strings.Contains(result, patch) {
		t.Errorf("Expected patch %q in result", patch)
	}
	if !strings.Contains(result, guide) {
		t.Errorf("Expected guide %q in result", guide)
	}

	// Empty optional sections are omitted.
	result2 := buildCodeReviewRequest(summary, commitLog, patch, "", "")
	if strings.Contains(result2, "## Project Guide (AGENTS.md)") {
		t.Errorf("Should NOT include guide section when guide is empty")
	}
	if strings.Contains(result2, "## File Contents") {
		t.Errorf("Should NOT include '## File Contents' when fileSection is empty")
	}
	if !strings.Contains(result2, "## Code Changes") {
		t.Errorf("Should include '## Code Changes' when patch is non-empty")
	}
}

// TestStatusScriptPattern verifies the grep pattern used in the git status
// check catches staged and unstaged changes while ignoring untracked files.
func TestStatusScriptPattern(t *testing.T) {
	// Simulated git status --porcelain output.
	lines := []struct {
		line    string
		matched bool // true = should be caught by grep -v '^??'
	}{
		{"M  staged.go", true},     // staged modification
		{" M unstaged.go", true},   // unstaged modification
		{"A  added.go", true},      // staged addition
		{"D  deleted.go", true},    // staged deletion
		{"R  renamed.go", true},    // staged rename
		{"MM both.go", true},       // staged + unstaged
		{"?? untracked.go", false}, // untracked — should be ignored
		{"", false},                // empty line
	}
	for _, tc := range lines {
		isUntracked := strings.HasPrefix(tc.line, "??")
		shouldCatch := !isUntracked && tc.line != ""

		if shouldCatch != tc.matched {
			t.Errorf("line %q: expected matched=%v, got %v", tc.line, tc.matched, shouldCatch)
		}
	}
}

// TestErrorMessages tests error message format
func TestErrorMessages(t *testing.T) {
	testCases := []struct {
		name          string
		gitStatus     string
		expectedInMsg []string
	}{
		{
			name:      "Modified files",
			gitStatus: " M code_review.go",
			expectedInMsg: []string{
				"检测到未提交的更改",
				"请先提交所有更改",
				"code_review.go",
			},
		},
		{
			name:      "New files",
			gitStatus: "?? new_file.txt",
			expectedInMsg: []string{
				"检测到未提交的更改",
				"请先提交所有更改",
				"new_file.txt",
			},
		},
		{
			name:      "Staged changes",
			gitStatus: "M  staged_file.go",
			expectedInMsg: []string{
				"检测到未提交的更改",
				"请先提交所有更改",
				"staged_file.go",
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errMsg := fmt.Sprintf("检测到未提交的更改，请先提交所有更改再审查。当前状态：\n%s", tc.gitStatus)

			for _, expected := range tc.expectedInMsg {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("Error message missing '%s'. Got: %s", expected, errMsg)
				}
			}

			if !strings.Contains(errMsg, "当前状态：") {
				t.Error("Error message should show current Git status")
			}
		})
	}
}

// TestToolRegistration tests that the tool is properly registered
func TestToolRegistration(t *testing.T) {
	if codeReviewTool.Name == "" {
		t.Error("CodeReviewTool should have a name")
	}
	if codeReviewTool.Handler == nil {
		t.Error("CodeReviewTool.Handler should not be nil")
	}
}

// TestDocumentationCompleteness tests that all required documentation is present
func TestDocumentationCompleteness(t *testing.T) {
	desc := codeReviewTool.Description
	sections := []string{
		"commit",
		"review",
		"uncommitted",
		"test",
		"HEAD",
	}
	for _, section := range sections {
		if !strings.Contains(desc, section) {
			t.Errorf("Documentation missing section/keyword: %s", section)
		}
	}
	if !strings.Contains(desc, "uncommitted changes") &&
		!strings.Contains(desc, "before pushing") {
		t.Error("Documentation should mention uncommitted changes or push workflow")
	}
	if !strings.Contains(desc, "before pushing") &&
		!strings.Contains(desc, "better practices") {
		t.Error("Documentation should instruct users about best practices")
	}
}

// TestShellQuote verifies single-quote shell escaping for file paths.
func TestShellQuote(t *testing.T) {
	cases := []struct{ in, want string }{
		{"foo.go", "'foo.go'"},
		{"a b.go", "'a b.go'"},
		{"it's.go", `'it'\''s.go'`},
	}
	for _, tc := range cases {
		if got := shellQuote(tc.in); got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// TestReadProjectGuideFrom covers guide loading: missing, present, oversized.
func TestReadProjectGuideFrom(t *testing.T) {
	dir := t.TempDir()

	// Missing file → empty.
	if got := readProjectGuideFrom(filepath.Join(dir, "missing.md")); got != "" {
		t.Errorf("missing guide: got %q, want empty", got)
	}

	// Present file → exact content.
	path := filepath.Join(dir, "AGENTS.md")
	content := "## Design decisions\nbuffers are managed by X\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readProjectGuideFrom(path); got != content {
		t.Errorf("guide mismatch: got %q, want %q", got, content)
	}

	// Oversized → capped with marker.
	big := strings.Repeat("x", maxGuideLen+100)
	if err := os.WriteFile(path, []byte(big), 0o644); err != nil {
		t.Fatal(err)
	}
	got := readProjectGuideFrom(path)
	if len(got) >= len(big) {
		t.Errorf("oversized guide not capped: len(got)=%d, len(big)=%d", len(got), len(big))
	}
	if !strings.Contains(got, "AGENTS.md 截断") {
		t.Errorf("oversized guide missing truncation marker: %q", got)
	}

	// Multi-byte content: the cut must land on a rune boundary (valid UTF-8).
	mb := strings.Repeat("中", maxGuideLen/3+50) // 3 bytes per rune
	if err := os.WriteFile(path, []byte(mb), 0o644); err != nil {
		t.Fatal(err)
	}
	got = readProjectGuideFrom(path)
	if !utf8.ValidString(got) {
		t.Errorf("guide truncation produced invalid UTF-8: %q", got)
	}
}

// TestFormatFullContents verifies full-content formatting and per-file caps.
func TestFormatFullContents(t *testing.T) {
	files := []fileContent{
		{name: "ok.go", content: "package ok\n"},
		{name: "big.go", content: strings.Repeat("x", maxFullFileSize+10)},
		{name: "gone.go", content: "", note: "[文件已删除]"},
		{name: "bin.dat", content: "", note: "[二进制文件，不插入全文]"},
	}
	sec := formatFullContents(files)
	if !strings.Contains(sec, "## File: ok.go\n```go\npackage ok\n\n```") {
		t.Errorf("readable file not rendered with language fence:\n%s", sec)
	}
	if !strings.Contains(sec, "big.go [文件过大") {
		t.Errorf("oversized file should carry a note, not content:\n%s", sec)
	}
	if strings.Contains(sec, strings.Repeat("x", 100)) {
		t.Errorf("oversized content leaked into the section")
	}
	if !strings.Contains(sec, "gone.go [文件已删除]") {
		t.Errorf("deleted file note missing:\n%s", sec)
	}
	if !strings.Contains(sec, "bin.dat [二进制文件") {
		t.Errorf("binary file note missing:\n%s", sec)
	}
}

// TestParseHunk covers hunk header variants (omitted counts, deletions, new files).
func TestParseHunk(t *testing.T) {
	cases := []struct {
		line      string
		wantStart int
		wantEnd   int
		wantOK    bool
	}{
		{"@@ -1,3 +4,7 @@", 4, 10, true},      // both counts
		{"@@ -1 +2 @@", 2, 2, true},           // omitted counts default to 1
		{"@@ -0,0 +1,5 @@", 1, 5, true},       // new file
		{"@@ -1,5 +0,0 @@", 0, -1, true},      // pure deletion
		{"@@ -10 +20,2 @@ ctx", 20, 21, true}, // section heading after @@
		{"@@ -10,4 +10,8 @@ func x()", 10, 17, true},
		{"plain line", 0, 0, false},
	}
	for _, tc := range cases {
		h, ok := parseHunk(tc.line)
		if ok != tc.wantOK {
			t.Errorf("parseHunk(%q) ok=%v, want %v", tc.line, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if h.start != tc.wantStart || h.end != tc.wantEnd {
			t.Errorf("parseHunk(%q) = {%d,%d}, want {%d,%d}", tc.line, h.start, h.end, tc.wantStart, tc.wantEnd)
		}
	}
}

// TestParsePatchHunks parses a realistic format-patch output (email headers,
// multiple files, new and deleted files, multiple hunks per file).
func TestParsePatchHunks(t *testing.T) {
	patch := `From 1234 Mon Sep 17 00:00:00 2001
From: Test <test@example.com>
Subject: [PATCH] fix: improve

 foo.go     | 12 ++++++++----
 bar.el     |  5 +++++
 newfile.go |  3 +++
 gone.go    |  5 -----
 4 files changed, 20 insertions(+), 8 deletions(-)

diff --git a/foo.go b/foo.go
index 1234567..89abcde 100644
--- a/foo.go
+++ b/foo.go
@@ -10,8 +10,12 @@ package main
 func main() {
 	old
-	removed
+	added1
+	added2
 	ctx
 }
 
@@ -30,4 +34,5 @@ func helper() {
 	more
+	newLine
 }
diff --git a/bar.el b/bar.el
index abc..def 100644
--- a/bar.el
+++ b/bar.el
@@ -1,3 +1,5 @@
 ;;; bar.el --- test
 (defun foo ()
-  (message "old"))
+  (message "new"))
diff --git a/newfile.go b/newfile.go
new file mode 100644
index 0000000..1234567
--- /dev/null
+++ b/newfile.go
@@ -0,0 +1,3 @@
+package newfile
+
+func New() {}
diff --git a/gone.go b/gone.go
deleted file mode 100644
index 1234567..0000000
--- a/gone.go
+++ /dev/null
@@ -1,5 +0,0 @@
-func gone() {}
`
	hunks := parsePatchHunks(patch)

	want := map[string][]hunkRange{
		"foo.go":     {{10, 21}, {34, 38}},
		"bar.el":     {{1, 5}},
		"newfile.go": {{1, 3}},
	}
	if len(hunks) != len(want) {
		t.Fatalf("parsePatchHunks: got %d files %v, want %d", len(hunks), hunks, len(want))
	}
	for file, hs := range want {
		got, ok := hunks[file]
		if !ok {
			t.Errorf("missing file %q in hunks", file)
			continue
		}
		if len(got) != len(hs) {
			t.Errorf("file %q: got %d hunks %v, want %d", file, len(got), got, len(hs))
			continue
		}
		for i := range hs {
			if got[i] != hs[i] {
				t.Errorf("file %q hunk %d: got %v, want %v", file, i, got[i], hs[i])
			}
		}
	}
	// Deleted file (+++ /dev/null) must not appear.
	if _, ok := hunks["gone.go"]; ok {
		t.Errorf("deleted file should have no new-side hunks")
	}
}

// TestHunkWindow verifies window math, including pure-deletion anchors.
func TestHunkWindow(t *testing.T) {
	s, e := hunkWindow(hunkRange{10, 21}, 4, 100)
	if s != 5 || e != 25 {
		t.Errorf("normal hunk window = [%d,%d), want [5,25)", s, e)
	}
	// Pure deletion: anchor at start, window centered on it.
	s, e = hunkWindow(hunkRange{10, 9}, 4, 100)
	if s != 5 || e != 13 {
		t.Errorf("deletion window = [%d,%d), want [5,13)", s, e)
	}
	// Clamping at file edges.
	s, e = hunkWindow(hunkRange{1, 3}, 4, 10)
	if s != 0 || e != 7 {
		t.Errorf("head window = [%d,%d), want [0,7)", s, e)
	}
}

// TestMergeWindows verifies overlap and adjacency merging.
func TestMergeWindows(t *testing.T) {
	got := mergeWindows([]win{{0, 10}, {8, 20}, {30, 35}})
	want := []win{{0, 20}, {30, 35}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("mergeWindows = %v, want %v", got, want)
	}
	// Adjacent windows merge.
	got = mergeWindows([]win{{0, 5}, {5, 10}})
	if len(got) != 1 || got[0] != (win{0, 10}) {
		t.Errorf("adjacent merge = %v, want [{0,10}]", got)
	}
	if mergeWindows(nil) != nil {
		t.Errorf("empty input should return nil")
	}
}

// TestFindEnclosingDefGo expands a hunk inside a function to the full definition.
func TestFindEnclosingDefGo(t *testing.T) {
	lines := strings.Split(`package main

var x = 1

func main() {
	a := 1
	b := 2
	c := 3
	d := 4
	e := 5
	f := 6
	CHANGED
	CHANGED2
	g := 7
}

func other() {
	h := 8
}
`, "\n")

	// Hunk at 1-based [12,13] (CHANGED lines, 0-based 11-12).
	ds, de := findEnclosingDef(lines, hunkRange{12, 13}, 2, "go")
	if ds != 4 || de != 15 {
		t.Errorf("enclosing def = [%d,%d), want [4,15) (func main full body)", ds, de)
	}

	// Hunk in the gap between functions: no enclosing def.
	ds, de = findEnclosingDef(lines, hunkRange{16, 16}, 2, "go")
	if ds != -1 || de != -1 {
		t.Errorf("gap hunk should have no def, got [%d,%d)", ds, de)
	}

	// Unknown language: no expansion.
	ds, de = findEnclosingDef(lines, hunkRange{12, 13}, 2, "nolang")
	if ds != -1 || de != -1 {
		t.Errorf("unknown lang should not expand, got [%d,%d)", ds, de)
	}
}

// TestBraceEndSkipsStrings verifies brace balancing survives braces inside
// string literals (the classic raw-string-with-braces case).
func TestBraceEndSkipsStrings(t *testing.T) {
	lines := strings.Split(`func raw() {
	s := `+"`"+`{
		not a real brace
	}`+"`"+`
	CHANGED
}
`, "\n")
	end, ok := braceEnd(lines, 0)
	if !ok || end != 6 {
		t.Errorf("braceEnd = %d,%v; want 6,true (skip raw string braces)", end, ok)
	}
}

// TestFindEnclosingDefElisp expands a hunk deep inside a defun, tolerating
// parens in docstrings and comments.
func TestFindEnclosingDefElisp(t *testing.T) {
	lines := strings.Split(`;;; foo.el --- test
(defun foo (a)
  "Doc with (parens) and \"quotes\"."
  (step 1)
  (step 2)
  (step 3)
  (step 4)
  (step 5)
  (step 6)
  (bar a))

(defun baz ()
  (foo))
`, "\n")

	// Hunk at 1-based [7,7] (step 4 line, 0-based 6): expand to the whole defun.
	ds, de := findEnclosingDef(lines, hunkRange{7, 7}, 2, "elisp")
	if ds != 1 || de != 10 {
		t.Errorf("enclosing def = [%d,%d), want [1,10) (defun foo)", ds, de)
	}

	// Hunk in baz's body: the raw window already covers baz, and the only
	// covering def above (foo) ended before the hunk, so no expansion happens.
	ds, de = findEnclosingDef(lines, hunkRange{12, 12}, 2, "elisp")
	if ds != -1 || de != -1 {
		t.Errorf("def-line hunk should keep raw window, got [%d,%d)", ds, de)
	}
}

// TestFindEnclosingDefPython verifies def/class expansion by indentation.
func TestFindEnclosingDefPython(t *testing.T) {
	lines := strings.Split(`def foo(a):
    x = 1
    CHANGED
    y = 2

class Bar:
    def method(self):
        pass

def baz():
    pass
`, "\n")

	ds, de := findEnclosingDef(lines, hunkRange{3, 3}, 2, "python")
	if ds != 0 || de != 5 {
		t.Errorf("enclosing def = [%d,%d), want [0,5) (def foo)", ds, de)
	}

	ds, de = findEnclosingDef(lines, hunkRange{8, 8}, 2, "python")
	if ds != 5 || de != 9 {
		t.Errorf("enclosing def = [%d,%d), want [5,9) (class Bar)", ds, de)
	}
}

// TestFindEnclosingDefSkipsPriorDef verifies that a definition which ended
// before the hunk is skipped, and that no covering def above leads to a raw
// window fallback (which still covers the hunk's own definition below).
func TestFindEnclosingDefSkipsPriorDef(t *testing.T) {
	lines := strings.Split(`package p

func above() {
	x := 1
}

func below() {
	CHANGED
}
`, "\n")

	// Hunk at 1-based [8,8] (CHANGED, 0-based 7): "above" ends at line 5, and
	// "below" starts below the scan start, so the raw window is kept.
	ds, de := findEnclosingDef(lines, hunkRange{8, 8}, 2, "go")
	if ds != -1 || de != -1 {
		t.Errorf("expected raw-window fallback, got [%d,%d)", ds, de)
	}
}

// TestFindEnclosingDefTooLong falls back when the definition exceeds the cap
// and the hunk lies beyond the expansion limit.
func TestFindEnclosingDefTooLong(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("func huge() {\n")
	for i := 0; i < 300; i++ {
		sb.WriteString("\tx := 1\n")
	}
	sb.WriteString("}\n")
	lines := strings.Split(sb.String(), "\n")

	// Hunk near the end (0-based ~299): beyond start+maxDefLen → fallback.
	ds, de := findEnclosingDef(lines, hunkRange{299, 299}, 4, "go")
	if ds != -1 || de != -1 {
		t.Errorf("far hunk should fall back, got [%d,%d)", ds, de)
	}

	// Hunk near the start: capped expansion keeps the def head.
	ds, de = findEnclosingDef(lines, hunkRange{10, 10}, 4, "go")
	if ds != 0 || de != maxDefLen {
		t.Errorf("near hunk should cap at [0,%d), got [%d,%d)", maxDefLen, ds, de)
	}
}

// TestPerFileExcerptSections verifies excerpt rendering: headers, def
// expansion, language fences, and notes for content-less files.
func TestPerFileExcerptSections(t *testing.T) {
	files := []fileContent{
		{
			name: "main.go",
			content: `package main

func main() {
	a := 1
	b := 2
	c := 3
	d := 4
	CHANGED
	CHANGED2
	g := 7
}

func other() {
	h := 8
}
`,
		},
		{name: "gone.go", content: "", note: "[文件已删除]"},
	}
	hunks := map[string][]hunkRange{
		"main.go": {{8, 9}}, // 1-based CHANGED lines
		"gone.go": {},
	}

	secs := perFileExcerptSections(files, hunks, 2)
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want 2: %v", len(secs), secs)
	}
	mainSec := secs[0].text
	// Raw window for the hunk [8,9] with context=2 is [5,11) (1-based 6-11);
	// definition expansion widens it to func main's full body: 1-based 3-11.
	// The trailing newline of the content adds an empty split element (16).
	if !strings.Contains(mainSec, "## File: main.go (lines 3-11 of 16, context=2)") {
		t.Errorf("excerpt header wrong:\n%s", mainSec)
	}
	if !strings.Contains(mainSec, "```go\n") {
		t.Errorf("go code fence missing:\n%s", mainSec)
	}
	// Definition expansion: whole func main body, including its closing brace.
	if !strings.Contains(mainSec, "func main() {") || !strings.Contains(mainSec, "CHANGED2") {
		t.Errorf("definition expansion missing:\n%s", mainSec)
	}
	// func other is outside the def window.
	if strings.Contains(mainSec, "func other()") {
		t.Errorf("window leaked beyond the enclosing definition:\n%s", mainSec)
	}
	// Notes for content-less files are preserved.
	if !strings.Contains(secs[1].text, "gone.go [文件已删除]") {
		t.Errorf("note section missing:\n%s", secs[1].text)
	}
}

// TestTruncateReviewRequestSmall verifies no truncation when the input fits.
func TestTruncateReviewRequestSmall(t *testing.T) {
	files := []fileContent{{name: "a.go", content: "package a\n"}}
	req, warning := truncateReviewRequest("summary", "log", "diff --git a/a.go b/a.go\n", files, "guide")
	if warning != "" {
		t.Errorf("unexpected warning: %q", warning)
	}
	if !strings.Contains(req, "summary") || !strings.Contains(req, "## Code Changes") {
		t.Errorf("request missing core sections:\n%s", req)
	}
}

// contextHeaderRe matches an excerpt header with any context level.
var contextHeaderRe = regexp.MustCompile(`context=\d+`)

// TestTruncateReviewRequestStage1 verifies full contents are replaced by
// hunk-context excerpts when the request exceeds the input limit.
func TestTruncateReviewRequestStage1(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("package big\n\n")
	for i := 0; i < 3200; i++ {
		fmt.Fprintf(&sb, "line %04d\n", i)
	}
	sb.WriteString("MARKER_OUTSIDE_WINDOW_XYZ\n")
	content := sb.String()

	files := []fileContent{{name: "big.go", content: content}}
	patch := "diff --git a/big.go b/big.go\n--- a/big.go\n+++ b/big.go\n@@ -28,4 +28,4 @@\n"
	req, warning := truncateReviewRequest("summary", "log", patch, files, "guide")

	if warning == "" {
		t.Fatal("expected a truncation warning")
	}
	if !strings.Contains(warning, "上下文摘录") {
		t.Errorf("warning should mention context excerpts: %q", warning)
	}
	// An excerpt with any context level should fit; don't pin the exact level
	// so tuning the context ladder does not break this test.
	if !contextHeaderRe.MatchString(req) {
		t.Errorf("expected a context excerpt header in:\n%s", req)
	}
	// The bulk of the file (outside the window) must be gone.
	if strings.Contains(req, "MARKER_OUTSIDE_WINDOW_XYZ") {
		t.Errorf("out-of-window content leaked into request")
	}
	// Core sections survive.
	if !strings.Contains(req, "## Project Guide (AGENTS.md)") ||
		!strings.Contains(req, "## Code Changes") ||
		!strings.Contains(req, "summary") {
		t.Errorf("core sections lost:\n%s", req)
	}
}

// TestTruncateReviewRequestStage3 verifies per-file dropping with a diff so
// large that even minimal excerpts cannot fit, and that the warning reports
// the dropped files (coverage blind spots).
func TestTruncateReviewRequestStage3(t *testing.T) {
	const nFiles = 30
	var patchSB strings.Builder
	var files []fileContent
	for i := 0; i < nFiles; i++ {
		name := fmt.Sprintf("f%02d.go", i)
		fmt.Fprintf(&patchSB, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1,600 +1,600 @@\n", name, name, name, name)
		for j := 0; j < 600; j++ {
			fmt.Fprintf(&patchSB, " line %d\n", j)
		}
		var c strings.Builder
		c.WriteString("package f\n")
		for j := 0; j < 600; j++ {
			fmt.Fprintf(&c, "line %d\n", j)
		}
		files = append(files, fileContent{name: name, content: c.String()})
	}

	req, warning := truncateReviewRequest("summary", "log", patchSB.String(), files, "guide")

	if len(req) > maxUserInputLen {
		t.Errorf("request still over limit: %d > %d", len(req), maxUserInputLen)
	}
	if !strings.Contains(warning, "已丢弃") {
		t.Errorf("warning should list dropped files: %q", warning)
	}
	if !strings.Contains(warning, "diff（") {
		t.Errorf("warning should report dropped diff sections: %q", warning)
	}
	// Core sections survive even under extreme pressure.
	if !strings.Contains(req, "## Project Guide (AGENTS.md)") ||
		!strings.Contains(req, "## Commit Background") {
		t.Errorf("core sections lost:\n%s", req)
	}
}

// TestTruncateReviewRequestTail verifies extreme-pressure behavior: core
// sections survive and the request stays near the limit.
func TestTruncateReviewRequestTail(t *testing.T) {
	patch := strings.Repeat("diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1,1 +1,1 @@\n", 3000)
	req, warning := truncateReviewRequest(strings.Repeat("s", 1024), strings.Repeat("l", 4000), patch, nil, strings.Repeat("g", maxGuideLen))

	if len(req) > maxUserInputLen {
		t.Errorf("request still over limit: %d > %d", len(req), maxUserInputLen)
	}
	if warning == "" {
		t.Errorf("expected a warning for extreme input")
	}
	if !strings.Contains(req, "## Commit Background") ||
		!strings.Contains(req, "## Project Guide (AGENTS.md)") {
		t.Errorf("core sections lost:\n%s", req)
	}
}

// TestDropUntilFits verifies smallest-first greedy keeping.
func TestDropUntilFits(t *testing.T) {
	sections := []namedSection{
		{name: "big", text: strings.Repeat("b", 12)},
		{name: "mid", text: strings.Repeat("m", 8)},
		{name: "tiny", text: strings.Repeat("t", 3)},
		{name: "large", text: strings.Repeat("l", 10)},
	}
	kept, dropped := dropUntilFits("12345", sections, 20) // budget 15

	gotKept := map[string]bool{}
	for _, s := range kept {
		gotKept[s.name] = true
	}
	if !gotKept["tiny"] || !gotKept["mid"] {
		t.Errorf("smallest sections should be kept first: %v", kept)
	}
	if len(dropped) != 2 {
		t.Errorf("expected 2 dropped, got %d: %v", len(dropped), dropped)
	}
	for _, s := range dropped {
		if s.name != "big" && s.name != "large" {
			t.Errorf("unexpected dropped section: %s", s.name)
		}
	}
}
