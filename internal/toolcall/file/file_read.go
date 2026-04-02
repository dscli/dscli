package file

import (
	"context"

	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "read_file",
		Description: "读取项目内指定文件的内容，返回文件内容和元数据信息（大小、权限、修改时间等）",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
					"pattern":     toolcall.TitleLikePattern(128),
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleReadFile,
	})
}

// handleReadFile 读取文件（纯Go实现）
func handleReadFile(ctx context.Context, args ToolArgs) (output string, user string, err error) {
	output, user, err = handleReadFileWithLineRange(ctx, args)
	return
}
