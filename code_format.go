package main

import (
	"context"
	"fmt"
	"os"
	"strings"
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

功能：
1. 执行代码格式化命令，格式化项目代码
2. 支持自定义格式化命令
3. 返回格式化输出，包括错误信息

示例：
  # 使用默认格式化命令
  code_format()
  
  # 使用自定义格式化命令
  code_format(command="go fmt ./...")`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "格式化命令，可选，如果不提供则使用配置的默认命令",
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
	command := ToolArgsValue(args, "command", "")

	var output string
	var err error

	if command == "" {
		command = ContextValue(ctx, CodeFormatKey, "make fmt")
	}

	Printf("代码格式化 %s", command)
	// 使用用户提供的命令
	ctx = context.WithValue(ctx, ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, command)
	if err != nil {
		// 构建详细的错误信息
		var sb strings.Builder
		fmt.Fprintf(&sb, "❌ 代码格式化失败\n")
		fmt.Fprintf(&sb, "📝 使用的命令: %s\n", command)
		fmt.Fprintf(&sb, "💥 错误信息: %v\n", err)
		fmt.Fprintf(&sb, "📄 输出内容:\n%s", output)
		return sb.String(), fmt.Errorf("代码格式化失败")
	}

	// 构建成功信息
	var sb strings.Builder
	fmt.Fprintf(&sb, "✅ 代码格式化完成\n")
	fmt.Fprintf(&sb, "📝 使用的命令: %s\n", command)

	// 分析输出内容
	outputLines := strings.Split(strings.TrimSpace(output), "\n")
	if len(outputLines) == 0 || (len(outputLines) == 1 && outputLines[0] == "") {
		sb.WriteString("📊 格式化结果: 没有输出（可能是静默模式或无更改）\n")
	} else {
		fmt.Fprintf(&sb, "📊 格式化结果: %d 行输出\n", len(outputLines))
		sb.WriteString("📄 输出内容:\n")
		for i, line := range outputLines {
			if line != "" {
				sb.WriteString(fmt.Sprintf("  %d. %s\n", i+1, line))
			}
		}
	}

	// 添加建议
	sb.WriteString("\n💡 建议:\n")
	sb.WriteString("1. 使用 git diff 查看格式化后的代码变更\n")
	sb.WriteString("2. 使用 git status 查看是否有未提交的更改\n")
	sb.WriteString("3. 使用 make_test() 确保格式化后测试仍然通过\n")

	return sb.String(), nil
}
