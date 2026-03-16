package main

import (
	"context"
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
				"summary": map[string]any{
					"type": "string",
					"description": `要执行的Shell脚本要做什么的总结。
别太长，40个字以内。可选，脚本很短（比如40个字以内）可以不加。

示例：
1. 查找包含Hello方法Go文件
2. 处理Json数据
3. 读文件
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
func handleShell(ctx context.Context, args ToolArgs) (out string, err error) {
	script := ToolArgsValue(args, "script", "")

	if err = validateShell(script); err != nil {
		return
	}
	summary := ToolArgsValue(args, "summary", "")
	if summary == "" {
		summary = "\n```bash\n" + script + "\n```\n"
	}
	Notice("💻 执行Shell%s", summary)
	out, err = runShell(ctx, script)
	return
}

func runShell(ctx context.Context, script string) (result string, err error) {
	startTime := time.Now()
	name, arg := Shebang(script)
	ctx = context.WithValue(ctx, ShellNameKey, name)
	ctx = context.WithValue(ctx, ShellArgsKey, arg)
	out, err := ShellExec(ctx, script)
	executionTime := time.Since(startTime)

	// 记录调试信息（不在用户输出中显示）
	Debug("Shell命令执行时间: %v", executionTime)

	// 判断是Python还是Shell调用
	isPython := strings.Contains(strings.ToLower(script), "python") ||
		strings.Contains(strings.ToLower(name), "python")

	if err != nil {
		// 简化错误输出，不显示执行时间
		if isPython {
			result = fmt.Sprintf("🐍 Python执行失败:\n错误: %v\n\n输出内容:\n%s", err, out)
		} else {
			result = fmt.Sprintf("💻 Shell执行失败:\n错误: %v\n\n输出内容:\n%s", err, out)
		}
		return result, nil
	}

	// 简化成功输出，不显示执行时间
	if isPython {
		result = fmt.Sprintf("🐍 Python执行结果:\n%s", out)
	} else {
		result = fmt.Sprintf("💻 Shell执行结果:\n%s", out)
	}

	return
}
