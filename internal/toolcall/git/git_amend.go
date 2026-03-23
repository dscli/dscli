package git

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
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
		Strict: true,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitAmend,
	})
}

// handleGitAmend 处理Git amend操作
func handleGitAmend(ctx context.Context, args ToolArgs) (string, error) {
	// 显示操作标题
	PrintGitSection("修改提交")

	// 安全检查
	if safe, reason := checkAmendSafety(ctx); !safe {
		outfmt.Error("安全检查失败: %s", reason)
		return "", fmt.Errorf("amend操作不安全: %s", reason)
	}

	// 获取参数
	message, _ := args["message"].(string)
	noEdit, _ := args["no_edit"].(bool)

	// 构建git命令参数
	gitArgs := []string{"commit", "--amend"}
	if noEdit {
		gitArgs = append(gitArgs, "--no-edit")
	} else if message != "" {
		gitArgs = append(gitArgs, "-m", message)
	}

	outfmt.Info("执行: git %s", strings.Join(gitArgs, " "))

	// 执行git命令
	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = context.ProjectRoot
	output, err := cmd.CombinedOutput()
	out := string(output)

	if err != nil {
		outfmt.Error("Git提交修改失败: %v", err)
		if out != "" {
			outfmt.Error("输出: %s", out)
		}
		return "", fmt.Errorf("git commit --amend失败: %v", err)
	}

	// 如果输出为空，显示成功消息
	if out == "" {
		outfmt.Success("提交修改成功")
		return "Git提交修改成功", nil
	}

	// 提取提交哈希（如果可能）
	if strings.Contains(out, "[") && strings.Contains(out, "]") {
		for line := range strings.SplitSeq(out, "\n") {
			if strings.Contains(line, "[") && strings.Contains(line, "]") {
				outfmt.Success("提交修改成功: %s", strings.TrimSpace(line))
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
	_, err := uncommittedChanges(ctx)
	if err != nil && err.Error() != "no uncommitted changes" {
		return false, fmt.Sprintf("检查未提交更改失败: %v", err)
	}

	// 检查3：是否已推送
	if isCommitPushed(ctx) {
		outfmt.Error("⚠️  警告：当前提交可能已推送到远程仓库")
		outfmt.Error("   已推送的提交不可修改！")
		return false, "当前提交已推送到远程仓库，已推送提交不可修改"
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

// uncommittedChanges 检查是否有未提交的更改
func uncommittedChanges(ctx context.Context) (hasChanges bool, err error) {
	// 执行git status --porcelain
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = context.ProjectRoot
	output, err := cmd.Output()
	if err != nil {
		err = fmt.Errorf("git status failed: %w", err)
		return
	}

	out := string(output)
	if out == "" {
		err = fmt.Errorf("no uncommitted changes")
		return
	}

	// 分割行
	for line := range strings.SplitSeq(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// 检查是否为未提交的更改
		// git status --porcelain格式: XY filename
		// X: 暂存区状态, Y: 工作区状态
		// 只要不是"??"（新文件）或"!!"（忽略文件），就认为有未提交的更改
		if len(line) >= 2 {
			status := line[:2]
			if status != "??" && status != "!!" {
				hasChanges = true
				return
			}
		}
	}

	err = fmt.Errorf("no uncommitted changes")
	return
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
