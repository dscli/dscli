package main

import (
	"context"
	"fmt"
	"strconv"
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

	// 安全检查
	if safe, reason := checkAmendSafety(ctx); !safe {
		Error("安全检查失败: %s", reason)
		return "", fmt.Errorf("amend操作不安全: %s", reason)
	}

	// 确认操作
	if !confirmAmend() {
		return "用户取消了amend操作", nil
	}

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

// checkAmendSafety 检查amend操作的安全性
func checkAmendSafety(ctx context.Context) (bool, string) {
	// 检查1：是否在git仓库中
	if !isGitRepository(ctx) {
		return false, "当前不在Git仓库中"
	}

	// 检查2：是否有未提交的更改（amend会包含这些更改）
	if hasUncommittedChanges(ctx) {
		Warn("有未暂存的更改，amend操作会包含这些更改")
	}

	// 检查3：是否已推送
	if isCommitPushed(ctx) {
		Error("⚠️  警告：当前提交可能已推送到远程仓库")
		Error("   修改已推送的提交需要使用 git push --force-with-lease")
		Error("   这可能会影响其他协作者，请谨慎操作")
		return false, "当前提交已推送到远程仓库，修改需要强制推送"
	}

	return true, ""
}

// isGitRepository 检查当前是否在Git仓库中
func isGitRepository(ctx context.Context) bool {
	script := `git rev-parse --is-inside-work-tree 2>/dev/null || echo "false"`
	out, err := ShellExec(ctx, script)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) == "true"
}

// hasUncommittedChanges 检查是否有未提交的更改
func hasUncommittedChanges(ctx context.Context) bool {
	script := `git status --short`
	out, err := ShellExec(ctx, script)
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

// isCommitPushed 检查当前提交是否已推送到远程
func isCommitPushed(ctx context.Context) bool {
	// 获取当前分支名
	branchScript := `git branch --show-current 2>/dev/null || echo ""`
	branch, err := ShellExec(ctx, branchScript)
	if err != nil {
		return false
	}
	branch = strings.TrimSpace(branch)
	if branch == "" {
		return false // 没有分支，假设未推送
	}

	// 检查远程分支是否存在
	remoteScript := `git ls-remote --heads origin ` + branch + ` 2>/dev/null | wc -l`
	remoteOut, err := ShellExec(ctx, remoteScript)
	if err != nil {
		return false // 如果检查失败，假设未推送
	}

	remoteCount, _ := strconv.Atoi(strings.TrimSpace(remoteOut))
	if remoteCount == 0 {
		return false // 远程分支不存在，肯定未推送
	}

	// 检查本地HEAD是否已推送到远程
	pushScript := `git log --oneline origin/` + branch + `..HEAD 2>/dev/null | wc -l`
	pushOut, err := ShellExec(ctx, pushScript)
	if err != nil {
		return false // 如果检查失败，假设未推送
	}

	lines := strings.TrimSpace(pushOut)
	count, _ := strconv.Atoi(lines)
	return count == 0 // 如果本地没有比远程多的提交，说明已推送
}

// confirmAmend 确认amend操作
func confirmAmend() bool {
	// 在交互模式下询问用户确认
	// 注意：当前版本假设总是交互模式
	// 未来可以添加IsInteractive()函数检查

	Println("⚠️  确认要修改最新的提交吗？")
	Println("   按 Enter 继续，或输入 'n' 取消")

	var response string
	fmt.Scanln(&response)

	return response == "" || strings.ToLower(response) == "y" || strings.ToLower(response) == "yes"
}
