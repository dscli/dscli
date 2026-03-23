package make

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
)

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
  参数：
  command: 必需，构建命令

功能：
1. 执行构建命令检查项目是否能成功构建
2. 返回构建输出，包括错误信息
3. 主要用于发现语法错误、类型错误等编译问题

示例：
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
					"description": "构建命令",
					"pattern":     TitleLikePattern(128),
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
		Category: "build_ops",
		Handler:  handleMakeBuild,
	})
}

// handleMakeBuild 处理构建检查请求
func handleMakeBuild(ctx context.Context, args ToolArgs) (output string, err error) {
	// 检查是否提供了自定义命令
	userCmd := ToolArgsValue(args, "command", "")
	if userCmd == "" {
		err = fmt.Errorf("no command provided")
		return
	}

	outfmt.Printf("🔨 执行构建命令: %s", userCmd)

	// 使用用户提供的命令
	ctx = context.WithValue(ctx, context.ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, userCmd)
	if err != nil {
		err = fmt.Errorf("构建失败: %w\n输出:\n%s", err, output)
		return
	}
	// 如果输出为空，添加提示信息
	if output == "" {
		output = "（构建命令执行成功，但无输出）"
	}

	output = fmt.Sprintf("🔨 构建成功\n命令: %s\n输出:\n%s", userCmd, output)
	return
}