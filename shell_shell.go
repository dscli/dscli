package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

func init() { // 注册shell工具
	RegisterTool(ToolDef{
		Name: "shell",
		Description: `在项目根目录执行Shell脚本。
支持shebang指定解释器（如bash、sh等）。
脚本通过标准输入传递，避免命令行长度限制。

输出格式：
- 成功时：返回包含执行结果和执行统计的格式化文本
- 失败时：返回包含错误信息、输出内容和执行统计的格式化文本

示例：
1. Bash脚本：echo "Hello"
2. Shell脚本：ls -la
3. 文件操作：cat file.txt
4. Git操作：git status

注意：谨慎使用，避免破坏性操作。确保脚本在项目目录内执行。`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type": "string",
					"description": `要执行的Shell脚本内容。
脚本执行结果会以格式化文本返回，包含执行统计信息。

示例：
1. Bash脚本：echo "Hello"
2. Shell脚本：ls -la
3. 文件操作：cat file.txt
4. Git操作：git status
`,
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		},
		Category: "system",
		Timeout:  60 * time.Second, // 设置60秒超时
		Handler:  handleShell,
	})
}

// handleShell 执行Shell脚本
// handleShell 执行Shell脚本
func handleShell(ctx context.Context, args ToolArgs) (out string, err error) {
	script := ToolArgsValue(args, "script", "")

	if err = validateShell(script); err != nil {
		return
	}

	// 应用智能超时
	ctx = withSmartTimeout(ctx, script)

	Notice("Shell: %s", ShortenShellScript(script))
	out, err = runShell(ctx, script)
	return
}

// withSmartTimeout 根据命令类型设置智能超时
func withSmartTimeout(ctx context.Context, script string) context.Context {
	timeout := classifyCommandTimeout(script)

	// 记录超时设置（调试信息）
	Debug("设置命令超时: %v", timeout)

	ctx, cancel := context.WithTimeout(ctx, timeout)

	// 设置取消函数到上下文，确保资源释放
	ctx = context.WithValue(ctx, "cancelFunc", cancel)

	// 超时预警
	go timeoutWarning(ctx, script, timeout)

	return ctx
}

// classifyCommandTimeout 根据命令类型分类设置超时
func classifyCommandTimeout(script string) time.Duration {
	cmdSummary := ShortenShellScript(script)
	if cmdSummary == "" {
		cmdSummary = script
	}

	cmdLower := strings.ToLower(cmdSummary)

	// 快速命令（10秒）
	quickCmds := []string{"ls", "pwd", "echo", "cat", "head", "tail", "wc", "grep", "find", "which"}
	for _, cmd := range quickCmds {
		if strings.HasPrefix(cmdLower, cmd+" ") || cmdLower == cmd {
			return 10 * time.Second
		}
	}

	// 中等命令（30秒）
	mediumCmds := []string{"tar", "zip", "unzip", "curl", "wget", "git", "docker ps", "kubectl get"}
	for _, cmd := range mediumCmds {
		if strings.Contains(cmdLower, cmd) {
			return 30 * time.Second
		}
	}

	// 构建命令（60秒）
	buildCmds := []string{"make", "go build", "docker build", "npm install", "yarn install", "cargo build"}
	for _, cmd := range buildCmds {
		if strings.Contains(cmdLower, cmd) {
			return 60 * time.Second
		}
	}

	// 默认超时（30秒）
	return 30 * time.Second
}

// timeoutWarning 超时预警
// timeoutWarning 超时预警
func timeoutWarning(ctx context.Context, script string, timeout time.Duration) {
	// 提前20%时间预警
	warningTime := timeout * 4 / 5

	select {
	case <-time.After(warningTime):
		cmdSummary := ShortenShellScript(script)
		if cmdSummary == "" {
			cmdSummary = script
			if len(cmdSummary) > 30 {
				cmdSummary = cmdSummary[:30] + "..."
			}
		}
		Println(fmt.Sprintf("⚠️  命令即将超时: %s (已运行 %v，剩余 %v)",
			cmdSummary, warningTime, timeout-warningTime))
	case <-ctx.Done():
		// 命令已完成或已超时
	}
}

func runShell(ctx context.Context, script string) (result string, err error) {
	startTime := time.Now()
	name, arg := Shebang(script)
	ctx = context.WithValue(ctx, ShellName, name)
	ctx = context.WithValue(ctx, ShellArgs, arg)
	out, err := ShellExec(ctx, script)
	executionTime := time.Since(startTime)

	// 记录调试信息（不在用户输出中显示）
	Debug("Shell命令执行时间: %v", executionTime)

	// 获取命令摘要
	cmdSummary := ShortenShellScript(script)
	if cmdSummary == "" {
		cmdSummary = script
		if len(cmdSummary) > 50 {
			cmdSummary = cmdSummary[:50] + "..."
		}
	}

	if err != nil {
		// 结构化错误输出
		var shellErr *ShellError
		if errors.As(err, &shellErr) {
			result = fmt.Sprintf("❌ %s\n⏱️  耗时: %v\n\n命令: %s\n\n错误详情:\n%s",
				shellErr.Context, executionTime, cmdSummary, shellErr.Error())
		} else {
			result = fmt.Sprintf("❌ 执行失败\n⏱️  耗时: %v\n\n命令: %s\n\n错误: %v\n\n输出内容:\n%s",
				executionTime, cmdSummary, err, out)
		}
		return result, nil
	}

	// 结构化成功输出
	result = fmt.Sprintf("✅ 执行成功\n⏱️  耗时: %v\n\n命令: %s\n\n%s",
		executionTime, cmdSummary, formatOutput(out))
	return
}

// formatOutput 格式化输出内容
func formatOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return "（无输出）"
	}

	lines := strings.Split(output, "\n")
	if len(lines) <= 10 {
		return output
	}

	// 如果输出太长，只显示首尾部分
	head := strings.Join(lines[:5], "\n")
	tail := strings.Join(lines[len(lines)-5:], "\n")
	return fmt.Sprintf("%s\n... (省略 %d 行) ...\n%s",
		head, len(lines)-10, tail)
}
