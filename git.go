package main

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
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
