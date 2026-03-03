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
		want      string // 期望的输出内容（与awk格式一致）
	}{
		{
			name: "读取完整文件",
			args: map[string]string{"path": "test.txt"},
			want: `1: Line 1
2: Line 2
3: Line 3
4: Line 4
5: Line 5
6: Line 6
7: Line 7
8: Line 8
9: Line 9
10: Line 10`,
		},
		{
			name: "读取指定行范围",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "3",
				"end_line":   "7",
			},
			want: `3: Line 3
4: Line 4
5: Line 5
6: Line 6
7: Line 7`,
		},
		{
			name: "从某行到文件末尾",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "8",
			},
			want: `8: Line 8
9: Line 9
10: Line 10`,
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
			want: "", // 空范围返回空字符串，与awk行为一致
		},
		{
			name: "单行读取",
			args: map[string]string{
				"path":       "test.txt",
				"start_line": "5",
				"end_line":   "5",
			},
			want: "5: Line 5",
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

			// 清理结果字符串（去除末尾可能的换行符）
			result = strings.TrimSpace(result)
			if result != tt.want {
				t.Errorf("handleReadFileWithLineRange() output mismatch")
				t.Logf("Expected:\n%s", tt.want)
				t.Logf("Got:\n%s", result)
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
		args map[string]string
		want string
	}{
		{
			name: "空文件读取",
			args: map[string]string{"path": "empty.txt"},
			want: "", // 空文件返回空字符串
		},
		{
			name: "单行文件完整读取",
			args: map[string]string{"path": "single.txt"},
			want: "1: Single line",
		},
		{
			name: "大文件部分读取",
			args: map[string]string{
				"path":       "big.txt",
				"start_line": "500",
				"end_line":   "510",
			},
			want: `500: Line 0500
501: Line 0501
502: Line 0502
503: Line 0503
504: Line 0504
505: Line 0505
506: Line 0506
507: Line 0507
508: Line 0508
509: Line 0509
510: Line 0510`,
		},
		{
			name: "读取第一行",
			args: map[string]string{
				"path":       "big.txt",
				"start_line": "1",
				"end_line":   "1",
			},
			want: "1: Line 0001",
		},
		{
			name: "读取最后一行",
			args: map[string]string{
				"path":       "big.txt",
				"start_line": "1000",
				"end_line":   "1000",
			},
			want: "1000: Line 1000",
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

			// 清理结果字符串
			result = strings.TrimSpace(result)
			if result != tt.want {
				t.Errorf("handleReadFileWithLineRange() output mismatch")
				t.Logf("Expected:\n%s", tt.want)
				t.Logf("Got:\n%s", result)
			}
		})
	}
}

func TestHandleReadFileWithLineRange_AwkComparison(t *testing.T) {
	// 创建与awk测试相同的文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "awk_test.txt")

	// 创建25行测试文件，与之前的awk测试一致
	var content strings.Builder
	for i := 1; i <= 25; i++ {
		fmt.Fprintf(&content, "Line %d\n", i)
	}
	if err := os.WriteFile(testFile, []byte(content.String()), 0o644); err != nil {
		t.Fatalf("Failed to create awk test file: %v", err)
	}

	originalRoot := ProjectRoot
	ProjectRoot = tmpDir
	defer func() { ProjectRoot = originalRoot }()

	// 测试与 awk 'NR>=10 && NR<=20 {print NR": "$0}' 完全一致
	t.Run("与awk格式完全一致", func(t *testing.T) {
		ctx := context.Background()
		result, err := handleReadFileWithLineRange(ctx, map[string]string{
			"path":       "awk_test.txt",
			"start_line": "10",
			"end_line":   "20",
		})
		if err != nil {
			t.Fatalf("handleReadFileWithLineRange() error: %v", err)
		}

		// 期望的输出（与awk完全一致）
		expected := `10: Line 10
11: Line 11
12: Line 12
13: Line 13
14: Line 14
15: Line 15
16: Line 16
17: Line 17
18: Line 18
19: Line 19
20: Line 20`

		result = strings.TrimSpace(result)
		if result != expected {
			t.Errorf("Output does not match awk format")
			t.Logf("Expected (awk format):\n%s", expected)
			t.Logf("Got:\n%s", result)
		}
	})

	// 测试其他awk常用模式
	t.Run("awk常用模式测试", func(t *testing.T) {
		testCases := []struct {
			name       string
			startLine  string
			endLine    string
			awkPattern string
			expected   string
		}{
			{
				name:       "前10行",
				startLine:  "1",
				endLine:    "10",
				awkPattern: "NR<=10",
				expected: `1: Line 1
2: Line 2
3: Line 3
4: Line 4
5: Line 5
6: Line 6
7: Line 7
8: Line 8
9: Line 9
10: Line 10`,
			},
			{
				name:       "最后5行",
				startLine:  "21",
				endLine:    "25",
				awkPattern: "NR>=21",
				expected: `21: Line 21
22: Line 22
23: Line 23
24: Line 24
25: Line 25`,
			},
			{
				name:       "中间5行",
				startLine:  "11",
				endLine:    "15",
				awkPattern: "NR>=11 && NR<=15",
				expected: `11: Line 11
12: Line 12
13: Line 13
14: Line 14
15: Line 15`,
			},
			{
				name:       "单行",
				startLine:  "13",
				endLine:    "13",
				awkPattern: "NR==13",
				expected:   "13: Line 13",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				ctx := context.Background()
				result, err := handleReadFileWithLineRange(ctx, map[string]string{
					"path":       "awk_test.txt",
					"start_line": tc.startLine,
					"end_line":   tc.endLine,
				})
				if err != nil {
					t.Errorf("handleReadFileWithLineRange() error: %v", err)
					return
				}

				result = strings.TrimSpace(result)
				if result != tc.expected {
					t.Errorf("Output does not match awk pattern: %s", tc.awkPattern)
					t.Logf("Expected (对应awk: '%s'):\n%s", tc.awkPattern, tc.expected)
					t.Logf("Got:\n%s", result)
				}
			})
		}
	})
}

