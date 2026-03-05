package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
)

// TestRegisterToolAndGetAllTools 测试工具注册和获取
func TestRegisterToolAndGetAllTools(t *testing.T) {
	// 测试获取工具列表
	tools := GetAllTools()
	if len(tools) == 0 {
		t.Error("GetAllTools应该返回至少一个工具")
	}

	// 检查返回的Tool结构体
	for _, tool := range tools {
		if tool.Type == "" {
			t.Error("工具应该有Type字段")
		}
		if tool.Function.Name == "" {
			t.Error("工具函数应该有名称")
		}
		if tool.Function.Description == "" {
			t.Error("工具函数应该有描述")
		}
	}
}

// TestShuffleExported 测试导出的Shuffle函数
func TestShuffleExported(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string // 期望不同的字符串
	}{
		{
			name:     "短字符串",
			input:    "abc",
			expected: "abc", // 长度相同
		},
		{
			name:     "空字符串",
			input:    "",
			expected: "",
		},
		{
			name:     "长字符串",
			input:    "test-string-for-shuffle",
			expected: "test-string-for-shuffle", // 长度相同
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := Shuffle(tc.input)

			// 检查长度是否相同
			if len(result) != len(tc.expected) {
				t.Errorf("长度不匹配: 输入长度=%d, 输出长度=%d",
					len(tc.input), len(result))
			}

			// 检查是否只包含字母字符
			for _, r := range result {
				if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && r != '-' {
					t.Errorf("结果包含非字母字符: %c", r)
				}
			}
		})
	}
}

// TestGetToolDisplayName verifies the PascalCase conversion for tool names.
func TestGetToolDisplayName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"read_file", "ReadFile"},
		{"git_add", "GitAdd"},
		{"shell", "Shell"},
		{"web_reader", "WebReader"},
		{"write_file_with_line_range", "WriteFileWithLineRange"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := GetToolDisplayName(tt.input)
			if got != tt.expected {
				t.Errorf("GetToolDisplayName(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

// TestHandleToolCallRecordingErrorDoesNotShadowResult verifies that a
// RecordToolUsage failure does not discard the tool's real result.
func TestHandleToolCallRecordingErrorDoesNotShadowResult(t *testing.T) {
	const testToolName = "test_recording_shadow"
	const expectedResult = "tool-succeeded-with-data"

	RegisterTool(ToolDef{
		Name:        testToolName,
		Description: "test tool for recording shadow test",
		Parameters:  map[string]any{"type": "object", "properties": map[string]any{}},
		Category:    "test",
		Handler: func(ctx context.Context, args map[string]string) (string, error) {
			return expectedResult, nil
		},
	})
	defer delete(toolRegistry, testToolName)

	// Suppress error output during test.
	var errBuf bytes.Buffer
	oldErrWriter := outputErrorWriter
	SetErrorWriter(&errBuf)
	defer SetErrorWriter(oldErrWriter)

	argsJSON, _ := json.Marshal(map[string]string{})
	result, err := HandleToolCall(context.Background(), testToolName, argsJSON)

	if err != nil {
		t.Fatalf("HandleToolCall returned error when tool succeeded: %v", err)
	}
	if result != expectedResult {
		t.Errorf("HandleToolCall result = %q, want %q", result, expectedResult)
	}
}
