package main

import (
	"context"
	"fmt"
	"strconv"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_log",
		Description: "查看提交历史",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_count": map[string]any{
					"type":        "string",
					"description": `最大显示数量，默认"10"`,
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitLog,
	})
}

// handleGitLog git日志
func handleGitLog(ctx context.Context, args map[string]string) (string, error) {
	maxCountRaw, ok := args["max_count"]
	if !ok || maxCountRaw == "" {
		maxCountRaw = "10"
	}
	_, err := strconv.Atoi(maxCountRaw)
	if err != nil {
		err = fmt.Errorf("%q is not integer in string: %w", maxCountRaw, err)
		return "", err
	}

	Println("git log --oneline -n", maxCountRaw)
	out, err := gitCommand(ctx, "log", "-n", maxCountRaw, "--oneline")
	if err != nil {
		return "", err
	}

	if out == "" {
		out = "git log succeed without output"
	}
	return out, nil
}
