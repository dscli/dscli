package main

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_push",
		Description: "推送 Git 分支到远程",
		Strict:      true,
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
func handleGitPush(ctx context.Context, args ToolArgs) (string, error) {
	options := ToolArgsValue(args, "options", "")
	options = strings.TrimSpace(options)

	// 显示操作标题
	PrintGitSection("推送分支")

	// 解析推送选项
	names := strings.Fields(options)
	gitArgs := []string{"push"}

	if options != "" {
		Info("推送选项: %s", options)
		gitArgs = append(gitArgs, names...)
	} else {
		Info("使用默认推送设置")
	}

	// 显示推送信息
	PrintSubSection("推送信息")
	Info("正在推送分支到远程仓库...")

	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}

	// 解析推送结果
	if out == "" || strings.Contains(out, "命令执行成功（无输出）") {
		Success("推送成功")
		return "推送成功", nil
	}

	// 检查推送结果
	lines := strings.Split(strings.TrimSpace(out), "\n")

	PrintSubSection("推送结果")
	for _, line := range lines {
		if strings.Contains(line, "Everything up-to-date") {
			Success("所有分支已是最新")
		} else if strings.Contains(line, "error:") || strings.Contains(line, "fatal:") {
			Error("%s", line)
		} else if strings.Contains(line, "Counting objects:") || strings.Contains(line, "Compressing objects:") {
			Info("%s", line)
		} else if strings.Contains(line, "Writing objects:") || strings.Contains(line, "Total") {
			Success("%s", line)
		} else if strings.Contains(line, "remote:") {
			Notice("%s", line)
		} else {
			fmt.Fprintln(outputWriter, line)
		}
	}

	return out, nil
}
