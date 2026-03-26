package make

import (
	"fmt"
	"os"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	// 注册代码格式化工具
	RegisterTool(ToolDef{
		Name: "code_format",
		Description: `运行代码格式化命令，格式化项目代码。

参数：
  command: 必需，格式化命令

功能：
1. 执行代码格式化命令，格式化项目代码
2. 支持自定义格式化命令
3. 返回格式化输出，包括错误信息

示例：
  # 使用自定义格式化命令
  code_format(command="go fmt ./...")`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "格式化命令",
					"pattern":     TitleLikePattern(128),
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleCodeFormat,
	})
}

// handleCodeFormat 处理代码格式化请求
func handleCodeFormat(ctx context.Context, args ToolArgs) (output string, user string, err error) {
	// 检查是否提供了自定义命令
	command := ToolArgsValue(args, "command", "")

	if command == "" {
		err = fmt.Errorf("no format command specified")
		return
	}
	outfmt.Printf("代码格式化 %s", command)
	// 使用用户提供的命令
	ctx = context.WithValue(ctx, context.ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, command)
	if err != nil {
		// 构建详细的错误信息
		var sb strings.Builder
		fmt.Fprintf(&sb, "❌ 代码格式化失败\n")
		fmt.Fprintf(&sb, "📝 使用的命令: %s\n", command)
		fmt.Fprintf(&sb, "💥 错误信息: %v\n", err)
		fmt.Fprintf(&sb, "📄 输出内容:\n%s", output)
		output = sb.String()
		err = fmt.Errorf("代码格式化失败")
		return
	}

	// 构建成功信息
	var sb strings.Builder
	fmt.Fprintf(&sb, "✅ 代码格式化完成\n")
	fmt.Fprintf(&sb, "📝 使用的命令: %s\n", command)
	fmt.Fprintf(&sb, "📄 输出内容:\n%s", output)
	fmt.Fprintf(&sb, "\n💡 建议:\n")
	fmt.Fprintf(&sb, "1. 使用 make_build() 确保格式化后代码仍然可以编译\n")
	fmt.Fprintf(&sb, "2. 使用 git diff 查看格式化后的变更\n")
	fmt.Fprintf(&sb, "3. 使用 make_test() 确保格式化后测试仍然通过\n")

	output = sb.String()
	return
}
