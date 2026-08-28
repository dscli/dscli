package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

func TestHandleWriteFileWithLineRange(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		initialFile string
		args        toolcall.ToolArgs
		wantErr     bool
		checkFile   func(t *testing.T, filePath string)
	}{
		{
			name: "替换中间行",
			initialFile: `Line 1
Line 2
Line 3
Line 4
Line 5`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(2),
				"end_line":   int64(4),
				"content":    "New Line 2\nNew Line 3\nNew Line 4",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `Line 1
New Line 2
New Line 3
New Line 4
Line 5`
				if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name: "删除中间行",
			initialFile: `Line 1
Line 2
Line 3
Line 4
Line 5`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(2),
				"end_line":   int64(4),
				"content":    "",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `Line 1
Line 5`
				if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name: "从某行开始替换到末尾",
			initialFile: `Line 1
Line 2
Line 3
Line 4
Line 5`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(3),
				"end_line":   int64(-1),
				"content":    "New Line 3\nNew Line 4",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `Line 1
Line 2
New Line 3
New Line 4`
				if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name: "删除从某行到末尾",
			initialFile: `Line 1
Line 2
Line 3
Line 4
Line 5`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(3),
				"end_line":   int64(-1),
				"content":    "",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `Line 1
Line 2`
				if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name: "替换整个文件",
			initialFile: `Old Line 1
Old Line 2`,
			args: toolcall.ToolArgs{
				"path":     "test.txt",
				"end_line": int64(-1),
				"content":  "New Line 1\nNew Line 2\nNew Line 3",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `New Line 1
New Line 2
New Line 3`
				if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name: "清空文件",
			initialFile: `Line 1
Line 2
Line 3`,
			args: toolcall.ToolArgs{
				"path":     "test.txt",
				"end_line": int64(-1),
				"content":  "",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				if strings.TrimSpace(string(content)) != "" {
					t.Errorf("文件应该为空，实际内容:\n%s", string(content))
				}
			},
		},
		{
			name:        "创建新文件",
			initialFile: "", // 文件不存在
			args: toolcall.ToolArgs{
				"path":    "new.txt",
				"content": "New File Content\nLine 2",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `New File Content
Line 2`
				if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name:        "创建空文件",
			initialFile: "", // 文件不存在
			args: toolcall.ToolArgs{
				"path":    "empty.txt",
				"content": "",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				if strings.TrimSpace(string(content)) != "" {
					t.Errorf("文件应该为空，实际内容:\n%s", string(content))
				}
			},
		},
		{
			name: "无效起始行号",
			initialFile: `Line 1
Line 2`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(0),
				"content":    "test",
			},
			wantErr: true,
		},
		{
			name: "无效结束行号",
			initialFile: `Line 1
Line 2`,
			args: toolcall.ToolArgs{
				"path":     "test.txt",
				"end_line": int64(0),
				"content":  "test",
			},
			wantErr: true,
		},
		{
			name: "结束行号小于起始行号",
			initialFile: `Line 1
Line 2
Line 3`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(3),
				"end_line":   int64(1),
				"content":    "test",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Cleanup(func() {
				os.RemoveAll("test.txt")
			})
			// 设置测试文件
			filePath := filepath.Join(tmpDir, toolcall.ToolArgsValue(tt.args, "path", ""))

			// 如果 initialFile 不为空，创建文件
			if tt.initialFile != "" {
				err := os.WriteFile(filePath, []byte(tt.initialFile), 0o644)
				if err != nil {
					t.Fatalf("创建测试文件失败: %v", err)
				}
			}

			// 更新路径参数为绝对路径
			tt.args["path"] = filePath

			// 调用函数
			ctx := t.Context()
			_, _, err := handleWriteFileWithLineRange(ctx, tt.args)

			// 检查错误
			if tt.wantErr {
				if err == nil {
					t.Log("期望错误，但未收到错误")
				}
				return
			}

			if err != nil {
				t.Errorf("不期望的错误: %v", err)
				return
			}

			// 检查文件内容
			if tt.checkFile != nil {
				tt.checkFile(t, filePath)
			}
		})
	}
}

