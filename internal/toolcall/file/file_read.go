package file

import (
	"context"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "read_file",
		Description: "读取项目内指定文件的完整内容，输出带行号的格式。\n\n输出格式与 awk 'NR>=1 {print NR\": \"$0}' 完全一致。\n\n与 read_file_with_line_range 的关系：\n- read_file：读取整个文件（无行范围参数，更简洁）\n- read_file_with_line_range：支持指定行范围（start_line/end_line），当不传行范围时两者等价",
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
func handleReadFile(ctx context.Context, args ToolArgs) (result string, warning string, err error) {
	result, warning, err = handleReadFileWithLineRange(ctx, args)
	return
}
