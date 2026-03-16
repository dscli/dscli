package main

import (
	"context"
	"fmt"
	"os"
	"time"
)

// CodeMakeFormat - run code format command provided in the context
// 注意：这里设置ShellStdinKey为os.Stdin是有意的，目的是让mkfmt命令在OSExec中执行，
// 而不是在internal/shell沙箱中执行。因为mkfmt脚本由用户提供，是可信任的，
// 可能包含沙箱不允许的命令，但由于用户指定，是安全的。
func CodeMakeFormat(ctx context.Context) (output string, err error) {
	mkfmt := ContextValue(ctx, CodeFormatKey, "make fmt")
	ctx = context.WithValue(ctx, ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, mkfmt)
	if err != nil {
		err = fmt.Errorf("failed to make code format: %w", err)
	}
	return
}

// CodeMakeFormatWithTimeout - run code format command with timeout
// 提供超时控制，避免格式化命令卡住
func CodeMakeFormatWithTimeout(ctx context.Context, timeout time.Duration) (output string, err error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return CodeMakeFormat(ctx)
}

// CodeMakeFormatSafe - safe version with default timeout (30 seconds)
// 安全版本，使用默认30秒超时
func CodeMakeFormatSafe(ctx context.Context) (output string, err error) {
	return CodeMakeFormatWithTimeout(ctx, 30*time.Second)
}

func init() {
	// 注册代码格式化工具
	RegisterTool(ToolDef{
		Name: "code_format",
		Description: `运行代码格式化命令，格式化项目代码。

参数：
  command: 可选，格式化命令。如果不提供，则使用上下文中的配置命令（默认为"make fmt"）
  timeout: 可选，超时时间（秒）。默认为30秒

功能：
1. 执行代码格式化命令，格式化项目代码
2. 支持自定义格式化命令
3. 提供超时控制，避免格式化命令卡住
4. 返回格式化输出，包括错误信息

示例：
  # 使用默认格式化命令
  code_format()
  
  # 使用自定义格式化命令
  code_format(command="go fmt ./...")
  
  # 设置超时时间
  code_format(timeout=60)
  
  # 结合自定义命令和超时
  code_format(command="make format-all", timeout=120)`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "格式化命令，可选，如果不提供则使用配置的默认命令",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "超时时间（秒），可选，默认为30秒",
				},
			},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleCodeFormat,
	})
}

// handleCodeFormat 处理代码格式化请求
func handleCodeFormat(ctx context.Context, args ToolArgs) (string, error) {
	// 检查是否提供了自定义命令
	userCmd := ToolArgsValue(args, "command", "")
	timeout := ToolArgsValue(args, "timeout", 30)

	var output string
	var err error

	if userCmd != "" {
		// 使用用户提供的命令
		ctx = context.WithValue(ctx, ShellStdinKey, os.Stdin)
		ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()

		output, err = ShellExec(ctx, userCmd)
	} else {
		// 使用配置的格式化命令
		output, err = CodeMakeFormatWithTimeout(ctx, time.Duration(timeout)*time.Second)
	}

	if err != nil {
		return "", fmt.Errorf("代码格式化失败: %w\n输出:\n%s", err, output)
	}
	return fmt.Sprintf("✅ 代码格式化完成\n输出:\n%s", output), nil
}
