package main

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/context"
)

// CodeMakeBuild - run build command provided in the context
// 注意：这里设置ShellStdinKey为os.Stdin是有意的，目的是让构建命令在OSExec中执行，
// 而不是在internal/shell沙箱中执行。因为构建脚本由用户提供，是可信任的，
// 可能包含沙箱不允许的命令，但由于用户指定，是安全的。
func CodeMakeBuild(ctx context.Context) (output string, err error) {
	buildCmd := context.ContextValue(ctx, context.MakeBuildKey, "make build")
	ctx = context.WithValue(ctx, context.ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, buildCmd)
	if err != nil {
		err = fmt.Errorf("failed to make build: %w", err)
	}
	return
}

func init() {
	// 注册构建检查工具
	RegisterTool(ToolDef{
		Name: "make_build",
		Description: `检查项目构建是否成功，主要用于发现语法错误和编译问题。

参数：
  command: 可选，构建命令。如果不提供，则使用上下文中的配置命令（默认为"make build"）

功能：
1. 执行构建命令检查项目是否能成功构建
2. 返回构建输出，包括错误信息
3. 主要用于发现语法错误、类型错误等编译问题

示例：
  # 使用默认构建命令
  make_build()
  
  # 使用自定义构建命令
  make_build(command="go build ./...")
  
  # 使用特定目标的构建命令
  make_build(command="make build-all")`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "构建命令，可选，如果不提供则使用配置的默认命令",
					"pattern":     TitleLikePattern(128),
				},
			},
			"additionalProperties": false,
		},
		Category: "build_ops",
		Handler:  handleMakeBuild,
	})
}

// handleMakeBuild 处理构建检查请求
func handleMakeBuild(ctx context.Context, args ToolArgs) (string, error) {
	// 检查是否提供了自定义命令
	userCmd := ToolArgsValue(args, "command", "")

	// 记录使用的命令
	cmdToUse := userCmd
	if cmdToUse == "" {
		cmdToUse = context.ContextValue(ctx, context.MakeBuildKey, "make build")
	}
	Printf("🔨 执行构建命令: %s", cmdToUse)

	if userCmd != "" {
		// 使用用户提供的命令
		ctx = context.WithValue(ctx, context.ShellStdinKey, os.Stdin)
		output, err := ShellExec(ctx, userCmd)
		if err != nil {
			return "", fmt.Errorf("构建失败: %w\n输出:\n%s", err, output)
		}
		// 如果输出为空，添加提示信息
		if output == "" {
			output = "（构建命令执行成功，但无输出）"
		}
		return fmt.Sprintf("🔨 构建成功\n命令: %s\n输出:\n%s", userCmd, output), nil
	}

	// 使用配置的构建命令
	output, err := CodeMakeBuild(ctx)
	if err != nil {
		return "", fmt.Errorf("构建失败: %w\n输出:\n%s", err, output)
	}
	// 如果输出为空，添加提示信息
	if output == "" {
		output = "（构建命令执行成功，但无输出）"
	}
	return fmt.Sprintf("🔨 构建成功\n命令: %s\n输出:\n%s", cmdToUse, output), nil
}
