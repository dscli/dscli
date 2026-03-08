package main

import "context"

func init() {
	RegisterTool(ToolDef{
		Name:        "git_status",
		Description: "查看 Git 仓库状态",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitStatus,
	})
}

// handleGitStatus git状态
func handleGitStatus(ctx context.Context, args map[string]string) (string, error) {
	Println("git status --short")
	out, err := gitCommand(ctx, "status", "--short")
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "工作区干净，无变更"
	}
	return out, nil
}
