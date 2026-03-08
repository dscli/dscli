package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// gitCommand 执行git命令（直接使用exec.Command）
func gitCommand(ctx context.Context, args ...string) (string, error) {
	// 检查context是否已经取消
	if ctx.Err() != nil {
		return "", fmt.Errorf("the context has been cancelled: %w", ctx.Err())
	}

	// 创建命令
	cmd := exec.Command("git", args...)
	cmd.Dir = ProjectRoot

	// 设置输出缓冲区
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// 启动命令
	startTime := time.Now()
	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("failed to start git command: %w", err)
	}

	// 创建完成通道
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// 等待命令完成或context取消
	select {
	case <-ctx.Done():
		// context被取消或超时，终止进程
		cmd.Process.Kill()
		<-done // 等待进程完全终止
		err = ctx.Err()
		if err == context.DeadlineExceeded {
			return stderrBuf.String(), fmt.Errorf("git命令执行超时: %w", err)
		}
		return stderrBuf.String(), fmt.Errorf("git命令被取消: %w", err)
	case err = <-done:
		// 命令执行完成
		stdout := stdoutBuf.String()
		stderr := stderrBuf.String()

		if err != nil {
			// 命令执行失败
			errorMsg := stderr
			if errorMsg == "" {
				errorMsg = err.Error()
			}
			return stdout, fmt.Errorf("failed to execute git command: %s", errorMsg)
		}

		// 命令执行成功
		executionTime := time.Since(startTime)
		if stdout == "" && stderr == "" {
			stdout = "命令执行成功（无输出）"
		}

		// 构建包含执行统计的结果
		result := fmt.Sprintf(`📝 执行结果:
%s

📊 执行统计:
执行时间: %v
状态: 成功`, stdout, executionTime)

		return result, nil
	}
}

// handleGitAdd git添加
func handleGitAdd(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok {
		path = ""
	}
	path = strings.TrimSpace(path)
	Println("git add", path)
	names := strings.Fields(path)
	gitArgs := []string{"add"}
	gitArgs = append(gitArgs, names...)
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = fmt.Sprintf("(%s)已添加到暂存区", strings.Join(names, " "))
	}
	return out, nil
}

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
	optionWords := strings.FieldsSeq(options)
	for word := range optionWords {
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

// handleGitLog git日志
// handleGitLog git日志
// handleGitLog git日志
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

// handleGitDiff git差异
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

// init
func init() {
	RegisterTool(ToolDef{
		Name:        "git_add",
		Description: "将文件添加到 Git 暂存区",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录），多个文件用空格分隔",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitAdd,
	})

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
