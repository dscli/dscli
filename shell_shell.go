package main

import (
	"context"
	"fmt"
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
func handleShell(ctx context.Context, args map[string]string) (out string, err error) {
	script, ok := args["script"]
	if !ok {
		script = ""
	}

	if err = validateShell(script); err != nil {
		return
	}

	Notice("Shell: %s", ShortenShellScript(script))
	out, err = runShell(ctx, script)
	return
}

func runShell(ctx context.Context, script string) (result string, err error) {
	startTime := time.Now()
	name, arg := Shebang(script)
	ctx = context.WithValue(ctx, ShellName, name)
	ctx = context.WithValue(ctx, ShellArgs, arg)
	out, err := ShellExec(ctx, script)
	executionTime := time.Since(startTime)

	if err != nil {
		// 构建包含执行统计的失败结果
		result := fmt.Sprintf("❌ 执行失败:\n错误: %v\n\n输出内容:\n%s\n\n📊 执行统计:\n执行时间: %v\n状态: 失败",
			err, out, executionTime)
		return result, nil
	}

	// 构建包含执行统计的成功结果
	result = fmt.Sprintf("📝 执行结果:\n%s\n\n📊 执行统计:\n执行时间: %v\n状态: 成功",
		out, executionTime)

	return
}
