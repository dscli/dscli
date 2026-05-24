package file

import (
	"context"
	_ "embed"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed file_read.md
var file_read_md string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "read_file",
		Description: file_read_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, e.g. main.go",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleReadFile,
	})
}

// handleReadFile 读取文件完整内容（read_file_with_line_range 的简化版本，等价于不传行范围参数）
func handleReadFile(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	result, warning, err = handleReadFileWithLineRange(ctx, args)
	return result, warning, err
}
