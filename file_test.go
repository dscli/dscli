package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleReadFileWithLineRange(t *testing.T) {
	// 创建临时测试文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// 写入测试内容（10行）
	var content strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&content, "Line %d\n", i)
	}
	if err := os.WriteFile(testFile, []byte(content.String()), 0o644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// 保存原始 ProjectRoot
	originalRoot := ProjectRoot
	ProjectRoot = tmpDir
	defer func() { ProjectRoot = originalRoot }()

	tests := []struct {
		name      string
		args      map[string]string
		wantError bool
		checkFunc func(string) bool
	}{
		{
			name: "读取完整文件",
			args: map[string]string{"path": "test.txt"},
			checkFunc: func(result string) bool {
				return strings.Contains(result, "完整文件") &&
					strings.Contains(result, "Line 1") &&
					strings.Contains(result, "Line 10")
			},
		},
		{
			name: "读取指定行范围",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "3",
				"end_line":   "7",
			},
			checkFunc: func(result string) bool {
				return strings.Contains(result, "第 3-7 行") &&
					strings.Contains(result, "Line 3") &&
					strings.Contains(result, "Line 7") &&
					!strings.Contains(result, "   1: Line 1") && // 精确匹配行号
					!strings.Contains(result, "  10: Line 10") // 精确匹配行号
			},
		},
		{
			name: "从某行到文件末尾",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "8",
			},
			checkFunc: func(result string) bool {
				return strings.Contains(result, "第 8 行到文件末尾") &&
					strings.Contains(result, "   8: Line 8") && // 精确匹配行号
					strings.Contains(result, "  10: Line 10") && // 精确匹配行号
					!strings.Contains(result, "   1: Line 1") // 精确匹配行号
			},
		},
		{
			name: "无效起始行号",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "abc",
			},
			wantError: true,
		},
		{
			name: "结束行号小于起始行号",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "5",
				"end_line":   "3",
			},
			wantError: true,
		},
		{
			name:      "文件不存在",
			args:      map[string]string{"path": "nonexistent.txt"},
			wantError: true,
		},
		{
			name:      "缺少路径参数",
			args:      map[string]string{},
			wantError: true,
		},
		{
			name:      "空路径参数",
			args:      map[string]string{"path": ""},
			wantError: true,
		},
		{
			name: "行号超出范围",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "20",
				"end_line":   "30",
			},
			checkFunc: func(result string) bool {
				return strings.Contains(result, "指定行范围内无内容")
			},
		},
		{
			name: "单行读取",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "5",
				"end_line":   "5",
			},
			checkFunc: func(result string) bool {
				return strings.Contains(result, "第 5-5 行") &&
					strings.Contains(result, "   5: Line 5") && // 精确匹配行号
					!strings.Contains(result, "   4: Line 4") && // 精确匹配行号
					!strings.Contains(result, "   6: Line 6") // 精确匹配行号
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := handleReadFileWithLineRange(ctx, tt.args)

			if tt.wantError {
				if err == nil {
					t.Errorf("handleReadFileWithLineRange() expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("handleReadFileWithLineRange() unexpected error: %v", err)
				return
			}

			if tt.checkFunc != nil && !tt.checkFunc(result) {
				t.Errorf("handleReadFileWithLineRange() result validation failed")
				t.Logf("Result:\n%s", result)
			}

			// 验证结果包含必要的统计信息
			if !strings.Contains(result, "文件信息:") {
				t.Errorf("Result missing file information")
			}
			if !strings.Contains(result, "执行统计:") {
				t.Errorf("Result missing execution statistics")
			}
		})
	}
}

func TestHandleReadFileWithLineRange_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	// 测试空文件
	emptyFile := filepath.Join(tmpDir, "empty.txt")
	if err := os.WriteFile(emptyFile, []byte(""), 0o644); err != nil {
		t.Fatalf("Failed to create empty file: %v", err)
	}

	// 测试只有一行的文件
	singleLineFile := filepath.Join(tmpDir, "single.txt")
	if err := os.WriteFile(singleLineFile, []byte("Single line"), 0o644); err != nil {
		t.Fatalf("Failed to create single line file: %v", err)
	}

	// 测试大文件（模拟）
	bigFile := filepath.Join(tmpDir, "big.txt")
	var bigContent strings.Builder
	for i := 1; i <= 1000; i++ {
		fmt.Fprintf(&bigContent, "Line %04d\n", i)
	}
	if err := os.WriteFile(bigFile, []byte(bigContent.String()), 0o644); err != nil {
		t.Fatalf("Failed to create big file: %v", err)
	}

	originalRoot := ProjectRoot
	ProjectRoot = tmpDir
	defer func() { ProjectRoot = originalRoot }()

	tests := []struct {
		name string
		file string
		args map[string]string
	}{
		{
			name: "空文件读取",
			file: "empty.txt",
			args: map[string]string{"path": "empty.txt"},
		},
		{
			name: "单行文件完整读取",
			file: "single.txt",
			args: map[string]string{"path": "single.txt"},
		},
		{
			name: "大文件部分读取",
			file: "big.txt",
			args: map[string]string{
				"path":       "big.txt",
				"start_line": "500",
				"end_line":   "510",
			},
		},
		{
			name: "读取第一行",
			file: "big.txt",
			args: map[string]string{
				"path":       "big.txt",
				"start_line": "1",
				"end_line":   "1",
			},
		},
		{
			name: "读取最后一行",
			file: "big.txt",
			args: map[string]string{
				"path":       "big.txt",
				"start_line": "1000",
				"end_line":   "1000",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := handleReadFileWithLineRange(ctx, tt.args)
			if err != nil {
				t.Errorf("handleReadFileWithLineRange() error: %v", err)
				return
			}

			// 验证基本输出格式
			if !strings.Contains(result, "📄 文件内容") {
				t.Errorf("Result missing header")
			}
			if !strings.Contains(result, "文件信息:") {
				t.Errorf("Result missing file info")
			}
			if !strings.Contains(result, "执行统计:") {
				t.Errorf("Result missing execution stats")
			}
		})
	}
}
