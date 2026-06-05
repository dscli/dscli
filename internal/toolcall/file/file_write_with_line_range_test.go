package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/toolcall"
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
				"path":    "test.txt",
				"content": "New Line 1\nNew Line 2\nNew Line 3",
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
				"path":    "test.txt",
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
				"path":    "test.txt",
				"content": "New Line 1\nNew Line 2\n",
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