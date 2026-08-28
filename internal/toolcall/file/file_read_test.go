package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/toolcall"
)

// TestHandleReadFileWithLineRange_LargeFileHint 覆盖大文件 size 提示：
// 完整读取超过阈值（read-file-large-threshold，默认 200KB）时，
// 结果尾部应附文件大小提示；行范围读取和小文件不应提示。
func TestHandleReadFileWithLineRange_LargeFileHint(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "large.txt")

	// 构造 ~200KB 文件（12800 行 × 16 字节）
	bigContent := strings.Repeat("0123456789abcdef\n", 12800)
	if err := os.WriteFile(filePath, []byte(bigContent), 0o644); err != nil {
		t.Fatalf("创建大文件失败: %v", err)
	}

	// 阈值可配置：调低到 1KB 使测试文件必然触发（不依赖默认 200KB）
	config.Set("read-file-large-threshold", "1")
	t.Cleanup(func() { config.Set("read-file-large-threshold", "200") })

	ctx := t.Context()

	t.Run("full read of large file hints size", func(t *testing.T) {
		result, _, err := handleReadFile(ctx, toolcall.ToolArgs{"path": filePath})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "⚠️ 文件较大") {
			t.Errorf("expected large-file hint, got result without hint (len=%d)", len(result))
		}
		if !strings.Contains(result, "KB") && !strings.Contains(result, "MB") {
			t.Errorf("hint should include size, got: %q", result[len(result)-200:])
		}
	})

	t.Run("line-range read does not hint", func(t *testing.T) {
		result, _, err := handleReadFile(ctx, toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(5),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result, "⚠️ 文件较大") {
			t.Errorf("range read should not hint, got: %q", result[len(result)-200:])
		}
	})

	t.Run("small file does not hint", func(t *testing.T) {
		smallPath := filepath.Join(tmpDir, "small.txt")
		if err := os.WriteFile(smallPath, []byte("tiny\n"), 0o644); err != nil {
			t.Fatalf("创建小文件失败: %v", err)
		}
		result, _, err := handleReadFile(ctx, toolcall.ToolArgs{"path": smallPath})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if strings.Contains(result, "⚠️ 文件较大") {
			t.Errorf("small file should not hint, got: %q", result)
		}
	})

	t.Run("explicit full range still hints", func(t *testing.T) {
		result, _, err := handleReadFile(ctx, toolcall.ToolArgs{
			"path":       filePath,
			"start_line": int64(1),
			"end_line":   int64(-1),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(result, "⚠️ 文件较大") {
			t.Errorf("explicit full-range read should hint, got result without hint")
		}
	})
}