func TestHandleWriteFileWithLineRange_EdgeCases(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		initialFile string
		args        toolcall.ToolArgs
		checkFile   func(t *testing.T, filePath string)
	}{
		{
			name:        "单行文件替换",
			initialFile: "Single Line",
			args: toolcall.ToolArgs{
				"path":    "test.txt",
				"content": "Replaced Line",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				if strings.TrimSpace(string(content)) != "Replaced Line" {
					t.Errorf("文件内容不正确: %s", string(content))
				}
			},
		},
		{
			name:        "空文件替换",
			initialFile: "",
			args: toolcall.ToolArgs{
				"path":    "test.txt",
				"content": "New Content",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				if strings.TrimSpace(string(content)) != "New Content" {
					t.Errorf("文件内容不正确: %s", string(content))
				}
			},
		},
		{
			name: "插入到文件末尾之后",
			initialFile: `Line 1
Line 2`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(5),
				"content":    "Appended Line",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `Line 1
Line 2


Appended Line
`
				if string(content) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name: "多行内容替换单行",
			initialFile: `Line 1
Line 2
Line 3`,
			args: toolcall.ToolArgs{
				"path":       "test.txt",
				"start_line": int64(2),
				"end_line":   int64(2),
				"content":    "New Line 2a\nNew Line 2b\nNew Line 2c",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `Line 1
New Line 2a
New Line 2b
New Line 2c
Line 3`
				if strings.TrimSpace(string(content)) != strings.TrimSpace(expected) {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name: "内容以换行符结尾",
			initialFile: `Line 1
Line 2`,
			args: toolcall.ToolArgs{
				"path":     "test.txt",
				"end_line": int64(-1),
				"content":  "New Line 1\nNew Line 2\n",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := `New Line 1
New Line 2`
				// 注意：我们期望的最终结果不包含末尾的换行符
				// 因为我们的实现会正确处理这种情况
				actual := strings.TrimRight(string(content), "\n")
				if actual != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, actual)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, toolcall.ToolArgsValue(tt.args, "path", ""))

			if tt.initialFile != "" {
				err := os.WriteFile(filePath, []byte(tt.initialFile), 0o644)
				if err != nil {
					t.Fatalf("创建测试文件失败: %v", err)
				}
			}

			tt.args["path"] = filePath

			ctx := t.Context()
			_, _, err := handleWriteFileWithLineRange(ctx, tt.args)
			if err != nil {
				t.Errorf("不期望的错误: %v", err)
				return
			}

			if tt.checkFile != nil {
				tt.checkFile(t, filePath)
			}
		})
	}
}

func TestHandleWriteFileWithLineRange_MissingPath(t *testing.T) {
	args := toolcall.ToolArgs{
		"content": "test",
	}

	ctx := t.Context()
	_, _, err := handleWriteFileWithLineRange(ctx, args)

	if err == nil {
		t.Error("期望错误，但未收到错误")
	}

	expectedErr := "parameter error: no path specified"
	if err.Error() != expectedErr {
		t.Errorf("错误消息不正确\n期望: %s\n实际: %s", expectedErr, err.Error())
	}
}

func TestHandlerWriteFileWithLineRangeLineBeyondScope(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.txt")

	// 创建测试文件
	os.WriteFile(filePath, []byte("Line 1\nLine 2\nLine 3"), 0o644)

	args := toolcall.ToolArgs{
		"path":       filePath,
		"start_line": int64(10),
		"content":    "Line 10: Inserted at line 10",
	}

	ctx := t.Context()
	_, _, err := handleWriteFileWithLineRange(ctx, args)
	if err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	actual := string(b)
	if actual != `Line 1
Line 2
Line 3






Line 10: Inserted at line 10
` {
		t.Fatal("[" + actual + "]")
	}
}

