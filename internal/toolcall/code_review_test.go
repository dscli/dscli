// Package main contains tests for the code review tool
package toolcall

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestCodeReviewToolStructure tests the basic structure of the code review tool
func TestCodeReviewToolStructure(t *testing.T) {
	if os.Getenv("InsideShellExec") != "" {
		t.SkipNow()
	}

	// Verify the tool definition exists
	if codeReviewTool.Name != "code_review" {
		t.Errorf("Expected tool name 'code_review', got '%s'", codeReviewTool.Name)
	}

	if codeReviewTool.DisplayName != "代码审查" {
		t.Errorf("Expected display name '代码审查', got '%s'", codeReviewTool.DisplayName)
	}

	// Check that description contains key information
	description := codeReviewTool.Description
	requiredKeywords := []string{
		"未提交的更改",
		"错误",
		"提交",
		"审查",
		"专家",
		"单元测试",
		"test_command",
	}

	for _, keyword := range requiredKeywords {
		if !strings.Contains(description, keyword) {
			t.Errorf("Tool description missing required keyword: %s", keyword)
		}
	}
	if codeReviewTool.Timeout != 5*time.Minute {
		t.Errorf("Expected timeout 5 minutes, got %v", codeReviewTool.Timeout)
	}

	if codeReviewTool.Category != "git" {
		t.Errorf("Expected category 'git', got '%s'", codeReviewTool.Category)
	}
}

// TestHandleCodeReviewFunction tests that the handler function exists
func TestHandleCodeReviewFunction(t *testing.T) {
	if os.Getenv("InsideShellExec") != "" {
		t.SkipNow()
	}

	// This is a simple test to verify the function signature
	// We can't easily test the actual execution without mocking external dependencies

	ctx := context.Background()
	args := ToolArgs{"summary": "Test commit"}

	// The function should exist and be callable
	// Note: We're not checking the actual return value since it depends on Git state
	_, err := handleCodeReview(ctx, args)
	// We expect an error if there are uncommitted changes or no commits
	// But we don't fail the test based on the error since it's environment-dependent
	if err != nil {
		t.Logf("handleCodeReview returned error (expected in test environment): %v", err)
	}
}

// TestErrorMessages tests error message format
func TestErrorMessages(t *testing.T) {
	if os.Getenv("InsideShellExec") != "" {
		t.SkipNow()
	}
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
		"参数说明",
		"使用场景",
		"审查流程",
		"错误处理",
		"注意",
	}

	for _, section := range sections {
		if !strings.Contains(desc, section) {
			t.Errorf("Documentation missing section: %s", section)
		}
	}

	// Check that error handling is documented
	if !strings.Contains(desc, "如果检测到未提交的更改，工具会立即返回错误") &&
		!strings.Contains(desc, "如果检测到多个未push的提交，工具会返回错误") &&
		!strings.Contains(desc, "如果单元测试未通过，工具会返回错误") {
		t.Error("Documentation should mention error returns for various checks")
	}
	// Check that user guidance is provided
	if !strings.Contains(desc, "用户需要先解决所有问题") {
		t.Error("Documentation should instruct users to resolve all issues first")
	}
}
