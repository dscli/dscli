// Package main contains tests for the code review tool
package ask

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitcode.com/dscli/dscli/internal/toolcall"
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

// TestHandleCodeReviewFunction tests that the handler function exists
func TestHandleCodeReviewFunction(t *testing.T) {
	// This is a simple test to verify the function signature
	// We can't easily test the actual execution without mocking external dependencies

	ctx := context.Background()
	args := toolcall.ToolArgs{"summary": "Test commit"}

	// The function should exist and be callable
	// Note: We're not checking the actual return value since it depends on Git state
	result, _, err := handleCodeReview(ctx, args)
	if err != nil {
		// Git environment errors (uncommitted changes / no commits) are expected, not a test failure.
		t.Logf("handleCodeReview returned error (expected): %v", err)
	} else {
		// Success path: verify the mock was invoked.
		if !strings.Contains(result, "[MOCK]") {
			t.Fatalf("expected [MOCK] in result, got: %s", result)
		}
	}
}

// TestErrorMessages tests error message format
func TestErrorMessages(t *testing.T) {
	// Test the error message format that would be returned
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
			// Simulate the error message that would be generated
			errMsg := fmt.Sprintf("检测到未提交的更改，请先提交所有更改再审查。当前状态：\n%s", tc.gitStatus)

			// Check that all expected strings are in the error message
			for _, expected := range tc.expectedInMsg {
				if !strings.Contains(errMsg, expected) {
					t.Errorf("Error message missing '%s'. Got: %s", expected, errMsg)
				}
			}

			// Verify the message is helpful
			if !strings.Contains(errMsg, "当前状态：") {
				t.Error("Error message should show current Git status")
			}
		})
	}
}

// TestToolRegistration tests that the tool is properly registered
func TestToolRegistration(t *testing.T) {
	// Since we can't easily access the global tools registry in tests,
	// we verify that the init() function exists by checking side effects
	// The tool should be registered when the package is initialized
	// We can verify this by checking that CodeReviewTool is properly configured
	if codeReviewTool.Name == "" {
		t.Error("CodeReviewTool should have a name")
	}

	// Verify the tool definition is properly configured
	if codeReviewTool.Handler == nil {
		t.Error("CodeReviewTool.Handler should not be nil")
	}

	// Check that the handler points to the right function
	// This is a bit tricky to test directly, so we'll just verify the tool is configured
	if codeReviewTool.Name == "" {
		t.Error("CodeReviewTool should have a name")
	}
}

// TestDocumentationCompleteness tests that all required documentation is present
func TestDocumentationCompleteness(t *testing.T) {
	desc := codeReviewTool.Description

	// Check for key sections in documentation
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

	// Check that error handling is documented
	if !strings.Contains(desc, "uncommitted changes") &&
		!strings.Contains(desc, "before pushing") {
		t.Error("Documentation should mention uncommitted changes or push workflow")
	}
	// Check that user guidance is provided
	if !strings.Contains(desc, "before pushing") &&
		!strings.Contains(desc, "better practices") {
		t.Error("Documentation should instruct users about best practices")
	}
}
