package file

import (
	"context"
	"fmt"
	"os"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "write_file",
		Description: `将内容写入文件。如果文件不存在则自动创建目录结构。
- 首次写入：设置 append=false 覆盖或创建文件。
- 追加内容：设置 append=true 在文件末尾追加。
如果内容较大（如超过 8192 字符），请分多次调用本工具，每次写入部分内容，并使用 append=true 追加。
建议每次写入的内容长度不超过 8192 字符（约 500 行普通文本）。`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径",
					"pattern":     toolcall.TitleLikePattern(128),
				},
				"content": map[string]any{
					"type":        "string",
					"description": "写入的内容",
					"pattern":     toolcall.ContentLikePattern(8192),
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "是否追加，false 覆盖或创建，true 在文件末尾追加",
				},
			},
			"required":             []string{"path", "content", "append"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFile,
	})
}

// handleWriteFile 写入文件
func handleWriteFile(ctx context.Context, args toolcall.ToolArgs) (output string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	content := toolcall.ToolArgsValue(args, "content", "")
	if path == "" || content == "" {
		err = fmt.Errorf("文件路径 path 和文件内容 content 都不能为空")
		return
	}

	append := toolcall.ToolArgsValue(args, "append", false)
	delete(args, "append")

	fullPath := ResolvePath(ctx, path)

	if append {
		// 追加模式：打开文件并追加内容
		file, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return "", fmt.Errorf("无法打开文件进行追加: %w", err)
		}
		defer file.Close()

		// 检查文件是否为空
		fi, err := os.Stat(fullPath)
		if err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("获取文件信息失败: %w", err)
		}

		// 如果文件存在且不为空，先添加换行符（除非追加内容以换行符开头）
		if err == nil && fi.Size() > 0 {
			// 检查追加内容是否以换行符开头
			if !strings.HasPrefix(content, "\n") {
				// 总是添加换行符，确保追加内容在新行
				if _, err := file.WriteString("\n"); err != nil {
					return "", fmt.Errorf("写入换行符失败: %w", err)
				}
			}
		}

		// 写入内容
		if _, err := file.WriteString(content); err != nil {
			return "", fmt.Errorf("写入内容失败: %w", err)
		}

		lines := strings.Count(content, "\n") + 1
		if content == "" || strings.HasSuffix(content, "\n") {
			lines = strings.Count(content, "\n")
		}

		outfmt.Notice("追加内容到文件 \"%s\"，添加 %d 行", path, lines)

		// 运行make format并捕获结果

		// 构建最终结果
		result := fmt.Sprintf("成功追加内容到文件 \"%s\"，添加 %d 行", path, lines)
		return result, nil
	}

	// 非追加模式：使用现有的行范围写入
	return handleWriteFileWithLineRange(ctx, args)
}
