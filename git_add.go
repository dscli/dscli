package main

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_add",
		Description: "将文件添加到 Git 暂存区",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录），多个文件用空格分隔",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitAdd,
	})
}

// handleGitAdd git添加
func handleGitAdd(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok {
		path = ""
	}
	path = strings.TrimSpace(path)
	Println("git add", path)
	names := strings.Fields(path)
	gitArgs := []string{"add"}
	gitArgs = append(gitArgs, names...)
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = fmt.Sprintf("(%s)已添加到暂存区", strings.Join(names, " "))
	}
	return out, nil
}
