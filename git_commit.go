package main

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_commit",
		Description: "提交暂存区更改，需要提供提交信息。注意：不要在options中包含-m或--message参数。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "提交信息（不要在options中包含-m或--message参数）",
				},
				"options": map[string]any{
					"type": "string",
					"description": `其他git commit选项，例如：-a（提交所有更改）、
--amend（修改上次提交）、--no-edit（使用原提交信息）、
--allow-empty（允许空提交）。
多个选项用空格分隔，例如：-a --amend --no-edit`,
				},
			},
			"required":             []string{"message"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitCommit,
	})
}

// handleGitCommit git提交
// handleGitCommit git提交
func handleGitCommit(ctx context.Context, args map[string]string) (string, error) {
	message, ok := args["message"]
	if !ok {
		return "", fmt.Errorf("no message specified")
	}

	options, ok := args["options"]
	if !ok {
		options = ""
	}

	Println("git commit", options)

	options = strings.TrimSpace(options)

	// 更健壮的-m参数检查
	// 检查 -m、-m[空格]、--message 等变体
	optionWords := strings.Fields(options)
	for _, word := range optionWords {
		if word == "-m" || word == "--message" || strings.HasPrefix(word, "-m") {
			return "", fmt.Errorf("message参数已通过message字段提供，不要在options中包含-m或--message")
		}
	}

	gitArgs := []string{"commit", "-m", message}
	if options != "" {
		gitArgs = append(gitArgs, strings.Fields(options)...)
	}

	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "Git has commited"
	}
	return out, nil
}
