package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"slices"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "dscli_chat_reload",
		Description: "重新加载dscli chat进程，需要提供confirm=yes参数确认。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"confirm": map[string]any{
					"type":        "string",
					"description": `必须为 ` + "`yes`" + `，用于确认重载操作`,
				},
			},
			"required":             []string{"confirm"},
			"additionalProperties": false,
		},
		Category: "system",
		Handler:  handleDscliChatReload,
	})
}

// handleDscliChatReload 重新加载运行 dscli chat --reload
func handleDscliChatReload(ctx context.Context, args map[string]string) (result string, err error) {
	// 检查是否是重载命令
	isReload := false
	if v, ok := ctx.Value(IsReload).(bool); ok {
		isReload = v
	}

	// 检查 confirm 参数
	confirm, ok := args["confirm"]
	if !ok || confirm != "yes" {
		return "", fmt.Errorf("必须提供 confirm=yes 参数来确认重载")
	}

	if isReload {
		// 如果是重载进程，返回简单确认信息
		result = "重载进程已启动"
		return
	}

	Info("🔄 检测到重载命令，正在重启进程...")

	// 获取命令行参数
	var cmdArgs []string
	if v, ok := ctx.Value(CommandLineArgs).([]string); ok && len(v) > 0 {
		// 使用原始命令行参数
		cmdArgs = make([]string, len(v))
		copy(cmdArgs, v)

		// 确保有 --reload 标志
		hasReload := slices.Contains(cmdArgs, "--reload")
		if !hasReload {
			cmdArgs = append(cmdArgs, "--reload")
		}
	} else {
		// 如果没有命令行参数，使用默认参数
		cmdArgs = []string{"chat", "--reload"}
	}

	// 构建 exec 命令 - 使用绝对路径避免递归
	dscliPath, err := exec.LookPath("dscli")
	if err != nil {
		Error("找不到 dscli 命令: %v", err)
		return "", fmt.Errorf("找不到 dscli 命令: %v", err)
	}

	// 使用绝对路径执行，避免递归
	cmd := exec.Command(dscliPath, cmdArgs...)
	cmd.Dir = ProjectRoot
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 执行 exec（这会替换当前进程）
	if err = cmd.Run(); err != nil {
		Error("重载失败: %v", err)
		// 如果 exec 失败，返回错误信息
		err = fmt.Errorf("重载失败: %v", err)
		return "", err
	} else {
		// exec 成功，进程已被替换，这里不会执行
		os.Exit(0)
	}
	return
}