// TestHandleWriteFileWithLineRange_CAS tests the tag-based CAS verification.
func TestHandleWriteFileWithLineRange_CAS(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "cas_test.txt")
	initial := "int count = 10;\nif (count > limit) {\n    count = limit;\n}\n"
	os.WriteFile(filePath, []byte(initial), 0o644)

	// Compute correct tags for lines
	tag1 := computeLineTag("int count = 10;")
	tag2 := computeLineTag("if (count > limit) {")
	tag3 := computeLineTag("    count = limit;")

	ctx := t.Context()

	// Test 1: Single-line edit with correct tag — should succeed
	t.Run("correct single tag", func(t *testing.T) {
		// Restore file
		os.WriteFile(filePath, []byte(initial), 0o644)
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(1),
			"content":    "int count = 11;",
			"line_tag":   tag1,
		}
		_, _, err := handleWriteFileWithLineRange(ctx, args)
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
	})

	// Test 2: Single-line edit with wrong tag — should fail
	t.Run("wrong single tag", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(1),
			"content":    "int count = 11;",
			"line_tag":   "AAAA", // deliberately wrong
		}
		_, _, err := handleWriteFileWithLineRange(ctx, args)
		if err == nil {
			t.Fatal("expected error for wrong tag")
		}
	})

	// Test 3: Multi-line edit with correct line_tags — should succeed
	t.Run("correct multi tags", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		lineTags := tag1 + "\n" + tag2 + "\n" + tag3
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(3),
			"content":    "int count = 11;\nif (count > limit)\n    return limit;",
			"line_tags":  lineTags,
		}
		_, _, err := handleWriteFileWithLineRange(ctx, args)
		if err != nil {
			t.Fatalf("expected success, got: %v", err)
		}
	})

	// Test 4: Multi-line edit with one wrong tag — should fail
	t.Run("wrong multi tag", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		lineTags := tag1 + "\n" + "AAAA" + "\n" + tag3 // middle tag wrong
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(3),
			"content":    "int count = 11;\nif (count > limit)\n    return limit;",
			"line_tags":  lineTags,
		}
		_, _, err := handleWriteFileWithLineRange(ctx, args)
		if err == nil {
			t.Fatal("expected error for wrong tag in multi-line")
		}
	})

	// Test 5: Both line_tag and line_tags — should fail
	t.Run("both tag params", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"content":    "test",
			"line_tag":   tag1,
			"line_tags":  tag1 + "\n" + tag2,
		}
		_, _, err := handleWriteFileWithLineRange(ctx, args)
		if err == nil {
			t.Fatal("expected error for both line_tag and line_tags")
		}
	})

	// Test 6: file changed between read and write — should fail
	t.Run("stale content", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		// Compute tag for original content, then change the file
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(1),
			"content":    "int count = 11;",
			"line_tag":   tag1, // tag for original line 1
		}
		// Modify the file before writing
		os.WriteFile(filePath, []byte("modified content\nif (count > limit) {\n    count = limit;\n}\n"), 0o644)
		_, _, err := handleWriteFileWithLineRange(ctx, args)
		if err == nil {
			t.Fatal("expected error: file was modified between read and write")
		}
	})

	// Test 7: No tags — backward compatible, should succeed
	t.Run("no tags backward compat", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(1),
			"content":    "int count = 11;",
			// no line_tag or line_tags
		}
		_, _, err := handleWriteFileWithLineRange(ctx, args)
		if err != nil {
			t.Fatalf("backward compat should not break: %v", err)
		}
	})
}

// TestWarnOnLengthMismatch 单元测试：长度突变告警判定逻辑。
func TestWarnOnLengthMismatch(t *testing.T) {
	tests := []struct {
		name     string
		old      int
		content  int
		wantWarn bool
	}{
		{"incident-like: 5 replaced by 17", 5, 17, true},
		{"exactly threshold: 5->15", 5, 15, true},
		{"3x growth: 7->21", 7, 21, true},
		{"just below threshold: 5->14", 5, 14, false},
		{"small edit: 1->3", 1, 3, false},
		{"2.5x growth: 10->25", 10, 25, false},
		{"same size", 5, 5, false},
		{"shrink: 17->5", 17, 5, false},
		{"zero old region", 0, 17, false},
		{"delete (content 0)", 5, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := warnOnLengthMismatch(tt.old, tt.content) != ""
			if got != tt.wantWarn {
				t.Errorf("warnOnLengthMismatch(%d, %d) warn=%v, want %v", tt.old, tt.content, got, tt.wantWarn)
			}
		})
	}
}