func TestHandleSearchFileWithPattern(t *testing.T) {
	// 创建临时测试文件
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// 写入测试内容（包含各种模式）
	content := `First line with no special content
Second line contains the word error in it
Third line is normal
Fourth line has ERROR in uppercase
Fifth line contains error again
Sixth line is normal
Seventh line has Error with capital E
Eighth line is normal
Ninth line contains the word warning
Tenth line is the last line`

	if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
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
		want      string // 期望的输出内容
	}{
		{
			name: "搜索error（不区分大小写，默认上下文5行）",
			args: map[string]string{
				"path":    "test.txt",
				"pattern": "error",
			},
			want: `> 2: Second line contains the word error in it
  1: First line with no special content
  3: Third line is normal
  4: Fourth line has ERROR in uppercase
  5: Fifth line contains error again
  6: Sixth line is normal
  7: Seventh line has Error with capital E

  8: Eighth line is normal
  9: Ninth line contains the word warning

  10: Tenth line is the last line`,
		},
		{
			name: "搜索error（区分大小写）",
			args: map[string]string{
				"path":           "test.txt",
				"pattern":        "error",
				"case_sensitive": "true",
			},
			want: `> 2: Second line contains the word error in it
  1: First line with no special content
  3: Third line is normal
  4: Fourth line has ERROR in uppercase
  5: Fifth line contains error again
  6: Sixth line is normal
  7: Seventh line has Error with capital E

  8: Eighth line is normal
  9: Ninth line contains the word warning
  10: Tenth line is the last line`,
		},
		{
			name: "搜索ERROR（区分大小写）",
			args: map[string]string{
				"path":           "test.txt",
				"pattern":        "ERROR",
				"case_sensitive": "true",
			},
			want: `> 4: Fourth line has ERROR in uppercase
  1: First line with no special content
  2: Second line contains the word error in it
  3: Third line is normal
  5: Fifth line contains error again
  6: Sixth line is normal
  7: Seventh line has Error with capital E
  8: Eighth line is normal
  9: Ninth line contains the word warning`,
		},
		{
			name: "搜索warning（上下文2行）",
			args: map[string]string{
				"path":          "test.txt",
				"pattern":       "warning",
				"context_lines": "2",
			},
			want: `> 9: Ninth line contains the word warning
  7: Seventh line has Error with capital E
  8: Eighth line is normal
  10: Tenth line is the last line`,
		},
		{
			name: "搜索不存在的模式",
			args: map[string]string{
				"path":    "test.txt",
				"pattern": "nonexistent",
			},
			want: "", // 应该返回空字符串
		},
		{
			name: "限制最大匹配数为1",
			args: map[string]string{
				"path":        "test.txt",
				"pattern":     "error",
				"max_matches": "1",
			},
			want: `> 2: Second line contains the word error in it
  1: First line with no special content
  3: Third line is normal
  4: Fourth line has ERROR in uppercase
  5: Fifth line contains error again
  6: Sixth line is normal
  7: Seventh line has Error with capital E`,
		},
		{
			name: "缺少必需参数",
			args: map[string]string{
				"path": "test.txt",
				// 缺少pattern参数
			},
			wantError: true,
		},
		{
			name: "文件不存在",
			args: map[string]string{
				"path":    "nonexistent.txt",
				"pattern": "error",
			},
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			result, err := handleSearchFileWithPattern(ctx, tt.args)

			if tt.wantError {
				if err == nil {
					t.Errorf("handleSearchFileWithPattern() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("handleSearchFileWithPattern() error: %v", err)
				return
			}

			// 清理结果字符串
			result = strings.TrimSpace(result)
			if result != tt.want {
				t.Errorf("handleSearchFileWithPattern() output mismatch")
				t.Logf("Expected:\n%s", tt.want)
				t.Logf("Got:\n%s", result)
			}
		})
	}
}

