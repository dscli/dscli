package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

var (
	RegisterTool       = toolcall.RegisterTool
	ShellExec          = toolcall.ShellExec
	TitleLikePattern   = toolcall.TitleLikePattern
	ContentLikePattern = toolcall.ContentLikePattern
)

type (
	ToolDef   = toolcall.ToolDef
	ToolArgs  = toolcall.ToolArgs
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}

// gitCommand 执行git命令（直接使用exec.Command）
func gitCommand(ctx context.Context, args ...string) (string, error) {
	startTime := time.Now()

	// 创建命令
	cmd := exec.CommandContext(ctx, "git", args...)

	// 设置工作目录
	cmd.Dir = context.ProjectRoot

	// 捕获输出
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// 执行命令
	err := cmd.Run()
	stdout := strings.TrimSpace(stdoutBuf.String())
	stderr := strings.TrimSpace(stderrBuf.String())
	executionTime := time.Since(startTime)

	// 记录调试信息
	outfmt.Debug("Git命令执行时间: %v", executionTime)

	if err != nil {
		// 命令执行失败
		errorMsg := stderr
		if errorMsg == "" {
			errorMsg = err.Error()
		}
		outfmt.Error("Git命令执行失败: %s", errorMsg)
		return stdout, fmt.Errorf("failed to execute git command: %s", errorMsg)
	}

	// 命令执行成功
	if stdout == "" && stderr == "" {
		stdout = "命令执行成功（无输出）"
	} else {
		outfmt.Success("Git命令执行成功")
	}

	// 简化输出，不显示执行时间
	return stdout, nil
}
