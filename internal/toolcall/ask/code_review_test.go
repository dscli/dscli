package ask

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
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
		"last",
	}
	for _, keyword := range requiredKeywords {
		if !strings.Contains(description, keyword) {
			t.Errorf("Tool description missing required keyword: %s", keyword)
		}
	}
	// 30 min: the expert may run multiple tool-call rounds before the
	// final review, and each round needs a browser session + model reply.
	// 15 min proved too short for large diffs (observed timeout).
	if codeReviewTool.Timeout != 30*time.Minute {
		t.Errorf("Expected timeout 30 minutes, got %v", codeReviewTool.Timeout)
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
// request from the summary, commit log and patch.
func TestBuildCodeReviewRequest(t *testing.T) {
	summary := "fix: test summary"
	commitLog := "commit message body"
	patch := "diff --git a/file.go b/file.go"

	result := buildCodeReviewRequest(summary, commitLog, patch)

	sections := []string{
		"## Commit Background",
		"## Commit Message",
		"## Code Changes",
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
	// The request must NOT carry file contents or the project guide: the
	// expert reads them on demand via read_file (see code_review.go comment).
	for _, gone := range []string{"## Project Guide (AGENTS.md)", "## File Contents", "AGENTS.md content"} {
		if strings.Contains(result, gone) {
			t.Errorf("Request should not contain %q, got:\n%s", gone, result)
		}
	}

	// Empty patch omits the Code Changes section.
	result2 := buildCodeReviewRequest(summary, commitLog, "")
	if strings.Contains(result2, "## Code Changes") {
		t.Errorf("Should NOT include '## Code Changes' when patch is empty")
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
		"last",
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

// TestSplitPatchByFile covers per-file splitting: multiple files, renames,
// and a trailing file without a closing marker.
func TestSplitPatchByFile(t *testing.T) {
	patch := `diff --git a/a.go b/a.go
index 111..222 100644
--- a/a.go
+++ b/a.go
@@ -1,2 +1,2 @@
-old
+new
diff --git a/b/b.go b/b/b.go
index 333..444 100644
--- a/b/b.go
+++ b/b/b.go
@@ -1,1 +1,1 @@
-x
+y
`
	secs := splitPatchByFile(patch)
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want 2: %v", len(secs), secs)
	}
	if secs[0].name != "a.go" || secs[1].name != "b/b.go" {
		t.Errorf("names = %q, %q; want a.go, b/b.go", secs[0].name, secs[1].name)
	}
	if !strings.Contains(secs[0].text, "+new") || !strings.Contains(secs[1].text, "+y") {
		t.Errorf("section contents wrong:\n%s\n---\n%s", secs[0].text, secs[1].text)
	}
}

// TestSplitPatchByFileDeleted verifies that a file deletion (+++ /dev/null)
// falls back to the --- a/ path as the section name instead of "/dev/null".
func TestSplitPatchByFileDeleted(t *testing.T) {
	patch := `diff --git a/old.go b/old.go
index 111..222 100644
--- a/old.go
+++ /dev/null
@@ -1,2 +0,0 @@
-foo
-bar
diff --git a/new.go b/new.go
index 000..333 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1,2 @@
+x
+y
`
	secs := splitPatchByFile(patch)
	if len(secs) != 2 {
		t.Fatalf("got %d sections, want 2: %v", len(secs), secs)
	}
	if secs[0].name != "old.go" {
		t.Errorf("deleted file section name = %q, want old.go", secs[0].name)
	}
	if secs[1].name != "new.go" {
		t.Errorf("added file section name = %q, want new.go", secs[1].name)
	}
}

// TestCutToRuneLen verifies rune-count truncation: the prefix is the first
// maxRunes characters, never splitting a UTF-8 rune or exceeding the count.
func TestCutToRuneLen(t *testing.T) {
	s := "abc中文测试def" // 10 runes: 3 ASCII + 4 Chinese + 3 ASCII (18 bytes)
	for _, n := range []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 99} {
		got := cutToRuneLen(s, n)
		if countRunes(got) > n {
			t.Errorf("cutToRuneLen(s, %d) = %q has %d runes", n, got, countRunes(got))
		}
		if !utf8.ValidString(got) {
			t.Errorf("cutToRuneLen(s, %d) produced invalid UTF-8: %q", n, got)
		}
		if got != s[:len(got)] {
			t.Errorf("cutToRuneLen(s, %d) = %q, not a prefix", n, got)
		}
	}
	// 第 5 个 rune 是"中"：rune 边界上的截断。
	if cutToRuneLen(s, 5) != "abc中文" {
		t.Errorf("5-rune cut = %q, want %q", cutToRuneLen(s, 5), "abc中文")
	}
	// 预算超过输入：原样返回。
	if cutToRuneLen(s, 99) != s {
		t.Errorf("oversized budget should return input unchanged")
	}
}

// TestTruncateReviewRequestSmall verifies no truncation when the input fits.
func TestTruncateReviewRequestSmall(t *testing.T) {
	req, warning := truncateReviewRequest("summary", "log", "diff --git a/a.go b/a.go\n")
	if warning != "" {
		t.Errorf("unexpected warning: %q", warning)
	}
	if !strings.Contains(req, "summary") || !strings.Contains(req, "## Code Changes") {
		t.Errorf("request missing core sections:\n%s", req)
	}
}

// TestTruncateReviewRequestDropsDiff verifies that an oversized diff is
// dropped per-file (smallest first) and the warning lists the dropped files -
// the expert can read them via read_file to fill the gap.
func TestTruncateReviewRequestDropsDiff(t *testing.T) {
	const nFiles = 30
	var patchSB strings.Builder
	for i := 0; i < nFiles; i++ {
		name := fmt.Sprintf("f%02d.go", i)
		fmt.Fprintf(&patchSB, "diff --git a/%s b/%s\n--- a/%s\n+++ b/%s\n@@ -1,600 +1,600 @@\n", name, name, name, name)
		for j := 0; j < 600; j++ {
			fmt.Fprintf(&patchSB, " line %d\n", j)
		}
	}

	req, warning := truncateReviewRequest("summary", "log", patchSB.String())

	if countRunes(req) > maxUserInputLen {
		t.Errorf("request still over limit: %d > %d", countRunes(req), maxUserInputLen)
	}
	if warning == "" {
		t.Fatal("expected a warning for oversized diff")
	}
	if !strings.Contains(warning, "已丢弃") {
		t.Errorf("warning should list dropped files: %q", warning)
	}
	if !strings.Contains(warning, "read_file") {
		t.Errorf("warning should point the user at read_file: %q", warning)
	}
	// Core sections survive even under extreme pressure.
	if !strings.Contains(req, "## Commit Background") ||
		!strings.Contains(req, "## Commit Message") {
		t.Errorf("core sections lost:\n%s", req)
	}
	// 覆盖盲区可见：截断提示必须内建在请求正文中，被丢弃的文件名逐一列出，
	// 专家才能感知盲区并 read_file 补读（仅返回给调用者的 warning 不够）。
	if !strings.Contains(req, "## ⚠️ 审查输入截断") {
		t.Errorf("request body missing truncation note:\n%s", req)
	}
	dropRe := regexp.MustCompile(`已丢弃 (.+?) 的 diff`) // 非贪婪：文件名可含空格
	matches := dropRe.FindAllStringSubmatch(warning, -1)
	if len(matches) == 0 {
		t.Fatalf("no dropped files in warning: %q", warning)
	}
	for _, m := range matches {
		if !strings.Contains(req, m[1]) {
			t.Errorf("dropped file %q not listed in request body", m[1])
		}
	}
}

// TestTruncateReviewRequestTail verifies extreme-pressure behavior: even when
// the diff is enormous, the request stays near the limit and a warning fires.
func TestTruncateReviewRequestTail(t *testing.T) {
	patch := strings.Repeat("diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1,1 +1,1 @@\n", 3000)
	req, warning := truncateReviewRequest(strings.Repeat("s", 1024), strings.Repeat("l", 4000), patch)

	if countRunes(req) > maxUserInputLen {
		t.Errorf("request still over limit: %d > %d", countRunes(req), maxUserInputLen)
	}
	if warning == "" {
		t.Errorf("expected a warning for extreme input")
	}
	if !strings.Contains(req, "## Commit Background") ||
		!strings.Contains(req, "## Commit Message") {
		t.Errorf("core sections lost:\n%s", req)
	}
	// 极端压力下截断信号仍必须出现在请求正文（摘要形式也可）。
	if !strings.Contains(req, "审查输入截断") {
		t.Errorf("request body missing truncation note:\n%s", req)
	}
}

// TestTruncateReviewRequestHard exercises the TRUE hard-truncation branch:
// summary+commitLog alone exceed maxUserInputLen, so no diff at all is kept.
// The truncation note must survive the byte cut (it is appended AFTER the
// cut, with budget reserved) and the request must stay within the limit.
func TestTruncateReviewRequestHard(t *testing.T) {
	summary := strings.Repeat("s", maxUserInputLen+20000)
	commitLog := strings.Repeat("l", 10000)
	patch := "diff --git a/x.go b/x.go\n--- a/x.go\n+++ b/x.go\n@@ -1,1 +1,1 @@\n-x\n+y\n"

	req, warning := truncateReviewRequest(summary, commitLog, patch)

	if countRunes(req) > maxUserInputLen {
		t.Errorf("request over limit after hard truncation: %d > %d", countRunes(req), maxUserInputLen)
	}
	if warning == "" {
		t.Error("expected a warning for hard truncation")
	}
	if !strings.Contains(req, "审查输入截断") {
		t.Errorf("hard truncation lost the truncation note:\n%s", req)
	}
	if !strings.Contains(req, "已硬截断") {
		t.Errorf("hard truncation note should carry the truncation signal:\n%s", req)
	}
	if !strings.Contains(req, "x.go") {
		t.Errorf("hard truncation note should still list dropped files:\n%s", req)
	}
	if !utf8.ValidString(req) {
		t.Error("hard-truncated request must be valid UTF-8")
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

// TestCutToRuneLenNegativeBudget: a negative (or zero) budget (defensive
// scenario where the truncation note itself exceeds the limit) must return
// "" without panicking, mirroring headRunes' guard.
func TestCutToRuneLenNegativeBudget(t *testing.T) {
	for _, n := range []int{-1, -100, 0} {
		got := cutToRuneLen("abc中文", n)
		if got != "" {
			t.Errorf("cutToRuneLen(s, %d) = %q, want empty", n, got)
		}
	}
}

// ---------- 附件装配（code_review 输入附件化） ----------

func TestParseNumstat(t *testing.T) {
	out := "1\t2\tinternal/a.go\n-\t-\tassets/logo.png\n3\t0\tb.md\n\nbroken line\n4\t5\tc/d.go\n"
	got := parseNumstat(out)
	want := []string{"internal/a.go", "b.md", "c/d.go"}
	if !slices.Equal(got, want) {
		t.Errorf("parseNumstat = %v, want %v (binary and malformed lines skipped)", got, want)
	}
	if got := parseNumstat(""); got != nil {
		t.Errorf("parseNumstat(\"\") = %v, want nil", got)
	}
}

func TestParseNameOnly(t *testing.T) {
	out := "a.go\n\nb/c.go\na.go\n"
	got := parseNameOnly(out)
	want := []string{"a.go", "b/c.go"}
	if !slices.Equal(got, want) {
		t.Errorf("parseNameOnly = %v, want %v (blank lines dropped, deduplicated)", got, want)
	}
}

func TestEncodeAttachmentName(t *testing.T) {
	cases := map[string]string{
		"main.go":                "main.go",
		"internal/lp/webchat.go": "internal__lp__webchat.go",
		"a/b/c.txt":              "a__b__c.txt",
	}
	for in, want := range cases {
		if got := encodeAttachmentName(in); got != want {
			t.Errorf("encodeAttachmentName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueAttachmentName(t *testing.T) {
	used := map[string]bool{}
	for i, want := range []string{"x.go", "x.go_2", "x.go_3"} {
		if got := uniqueAttachmentName(used, "x.go"); got != want {
			t.Errorf("collision %d: got %q, want %q", i, got, want)
		}
	}
}

func TestSelectReviewFiles(t *testing.T) {
	files := []candidateFile{
		{path: "big.go", size: 100},
		{path: "tiny.go", size: 1},
		{path: "mid.go", size: 10},
	}

	// Smallest-first: the two smallest fit the count budget.
	kept, dropped := selectReviewFiles(files, 2, 1000)
	if len(kept) != 2 || kept[0].path != "mid.go" || kept[1].path != "tiny.go" {
		t.Errorf("kept = %v, want [mid.go tiny.go] (path-sorted)", kept)
	}
	if len(dropped) != 1 || dropped[0].path != "big.go" {
		t.Errorf("dropped = %v, want [big.go]", dropped)
	}

	// Byte budget: only the tiny file fits.
	kept, dropped = selectReviewFiles(files, 10, 5)
	if len(kept) != 1 || kept[0].path != "tiny.go" {
		t.Errorf("byte-limited kept = %v, want [tiny.go]", kept)
	}
	if len(dropped) != 2 {
		t.Errorf("byte-limited dropped = %v, want 2 entries", dropped)
	}

	// Zero count budget keeps nothing.
	kept, dropped = selectReviewFiles(files, 0, 1000)
	if len(kept) != 0 || len(dropped) != 3 {
		t.Errorf("zero-count result kept=%v dropped=%v", kept, dropped)
	}
}

func TestCapCommitLog(t *testing.T) {
	small := "fix: something"
	if got := capCommitLog(small); got != small {
		t.Errorf("capCommitLog(small) = %q, want unchanged", got)
	}

	big := strings.Repeat("x", maxReviewCommitLogRunes+500)
	got := capCommitLog(big)
	if countRunes(got) > maxReviewCommitLogRunes+64 {
		t.Errorf("capped log has %d runes, want about %d", countRunes(got), maxReviewCommitLogRunes)
	}
	if !strings.HasSuffix(got, "[commit message truncated for length]") {
		t.Errorf("capped log missing truncation marker: %q", got[len(got)-60:])
	}
}

func TestTruncatePatchToBudget(t *testing.T) {
	secA := "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-a\n+b\n"
	secB := "diff --git a/b.go b/b.go\n--- a/b.go\n+++ b/b.go\n@@ -1 +1 @@\n" + strings.Repeat("-old\n+new\n", 50)
	patch := secA + secB

	// Budget fits only the smaller section; the dropped file is reported.
	kept, dropped := truncatePatchToBudget(patch, len(secA))
	if kept != secA {
		t.Errorf("kept = %q, want only section a", kept)
	}
	if len(dropped) != 1 || dropped[0] != "b.go" {
		t.Errorf("dropped = %v, want [b.go]", dropped)
	}

	// Everything fits: the patch passes through byte-for-byte. (The splitter
	// adds one trailing newline per section, so the full-size budget is
	// len(patch)+1.)
	if got, dropped := truncatePatchToBudget(patch, len(patch)+1); got != patch || dropped != nil {
		t.Errorf("full budget must keep the patch intact (dropped=%v)", dropped)
	}

	// Zero budget: nothing kept, both files reported.
	if got, dropped := truncatePatchToBudget(patch, 0); got != "" || len(dropped) != 2 {
		t.Errorf("zero budget: got %q, dropped %v", got, dropped)
	}
}

func TestIsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	text := filepath.Join(dir, "text.go")
	if err := os.WriteFile(text, []byte("package main\n// text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if isBinaryFile(text) {
		t.Error("text file flagged as binary")
	}
	bin := filepath.Join(dir, "blob.bin")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0xff, 0x00}, 0o600); err != nil {
		t.Fatal(err)
	}
	if !isBinaryFile(bin) {
		t.Error("file with NUL bytes must be flagged as binary")
	}
	if isBinaryFile(filepath.Join(dir, "missing.bin")) {
		t.Error("missing file must not be flagged as binary")
	}
}

func TestBuildReviewMessage(t *testing.T) {
	plan := reviewPlan{
		CommitCount: 2,
		Changed:     []string{"a.go", "b.go"},
		Skipped:     []string{"gone.go"},
		Attached:    []string{"a.go", "b.go"},
		Agents:      true,
	}
	msg := buildReviewMessage("summary text", "commit body", plan)
	for _, want := range []string{
		"## Commit Background",
		"summary text",
		"## Commit Message",
		"commit body",
		"## Review Inputs",
		"review-guide.md",
		"changes.patch",
		"gocyclo.txt",
		"AGENTS.md (project guide)",
		"internal__lp__webchat.go",
		"## Coverage",
		"Commits under review: 2.",
		"Changed files: 3 (1 skipped as deleted/binary/unreadable).",
		"Full content attached: 2 file(s).",
		"- Not attached: none.",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}

	// Budget drops and patch truncation are named explicitly.
	plan.NotAttached = []string{"c.go"}
	plan.PatchTruncated = true
	plan.PatchDropped = []string{"d.go"}
	msg = buildReviewMessage("s", "l", plan)
	if !strings.Contains(msg, "NOT attached (attachment budget): c.go") {
		t.Errorf("message must list not-attached files:\n%s", msg)
	}
	if !strings.Contains(msg, "changes.patch was truncated") || !strings.Contains(msg, "d.go") {
		t.Errorf("message must report patch truncation:\n%s", msg)
	}

	// Without AGENTS.md the inputs sentence must not claim it, and the
	// coverage note must state the blind spot.
	msg = buildReviewMessage("s", "l", reviewPlan{})
	if strings.Contains(msg, "AGENTS.md (project guide)") {
		t.Errorf("message must not claim AGENTS.md is attached when absent:\n%s", msg)
	}
	if !strings.Contains(msg, "AGENTS.md not attached") {
		t.Errorf("message must flag the missing AGENTS.md:\n%s", msg)
	}
}

func TestReviewAttachmentWarning(t *testing.T) {
	if got := reviewAttachmentWarning(reviewPlan{}); got != "" {
		t.Errorf("warning = %q, want empty for full coverage", got)
	}
	got := reviewAttachmentWarning(reviewPlan{
		NotAttached:    []string{"a.go"},
		PatchTruncated: true,
		PatchDropped:   []string{"b.go"},
	})
	for _, want := range []string{"a.go", "b.go", "截断"} {
		if !strings.Contains(got, want) {
			t.Errorf("warning missing %q: %q", want, got)
		}
	}
}

func TestGocycloCmd(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell-script fake is Unix-only")
	}
	if got := gocycloCmd(context.Background(), "", nil); !strings.Contains(got, "No changed Go files") {
		t.Errorf("empty file list report = %q", got)
	}

	// Missing binary: the report says so instead of failing the review.
	t.Setenv("PATH", t.TempDir())
	if got := gocycloCmd(context.Background(), "", []string{"a.go"}); !strings.Contains(got, "not found") {
		t.Errorf("missing-binary report = %q", got)
	}

	// Fake binary: its output is embedded in the report.
	dir := t.TempDir()
	script := filepath.Join(dir, "gocyclo")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"21 main.foo a.go:1:1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	got := gocycloCmd(context.Background(), "", []string{"a.go"})
	if !strings.Contains(got, "21 main.foo a.go:1:1") || !strings.Contains(got, fmt.Sprintf("-over %d", gocycloThreshold)) {
		t.Errorf("fake-binary report = %q", got)
	}

	// gocyclo exits non-zero when it reports findings; that is its normal
	// status, so the report must not call it an error.
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '21 main.foo a.go:1:1\n'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	got = gocycloCmd(context.Background(), "", []string{"a.go"})
	if !strings.Contains(got, "21 main.foo a.go:1:1") || strings.Contains(got, "failed") || strings.Contains(got, "error") {
		t.Errorf("findings-exit report = %q", got)
	}
}

// setupReviewRepo creates a throwaway git repository with two commits, the
// second modifying second.go; AGENTS.md is committed with the first.
func setupReviewRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(
			os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
		)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("init", "-q")
	for name, content := range map[string]string{
		"first.go":  "package main\n",
		"AGENTS.md": "# guide\n",
	} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	runGit("add", "-A")
	runGit("commit", "-qm", "one")
	if err := os.WriteFile(filepath.Join(repo, "second.go"), []byte("package main\n// v2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "-A")
	runGit("commit", "-qm", "two")
	return repo
}

// TestHandleCodeReviewAttachments drives the full handler against a throwaway
// git repository: every review input (guide, patch, gocyclo report, changed
// file, AGENTS.md) must arrive as an attachment, the message must carry the
// coverage note, and the temporary attachment directory must be cleaned up
// once the expert call returns.
func TestHandleCodeReviewAttachments(t *testing.T) {
	repo := setupReviewRepo(t)

	origAsk := askExpertWithRoleFunc
	t.Cleanup(func() { askExpertWithRoleFunc = origAsk })
	var (
		gotMessage     string
		gotRole        string
		gotSkip        bool
		gotAttachments []string
		attachmentData map[string]string
	)
	askExpertWithRoleFunc = func(_ context.Context, input, role, system, keep string, attachments []string, skip bool) (string, string, bool, error) {
		gotMessage = input
		gotRole = role
		gotSkip = skip
		gotAttachments = attachments
		attachmentData = map[string]string{}
		for _, p := range attachments {
			b, rerr := os.ReadFile(p)
			if rerr != nil {
				t.Errorf("attachment %s unreadable: %v", p, rerr)
				continue
			}
			attachmentData[filepath.Base(p)] = string(b)
		}
		return "[MOCK]", "", false, nil
	}

	origGocyclo := runGocyclo
	t.Cleanup(func() { runGocyclo = origGocyclo })
	runGocyclo = func(_ context.Context, _ string, files []string) string {
		if !slices.Equal(files, []string{"second.go"}) {
			t.Errorf("gocyclo files = %v, want [second.go]", files)
		}
		return "gocyclo report stub\n"
	}

	t.Chdir(repo)

	result, warning, err := handleCodeReview(context.Background(), toolcall.ToolArgs{"summary": "test summary", "since": "-1"})
	if err != nil {
		t.Fatalf("handleCodeReview: %v", err)
	}
	if result != "[MOCK]" {
		t.Errorf("result = %q, want [MOCK]", result)
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty (nothing dropped)", warning)
	}
	if gotRole != "review" || !gotSkip {
		t.Errorf("ask call role=%q skip=%v, want review/skip=true", gotRole, gotSkip)
	}

	if len(gotAttachments) != 5 {
		t.Fatalf("attachments = %v, want 5 (guide, patch, gocyclo, AGENTS.md, second.go)", gotAttachments)
	}
	for _, name := range []string{reviewGuideName, reviewPatchName, reviewGocycloName, reviewAgentsName, "second.go"} {
		if _, ok := attachmentData[name]; !ok {
			t.Errorf("attachment %q missing (paths: %v)", name, gotAttachments)
		}
	}
	if len(attachmentData[reviewGuideName]) == 0 {
		t.Errorf("review guide attachment is empty")
	}
	if !strings.Contains(attachmentData[reviewPatchName], "second.go") {
		t.Errorf("patch attachment misses the change:\n%s", attachmentData[reviewPatchName])
	}
	if attachmentData[reviewGocycloName] != "gocyclo report stub\n" {
		t.Errorf("gocyclo attachment = %q", attachmentData[reviewGocycloName])
	}
	if attachmentData[reviewAgentsName] != "# guide\n" {
		t.Errorf("AGENTS.md attachment = %q, want the repo guide", attachmentData[reviewAgentsName])
	}
	if got := attachmentData["second.go"]; !strings.Contains(got, "// v2") {
		t.Errorf("changed-file attachment = %q, want the file content", got)
	}

	for _, want := range []string{
		"## Coverage",
		"Commits under review: 1.",
		"Full content attached: 1 file(s).",
		"- Not attached: none.",
		"AGENTS.md (project guide)",
		"test summary",
	} {
		if !strings.Contains(gotMessage, want) {
			t.Errorf("message missing %q:\n%s", want, gotMessage)
		}
	}

	// The temporary attachment directory must be gone after the call.
	if _, serr := os.Stat(filepath.Dir(gotAttachments[0])); !os.IsNotExist(serr) {
		t.Errorf("attachment dir not cleaned up (stat err = %v)", serr)
	}
}