func TestHandleSearchFileWithPattern_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")

	// 测试边缘情况：空文件
	t.Run("空文件", func(t *testing.T) {
		if err := os.WriteFile(testFile, []byte(""), 0o644); err != nil {
			t.Fatalf("Failed to create empty test file: %v", err)
		}

		originalRoot := ProjectRoot
		ProjectRoot = tmpDir
		defer func() { ProjectRoot = originalRoot }()

		ctx := context.Background()
		result, err := handleSearchFileWithPattern(ctx, map[string]string{
			"path":    "test.txt",
			"pattern": "anything",
		})
		if err != nil {
			t.Errorf("handleSearchFileWithPattern() error: %v", err)
			return
		}

		if strings.TrimSpace(result) != "" {
			t.Errorf("Expected empty result for empty file, got: %s", result)
		}
	})

	// 测试边缘情况：单行文件
	t.Run("单行文件匹配", func(t *testing.T) {
		content := "This line contains the word test"
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to create single line test file: %v", err)
		}

		originalRoot := ProjectRoot
		ProjectRoot = tmpDir
		defer func() { ProjectRoot = originalRoot }()

		ctx := context.Background()
		result, err := handleSearchFileWithPattern(ctx, map[string]string{
			"path":    "test.txt",
			"pattern": "test",
		})
		if err != nil {
			t.Errorf("handleSearchFileWithPattern() error: %v", err)
			return
		}

		expected := "> 1: This line contains the word test"
		if strings.TrimSpace(result) != expected {
			t.Errorf("Expected: %s\nGot: %s", expected, result)
		}
	})

	// 测试边缘情况：上下文行数超过文件边界
	t.Run("大上下文行数", func(t *testing.T) {
		// 创建只有3行的文件
		content := "Line 1\nLine 2 contains pattern\nLine 3"
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		originalRoot := ProjectRoot
		ProjectRoot = tmpDir
		defer func() { ProjectRoot = originalRoot }()

		ctx := context.Background()
		result, err := handleSearchFileWithPattern(ctx, map[string]string{
			"path":          "test.txt",
			"pattern":       "pattern",
			"context_lines": "10", // 上下文行数大于文件行数
		})
		if err != nil {
			t.Errorf("handleSearchFileWithPattern() error: %v", err)
			return
		}

		expected := `> 2: Line 2 contains pattern
  1: Line 1
  3: Line 3`
		if strings.TrimSpace(result) != expected {
			t.Errorf("Expected:\n%s\nGot:\n%s", expected, result)
		}
	})

	// 测试边缘情况：重叠的上下文
	t.Run("重叠上下文", func(t *testing.T) {
		// 创建文件，其中两个匹配行很接近
		content := `Line 1
Line 2 contains first match
Line 3
Line 4 contains second match
Line 5
Line 6`
		if err := os.WriteFile(testFile, []byte(content), 0o644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		originalRoot := ProjectRoot
		ProjectRoot = tmpDir
		defer func() { ProjectRoot = originalRoot }()

		ctx := context.Background()
		result, err := handleSearchFileWithPattern(ctx, map[string]string{
			"path":          "test.txt",
			"pattern":       "match",
			"context_lines": "2",
		})
		if err != nil {
			t.Errorf("handleSearchFileWithPattern() error: %v", err)
			return
		}

		// 检查是否有重复行
		lines := strings.Split(strings.TrimSpace(result), "\n")
		lineSet := make(map[string]bool)
		for _, line := range lines {
			if lineSet[line] {
				t.Errorf("Found duplicate line in output: %s", line)
			}
			lineSet[line] = true
		}
	})
}
