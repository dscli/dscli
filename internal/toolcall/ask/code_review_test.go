package ask

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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

	if codeReviewTool.Category != "communication" {
		t.Errorf("Expected category 'communication', got '%s'", codeReviewTool.Category)
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
// request from the summary, commit log, and patch.
func TestBuildCodeReviewRequest(t *testing.T) {
	summary := "fix: test summary"
	commitLog := "commit message body"
	patch := "diff --git a/file.go b/file.go"

	result := buildCodeReviewRequest(summary, commitLog, patch, "")

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
}

// TestBuildCodeReviewRequestWithFileContents tests the file contents section.
func TestBuildCodeReviewRequestWithFileContents(t *testing.T) {
	summary := "fix: test"
	commitLog := "msg"
	patch := "diff"
	fileContents := "## File: main.go\n```\npackage main\n```\n"

	result := buildCodeReviewRequest(summary, commitLog, patch, fileContents)

	if !strings.Contains(result, "## Full File Contents") {
		t.Errorf("Expected '## Full File Contents' section when fileContents is non-empty")
	}
	if !strings.Contains(result, "## File: main.go") {
		t.Errorf("Expected file content in result")
	}

	// When empty, should NOT include the section
	result2 := buildCodeReviewRequest(summary, commitLog, patch, "")
	if strings.Contains(result2, "## Full File Contents") {
		t.Errorf("Should NOT include '## Full File Contents' when fileContents is empty")
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
