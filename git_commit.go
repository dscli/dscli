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
func handleGitCommit(ctx context.Context, args ToolArgs) (string, error) {
	message := ToolArgsValue(args, "message", "")
	if message == "" {
		return "", fmt.Errorf("no message specified")
	}

	options := ToolArgsValue(args, "options", "")

	// 显示操作标题
	PrintGitSection("提交更改")

	// 显示提交信息
	Info("提交信息: %s", message)

	if options != "" {
		Info("提交选项: %s", options)
	}

	options = strings.TrimSpace(options)

	// 更健壮的-m参数检查
	// 检查 -m、-m[空格]、--message 等变体
	optionWords := strings.FieldsSeq(options)
	for word := range optionWords {
		if word == "-m" || word == "--message" || strings.HasPrefix(word, "-m") {
			Error("检测到-m或--message参数")
			Warn("提示: message参数已通过message字段提供，不要在options中包含-m或--message")
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

	// 如果输出为空，显示成功消息
	if out == "" || strings.Contains(out, "命令执行成功（无输出）") {
		Success("提交成功: %s", message)
		return "Git提交成功", nil
	}

	// 提取提交哈希（如果可能）
	if strings.Contains(out, "[") && strings.Contains(out, "]") {
		lines := strings.SplitSeq(out, "\n")
		for line := range lines {
			if strings.Contains(line, "[") && strings.Contains(line, "]") {
				Success("提交成功: %s", strings.TrimSpace(line))
				break
			}
		}
	}

	return out, nil
}