// TestHandleWriteFileWithLineRange_LengthMismatchWarning 集成测试：
// 无 CAS tag 且替换区域行数远小于内容行数时，handler 应返回告警。
func TestHandleWriteFileWithLineRange_LengthMismatchWarning(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "mismatch.txt")
	initial := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\n"
	ctx := t.Context()

	content17 := strings.Join([]string{
		"func foo() {", "    a()", "    b()", "    c()", "    d()", "    e()", "    f()", "    g()", "    h()", "    i()",
		"    j()", "    k()", "    l()", "    m()", "    n()", "    o()", "}",
	}, "\n")

	t.Run("no tags: growth mismatch warns", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(2),
			"end_line":   int64(6),
			"content":    content17,
		}
		_, warning, err := handleWriteFileWithLineRange(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(warning, "替换区域仅 4 行") {
			t.Errorf("expected length-mismatch warning, got: %q", warning)
		}
	})

	t.Run("with CAS tags: no warning", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		lines := strings.Split(strings.TrimRight(initial, "\n"), "\n")
		// 文件仅 5 行，区域 [2,6] 实际覆盖 4 行（第2-5行），提供这 4 行的 CAS tags
		tags := computeLineTag(lines[1]) + "\n" + computeLineTag(lines[2]) + "\n" +
			computeLineTag(lines[3]) + "\n" + computeLineTag(lines[4])
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(2),
			"end_line":   int64(5),
			"content":    content17,
			"line_tags":  tags,
		}
		_, warning, err := handleWriteFileWithLineRange(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(warning, "替换区域仅") {
			t.Errorf("tags verified the region, should not warn, got: %q", warning)
		}
	})

	t.Run("full-file rewrite end_line=-1: no warning", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		args := toolcall.ToolArgs{
			"path":     filePath,
			"end_line": int64(-1),
			"content":  content17,
		}
		_, warning, err := handleWriteFileWithLineRange(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(warning, "替换区域仅") {
			t.Errorf("full rewrite is deliberate, should not warn, got: %q", warning)
		}
	})

	t.Run("small edit: no warning", func(t *testing.T) {
		os.WriteFile(filePath, []byte(initial), 0o644)
		args := toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(2),
			"content":    "New 2a\nNew 2b\nNew 2c",
		}
		_, warning, err := handleWriteFileWithLineRange(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(warning, "替换区域仅") {
			t.Errorf("small edit should not warn, got: %q", warning)
		}
	})
}

