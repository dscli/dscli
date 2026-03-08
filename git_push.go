package main

import (
	"context"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_push",
		Description: "推送 Git 分支到远程",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"options": map[string]any{
					"type":        "string",
					"description": "选项，例如：--force-with-lease，多个选项用空格分隔，例如：origin main --force，可为空",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitPush,
	})
}

// handleGitPush git push [options...]
func handleGitPush(ctx context.Context, args map[string]string) (string, error) {
	options, ok := args["options"]
	if !ok {
		options = ""
	}
	options = strings.TrimSpace(options)

	Println("git push", options)
	names := strings.Fields(options)
	gitArgs := []string{"push"}
	if options != "" {
		gitArgs = append(gitArgs, names...)
	}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
}
