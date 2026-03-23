package make

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	// 注册测试运行工具
	RegisterTool(ToolDef{
		Name: "make_test",
		Description: `运行项目测试，检查测试是否通过。

参数：
  command: 必需，测试命令

功能：
1. 执行测试命令运行项目测试
2. 返回测试输出，包括测试结果和失败信息

示例：
  make_test(command="make test")
  
  # 使用自定义测试命令
  make_test(command="go test ./... -v")`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "测试命令",
					"pattern":     TitleLikePattern(128),
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
		Category: "test_ops",
		Handler:  handleMakeTest,
	})
}

// handleMakeTest 处理测试运行请求
func handleMakeTest(ctx context.Context, args ToolArgs) (output string, err error) {
	// 检查是否提供了自定义命令
	userCmd := ToolArgsValue(args, "command", "")

	if userCmd == "" {
		err = fmt.Errorf("no command provided")
		return
	}

	// 记录使用的命令
	outfmt.Printf("🧪 执行测试命令: %s", userCmd)

	// 执行测试命令
	ctx = context.WithValue(ctx, context.ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, userCmd)
	if err != nil {
		err = fmt.Errorf("测试失败: %w\n命令: %s\n输出:\n%s", err, userCmd, output)
	}

	// 如果输出为空，添加提示信息
	if output == "" {
		output = "（测试命令执行成功，但无输出）"
	}

	output = fmt.Sprintf("🧪 测试完成\n命令: %s\n输出:\n%s", userCmd, output)
	return
}