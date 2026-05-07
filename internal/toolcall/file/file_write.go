package file

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/flycheck"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed file_write.md
var file_write_md string

const (
	// previewLastChars 截断时预览显示的最后字符数
	previewLastChars = 2048
	// maxOutputTokens LLM最大输出token限制（用于错误信息）
	maxOutputTokens = 327680 // 320K
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "write_file",
		Description: file_write_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path",
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "Append mode: false overwrites, true appends, default false",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write",
				},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFile,
	})
}

// handleWriteFile 写入文件
func handleWriteFile(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	truncated := context.ContextValue(ctx, context.FinishReasonLengthKey, false)
	path := toolcall.ToolArgsValue(args, "path", "")
	content := toolcall.ToolArgsValue(args, "content", "")
	lastlines := ""
	if truncated {
		runes := []rune(content)
		start := max(len(runes)-previewLastChars, 0)
		lastlines = string(runes[start:])
	}

	if path == "" {
		err = fmt.Errorf("文件路径 path 不能为空")
		if truncated {
			warning = fmt.Sprintf("内容截断，因为内容长度 %d 超过了最大输出 Tokens 要求 %d，请严格遵守 write_file 要求，严格控制输出。", len(content), maxOutputTokens)
		}
		return result, warning, err
	}

	append := toolcall.ToolArgsValue(args, "append", false)

	fullPath := ResolvePath(ctx, path)
	dirPath := filepath.Dir(fullPath)
	var fi os.FileInfo
	fi, err = os.Stat(dirPath)
	if err == nil && !fi.IsDir() {
		err = fmt.Errorf("%s is not directory", dirPath)
		return result, warning, err
	}

	if err != nil && os.IsNotExist(err) {
		err = os.MkdirAll(dirPath, 0o755)
	}

	if err != nil {
		err = fmt.Errorf("failed to get or create directory %s: %w", dirPath, err)
		return result, warning, err
	}

	var file *os.File
	if append {
		file, err = os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	} else {
		file, err = os.OpenFile(fullPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o644)
	}
	if err != nil {
		err = fmt.Errorf("无法打开文件: %w", err)
		return result, warning, err
	}
	defer file.Close()

	// 检查文件是否为空
	fi, err = os.Stat(fullPath)
	if err != nil && !os.IsNotExist(err) {
		err = fmt.Errorf("获取文件信息失败: %w", err)
		return result, warning, err
	}

	// 如果文件存在且不为空，先添加换行符（除非追加内容以换行符开头）
	if err == nil && fi.Size() > 0 {
		// 检查追加内容是否以换行符开头
		if !strings.HasPrefix(content, "\n") {
			// 总是添加换行符，确保追加内容在新行
			if _, err = file.WriteString("\n"); err != nil {
				err = fmt.Errorf("写入换行符失败: %w", err)
				return result, warning, err
			}
		}
	}

	// 写入内容
	if _, err = file.WriteString(content); err != nil {
		err = fmt.Errorf("写入内容失败: %w", err)
		return result, warning, err
	}

	lines := strings.Count(content, "\n") + 1
	if content == "" || strings.HasSuffix(content, "\n") {
		lines = strings.Count(content, "\n")
	}
	if append {
		outfmt.Notice("追加内容到文件 \"%s\"，添加 %d 行", path, lines)
		result = fmt.Sprintf("成功追加内容到文件 \"%s\"，添加 %d 行。", path, lines)
	} else {
		outfmt.Notice("写入文件 \"%s\"，%d 行", path, lines)
		result = fmt.Sprintf("成功写入文件 \"%s\"，%d 行。", path, lines)
	}
	if truncated {
		warning = fmt.Sprintf(`此次写入文件 %s 的内容是截断的内容。
请从上次输出内容的最后一完整行继续生成，并调用工具 write_file(path="%s", append=true, content="...继续生成的内容...")
追加入文件%s，为帮助你找到继续生成的点，现把上次截断内容最后几行展示给你：
---
%s---
如果觉得信息不足以继续生成，可以停下来询问。`, path, path, path, lastlines)
	}

	// Run flycheck on the written file and append issues to suggestion
	if flyResult, _, flyErr := flycheck.Flycheck(ctx, path); flyErr == nil && flyResult != "" {
		if warning != "" {
			warning += "\n\n"
		}
		warning += flyResult
	}

	return result, warning, err
}
