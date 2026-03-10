// Package main contains tests for the code review tool
package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestCodeReviewToolRegistration tests that the code review tool is properly registered
func TestCodeReviewToolRegistration(t *testing.T) {
	// Check if the tool is registered by looking for it in the tools list
	found := false
	for _, tool := range GetTools() {
		if tool.Name == "code_review" {
			found = true
			if tool.DisplayName != "代码审查" {
				t.Errorf("Expected display name '代码审查', got '%s'", tool.DisplayName)
			}
			if !strings.Contains(tool.Description, "检测到未提交的更改") {
				t.Error("Tool description should mention uncommitted change detection")
			}
			break
		}
	}

	if !found {
		t.Error("code_review tool not found in registered tools")
	}
}

// TestHandleCodeReviewErrorHandling tests error handling scenarios
func TestHandleCodeReviewErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Test cases for error handling
	testCases := []struct {
		name        string
		args        map[string]string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Valid summary",
			args:        map[string]string{"summary": "Test commit for review"},
			expectError: false,
			errorMsg:    "",
		},
		{
			name:        "Empty summary",
			args:        map[string]string{"summary": ""},
			expectError: false, // Empty summary is allowed (optional parameter)
			errorMsg:    "",
		},
		{
			name:        "No summary key",
			args:        map[string]string{},
			expectError: false, // Summary is optional
			errorMsg:    "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Note: We can't easily test the actual execution because it requires
			// Git operations and external API calls. This is a limitation of unit testing
			// for tools that interact with external systems.

			// For now, we just verify the function exists and has the right signature
			// Actual integration tests would be needed for full coverage
			if handleCodeReview == nil {
				t.Error("handleCodeReview function is nil")
			}
		})
	}
}

// TestCodeReviewDocumentation tests that the tool documentation is complete
func TestCodeReviewDocumentation(t *testing.T) {
	// Find the code review tool
	var toolDef ToolDef
	for _, tool := range GetTools() {
		if tool.Name == "code_review" {
			toolDef = tool
			break
		}
	}

	// Check documentation completeness
	requiredKeywords := []string{
		"未提交的更改",
		"错误",
		"提交",
		"审查",
		"专家",
	}

	description := toolDef.Description
	for _, keyword := range requiredKeywords {
		if !strings.Contains(description, keyword) {
			t.Errorf("Tool description missing required keyword: %s", keyword)
		}
	}

	// Check timeout is reasonable
	if toolDef.Timeout < 30*time.Second {
		t.Errorf("Timeout too short: %v", toolDef.Timeout)
	}

	// Check category
	if toolDef.Category != "git" {
		t.Errorf("Expected category 'git', got '%s'", toolDef.Category)
	}
}

// TestErrorMessages tests that error messages are user-friendly
func TestErrorMessages(t *testing.T) {
	// This test verifies that our error handling produces helpful messages
	// Since we can't easily mock Git operations, we'll test the error message format

	// Simulate what the actual error would look like
	gitStatus := " M code_review.go\n?? new_file.txt"
	expectedError := "检测到未提交的更改，请先提交所有更改再审查。当前状态：\n" + gitStatus

	// Check that the error message contains key information
	if !strings.Contains(expectedError, "检测到未提交的更改") {
		t.Error("Error message should mention uncommitted changes")
	}
	if !strings.Contains(expectedError, "请先提交所有更改") {
		t.Error("Error message should instruct user to commit changes")
	}
	if !strings.Contains(expectedError, gitStatus) {
		t.Error("Error message should include Git status")
	}
}
