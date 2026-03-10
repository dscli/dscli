package main

import (
	"context"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "write_file",
		Description: "将内容写入文件，如果文件不存在则创建，如果存在则覆盖。支持创建目录结构。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要写入的内容",
				},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFile,
	})
}

// handleWriteFile 写入文件（纯Go实现）
func handleWriteFile(ctx context.Context, args ToolArgs) (string, error) {
	return handleWriteFileWithLineRange(ctx, args)
}