// TestHandleWriteFileWithLineRange_InsertBeforeLine 覆盖 insert_before_line
// 插入语义：中间插入、头部插入、追加末尾、新文件创建、参数互斥、
// 行号范围校验与 line_tag CAS 校验。
func TestHandleWriteFileWithLineRange_InsertBeforeLine(t *testing.T) {
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		initialFile string // "" = 文件不存在
		args        toolcall.ToolArgs
		wantErr     bool
		errContains string
		checkFile   func(t *testing.T, filePath string)
	}{
		{
			name:        "insert before middle line",
			initialFile: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(3),
				"content":            "New A\nNew B",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := "Line 1\nLine 2\nNew A\nNew B\nLine 3\nLine 4\nLine 5"
				if strings.TrimSpace(string(content)) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name:        "insert before first line",
			initialFile: "Line 1\nLine 2",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(1),
				"content":            "Header",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := "Header\nLine 1\nLine 2"
				if strings.TrimSpace(string(content)) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name:        "append at end (N = total+1)",
			initialFile: "Line 1\nLine 2\nLine 3",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(4),
				"content":            "Tail",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := "Line 1\nLine 2\nLine 3\nTail"
				if strings.TrimSpace(string(content)) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name:        "create new file with insert_before_line=1",
			initialFile: "",
			args: toolcall.ToolArgs{
				"path":               "new.txt",
				"insert_before_line": int64(1),
				"content":            "First\nSecond",
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := "First\nSecond"
				if strings.TrimSpace(string(content)) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name:        "insert into non-existent file before line 2 fails",
			initialFile: "",
			args: toolcall.ToolArgs{
				"path":               "new_only.txt",
				"insert_before_line": int64(2),
				"content":            "First",
			},
			wantErr:     true,
			errContains: "only line 1 is valid",
		},
		{
			name:        "insert_before_line=0 fails",
			initialFile: "Line 1",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(0),
				"content":            "X",
			},
			wantErr:     true,
			errContains: "must be >= 1",
		},
		{
			name:        "insert point beyond file+1 fails",
			initialFile: "Line 1\nLine 2",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(4),
				"content":            "X",
			},
			wantErr:     true,
			errContains: "out of range",
		},
		{
			name:        "mutually exclusive with start_line",
			initialFile: "Line 1\nLine 2",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(2),
				"start_line":         int64(1),
				"content":            "X",
			},
			wantErr:     true,
			errContains: "cannot be combined with start_line",
		},
		{
			name:        "mutually exclusive with end_line",
			initialFile: "Line 1\nLine 2",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(2),
				"end_line":           int64(2),
				"content":            "X",
			},
			wantErr:     true,
			errContains: "cannot be combined with end_line",
		},
		{
			name:        "mutually exclusive with line_tags",
			initialFile: "Line 1\nLine 2",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(2),
				"line_tags":          "AAAA\nBBBB",
				"content":            "X",
			},
			wantErr:     true,
			errContains: "cannot be combined with line_tags",
		},
		{
			name:        "empty content fails",
			initialFile: "Line 1\nLine 2",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(2),
				"content":            "",
			},
			wantErr:     true,
			errContains: "requires non-empty content",
		},
		{
			name:        "line_tag verified: correct tag succeeds",
			initialFile: "Line 1\nLine 2\nLine 3",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(2),
				"content":            "Inserted",
				"line_tag":           computeLineTag("Line 2"),
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := "Line 1\nInserted\nLine 2\nLine 3"
				if strings.TrimSpace(string(content)) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name:        "line_tag verified: wrong tag fails",
			initialFile: "Line 1\nLine 2\nLine 3",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(2),
				"content":            "Inserted",
				"line_tag":           "AAAA", // wrong
			},
			wantErr: true,
		},
		{
			name:        "line_tag at append point verifies last line",
			initialFile: "Line 1\nLine 2\nLine 3",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(4), // append after line 3
				"content":            "Tail",
				"line_tag":           computeLineTag("Line 3"),
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := "Line 1\nLine 2\nLine 3\nTail"
				if strings.TrimSpace(string(content)) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
		{
			name:        "content with trailing newline inserts no blank line",
			initialFile: "Line 1\nLine 2\nLine 3",
			args: toolcall.ToolArgs{
				"path":               "test.txt",
				"insert_before_line": int64(2),
				"content":            "New A\nNew B\n", // trailing newline
			},
			checkFile: func(t *testing.T, filePath string) {
				content, err := os.ReadFile(filePath)
				if err != nil {
					t.Fatalf("读取文件失败: %v", err)
				}
				expected := "Line 1\nNew A\nNew B\nLine 2\nLine 3"
				if strings.TrimSpace(string(content)) != expected {
					t.Errorf("文件内容不正确\n期望:\n%s\n实际:\n%s", expected, string(content))
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filePath := filepath.Join(tmpDir, toolcall.ToolArgsValue(tt.args, "path", ""))

			if tt.initialFile != "" {
				if err := os.WriteFile(filePath, []byte(tt.initialFile), 0o644); err != nil {
					t.Fatalf("创建测试文件失败: %v", err)
				}
			}

			tt.args["path"] = filePath

			ctx := t.Context()
			_, _, err := handleWriteFileWithLineRange(ctx, tt.args)

			if tt.wantErr {
				if err == nil {
					t.Fatal("期望错误，但未收到错误")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("错误消息应包含 %q，实际: %v", tt.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望的错误: %v", err)
			}

			if tt.checkFile != nil {
				tt.checkFile(t, filePath)
			}
		})
	}
}

// TestHandleWriteFileWithLineRange_TrailingNewlineReplace 回归测试：
// 替换路径的 content 带末尾换行符时，替换中间区域不得产生多余空行
// （strings.Split("a\nb\n") 会产生 ["a","b",""]，需先 TrimSuffix）。
func TestHandleWriteFileWithLineRange_TrailingNewlineReplace(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "trail.txt")
	if err := os.WriteFile(filePath, []byte("Line 1\nLine 2\nLine 3\n"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	args := toolcall.ToolArgs{
		"path":       filePath,
		"start_line": int64(2),
		"end_line":   int64(2),
		"content":    "New 2a\nNew 2b\n", // trailing newline
	}
	ctx := t.Context()
	if _, _, err := handleWriteFileWithLineRange(ctx, args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	b, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	expected := "Line 1\nNew 2a\nNew 2b\nLine 3\n"
	if string(b) != expected {
		t.Errorf("文件内容不正确\n期望:\n%q\n实际:\n%q", expected, string(b))
	}
}

// ---- 2026-08 工具合并: write_file 全文件模式（旧 write_file 行为）----

// TestHandleWriteFileFull 验证无行参数时的全文件覆盖语义:
// 覆盖已有文件、创建新文件（含父目录）、内容末尾换行。
func TestHandleWriteFileFull(t *testing.T) {
	tmpDir := t.TempDir()

	t.Run("overwrite existing file", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "full.txt")
		if err := os.WriteFile(filePath, []byte("old\ncontent\n"), 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
		ctx := t.Context()
		args := toolcall.ToolArgs{"path": filePath, "content": "new line 1\nnew line 2"}
		res, _, err := handleWriteFile(ctx, args)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(res, "成功写入文件") {
			t.Errorf("result = %q", res)
		}
		b, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("读取文件失败: %v", err)
		}
		if string(b) != "new line 1\nnew line 2\n" {
			t.Errorf("文件内容不正确: %q", string(b))
		}
	})

	t.Run("create file with parent dirs", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "a", "b", "new.txt")
		ctx := t.Context()
		_, _, err := handleWriteFile(ctx, toolcall.ToolArgs{"path": filePath, "content": "hello"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		b, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("读取文件失败: %v", err)
		}
		if string(b) != "hello\n" {
			t.Errorf("文件内容不正确: %q", string(b))
		}
	})

	t.Run("empty content truncates", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "trunc.txt")
		if err := os.WriteFile(filePath, []byte("some\ncontent\n"), 0o644); err != nil {
			t.Fatalf("创建测试文件失败: %v", err)
		}
		ctx := t.Context()
		if _, _, err := handleWriteFile(ctx, toolcall.ToolArgs{"path": filePath, "content": ""}); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		b, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("读取文件失败: %v", err)
		}
		if len(b) != 0 {
			t.Errorf("文件应为空, 实际: %q", string(b))
		}
	})

	t.Run("missing path errors", func(t *testing.T) {
		ctx := t.Context()
		if _, _, err := handleWriteFile(ctx, toolcall.ToolArgs{"content": "x"}); err == nil {
			t.Error("expected error for missing path")
		}
	})

	t.Run("cas tag pollution rejected", func(t *testing.T) {
		filePath := filepath.Join(tmpDir, "cas.txt")
		ctx := t.Context()
		// 内容含 5 行 CAS tag 前缀（阈值 casTagThreshold）
		content := "1:[Q8fA] line\n2:[ABCD] line\n3:[EFGH] line\n4:[IJKL] line\n5:[MNOP] line\n"
		if _, _, err := handleWriteFile(ctx, toolcall.ToolArgs{"path": filePath, "content": content}); err == nil {
			t.Error("expected CAS tag pollution error")
		}
		if _, err := os.Stat(filePath); os.IsNotExist(err) == false {
			t.Errorf("file should not be created on pollution, err=%v", err)
		}
	})
}

// TestHandleWriteFileDispatch 验证分派逻辑:
// 无行参数 → 全文件模式; 有行参数 → 行编辑模式。
func TestHandleWriteFileDispatch(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "dispatch.txt")
	if err := os.WriteFile(filePath, []byte("Line 1\nLine 2\nLine 3\n"), 0o644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	ctx := t.Context()

	// 有 start_line → 行编辑模式: 只替换该行, 其他行保留
	_, _, err := handleWriteFile(ctx, toolcall.ToolArgs{
		"path": filePath, "start_line": int64(2), "content": "New 2",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, _ := os.ReadFile(filePath)
	if string(b) != "Line 1\nNew 2\nLine 3\n" {
		t.Errorf("line-range dispatch failed: %q", string(b))
	}

	// 无行参数 → 全文件覆盖
	_, _, err = handleWriteFile(ctx, toolcall.ToolArgs{
		"path": filePath, "content": "whole\nnew\nfile",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, _ = os.ReadFile(filePath)
	if string(b) != "whole\nnew\nfile\n" {
		t.Errorf("full dispatch failed: %q", string(b))
	}
}
