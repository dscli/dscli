package main

import (
	"context"
	"fmt"
	"os"
)

// CodeMakeTest - run test command provided in the context
// 注意：这里设置ShellStdinKey为os.Stdin是有意的，目的是让测试命令在OSExec中执行，
// 而不是在internal/shell沙箱中执行。因为测试脚本由用户提供，是可信任的，
// 可能包含沙箱不允许的命令，但由于用户指定，是安全的。
func CodeMakeTest(ctx context.Context) (output string, err error) {
	testCmd := ContextValue(ctx, MakeTestKey, "make test")
	ctx = context.WithValue(ctx, ShellStdinKey, os.Stdin)
	output, err = ShellExec(ctx, testCmd)
	if err != nil {
		err = fmt.Errorf("failed to make test: %w", err)
	}
	return
}

func init() {
	// 注册测试运行工具
	RegisterTool(ToolDef{
		Name: "make_test",
		Description: `运行项目测试，检查测试是否通过。

参数：
  command: 可选，测试命令。如果不提供，则使用上下文中的配置命令（默认为"make test"）
  test_pattern: 可选，测试模式。用于筛选要运行的测试（如果测试命令支持）

功能：
1. 执行测试命令运行项目测试
2. 返回测试输出，包括测试结果和失败信息
3. 支持测试模式筛选（如果底层测试命令支持）

示例：
  # 使用默认测试命令
  make_test()
  
  # 使用自定义测试命令
  make_test(command="go test ./... -v")
  
  # 使用测试模式筛选
  make_test(command="go test ./... -v -run TestUser")`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "测试命令，可选，如果不提供则使用配置的默认命令",
				},
				"test_pattern": map[string]any{
					"type":        "string",
					"description": "测试模式，用于筛选要运行的测试（如果测试命令支持）",
				},
			},
			"additionalProperties": false,
		},
		Category: "test_ops",
		Handler:  handleMakeTest,
	})
}

// handleMakeTest 处理测试运行请求
// handleMakeTest 处理测试运行请求
func handleMakeTest(ctx context.Context, args ToolArgs) (string, error) {
	// 检查是否提供了自定义命令
	userCmd := ToolArgsValue(args, "command", "")
	testPattern := ToolArgsValue(args, "test_pattern", "")

	var finalCmd string
	if userCmd != "" {
		finalCmd = userCmd
	} else {
		// 使用配置的测试命令
		finalCmd = ContextValue(ctx, MakeTestKey, "make test")
	}

	// 如果提供了测试模式，尝试添加到命令中
	// 注意：这取决于具体的测试命令是否支持模式参数
	if testPattern != "" {
		// 简单处理：如果是go test命令，添加-run参数
		// 对于其他测试命令，可能需要不同的处理方式
		finalCmd = fmt.Sprintf("%s -run %s", finalCmd, testPattern)
	}

	// 记录使用的命令
	Printf("执行测试命令: %s", finalCmd)

	// 执行测试命令
	ctx = context.WithValue(ctx, ShellStdinKey, os.Stdin)
	output, err := ShellExec(ctx, finalCmd)
	if err != nil {
		return "", fmt.Errorf("测试失败: %w\n命令: %s\n输出:\n%s", err, finalCmd, output)
	}

	// 如果输出为空，添加提示信息
	if output == "" {
		output = "（测试命令执行成功，但无输出）"
	}

	return fmt.Sprintf("✅ 测试完成\n命令: %s\n输出:\n%s", finalCmd, output), nil
}
