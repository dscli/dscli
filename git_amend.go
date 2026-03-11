package main

import (
	"context"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_amend",
		DisplayName: "修改提交",
		Description: `修改最新的Git提交（amend commit）。

参数说明：
- message: 可选，新的提交信息。如果不提供，则保持原提交信息不变。
- no_edit: 可选，是否不编辑提交信息。如果为true，则使用原提交信息或提供的message。

使用场景：
1. 修改上次提交的代码（添加漏掉的文件）
2. 修改上次提交的信息
3. 合并多个小提交为一个

注意：
- 只能修改最新的提交（HEAD）
- 如果修改了已推送的提交，需要使用git push --force-with-lease推送
- 建议在push之前使用此工具

示例：
1. 修改提交内容但不修改信息：git_amend()
2. 修改提交内容和信息：git_amend(message="修复拼写错误")
3. 修改提交内容但不编辑信息：git_amend(no_edit=true)`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "可选，新的提交信息。如果不提供，则保持原提交信息不变。",
				},
				"no_edit": map[string]any{
					"type":        "boolean",
					"description": "可选，是否不编辑提交信息。如果为true，则使用原提交信息或提供的message。",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitAmend,
	})
}

// handleGitAmend 处理Git amend操作
func handleGitAmend(ctx context.Context, args ToolArgs) (string, error) {
	message := ToolArgsValue(args, "message", "")
	noEdit := ToolArgsValue(args, "no_edit", false)

	// 显示操作标题
	PrintGitSection("修改提交")

	// 构建git命令参数
	gitArgs := []string{"commit", "--amend"}

	// 处理message参数
	if message != "" {
		Info("新提交信息: %s", message)
		gitArgs = append(gitArgs, "-m", message)
	}

	// 处理no_edit参数
	if noEdit {
		Info("不编辑提交信息")
		gitArgs = append(gitArgs, "--no-edit")
	}

	// 如果没有提供message且没有设置no_edit，则打开编辑器
	if message == "" && !noEdit {
		Info("将打开编辑器修改提交信息")
	}

	// 执行git命令
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}

	// 处理输出
	if out == "" || strings.Contains(out, "命令执行成功（无输出）") {
		Success("提交修改成功")
		return "Git提交修改成功", nil
	}

	// 提取提交哈希（如果可能）
	if strings.Contains(out, "[") && strings.Contains(out, "]") {
		lines := strings.Split(out, "\n")
		for _, line := range lines {
			if strings.Contains(line, "[") && strings.Contains(line, "]") {
				Success("提交修改成功: %s", strings.TrimSpace(line))
				break
			}
		}
	}

	return out, nil
}
