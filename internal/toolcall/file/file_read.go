package file

import (
	"context"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "read_file",
		Description: "Read file content with line numbers.\n\nOutput format matches: awk 'NR>=1 {print NR\": \"$0}'.\n\nEquivalent to read_file_with_line_range without line range parameters.",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
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