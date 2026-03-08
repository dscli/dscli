package main

import (
	"context"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_diff",
		Description: "查看文件或暂存区的差异",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录），多个文件用空格分隔",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitDiff,
	})
}

// handleGitDiff git差异
func handleGitDiff(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok {
		path = ""
	}
	path = strings.TrimSpace(path)

	Println("git diff HEAD --", path)
	gitArgs := []string{"diff"}
	if path != "" {
		names := strings.Fields(path)
		gitArgs = append(gitArgs, "HEAD", "--")
		gitArgs = append(gitArgs, names...)
	}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
}
