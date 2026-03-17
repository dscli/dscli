package main

import (
	"context"
	"fmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "write_file",
		Description: "将内容写入文件，如果文件不存在则创建，如果文件存在，append=false覆盖，append=true追加。支持创建目录结构。",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
					"pattern":     TitleLikePattern(128),
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要新建或追加的内容，建议不超过4096个字符",
					"pattern":     ContentLikePattern(4096),
				},
				"append": map[string]any{
					"type":        "boolean",
					"description": "追加或覆盖，true为追加，默认false为覆盖",
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
func handleWriteFile(ctx context.Context, args ToolArgs) (output string, err error) {
	path := ToolArgsValue(args, "path", "")
	content := ToolArgsValue(args, "content", "")
	if path == "" || content == "" {
		err = fmt.Errorf("文件路径 path 和文件内容 content 都不能为空")
		return
	}

	append := ToolArgsValue(args, "append", false)
	delete(args, "append")
	if !append {
		return handleWriteFileWithLineRange(ctx, args)
	}

	// append = true 时追加
	args["start_line"] = -1
	return handleWriteFileWithLineRange(ctx, args)
}
