package shell

import (
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	ishell "gitcode.com/dscli/dscli/internal/shell"
)

func init() { // 注册shell工具
	// 获取系统可用命令列表（init 阶段验证一次，后续复用）
	cmdsDesc := ishell.GetAvailableCommandsDescription(context.Background())

	desc := `在项目根目录执行Shell脚本。

输出格式：
- 成功时：返回包含执行结果和执行统计的格式化文本
- 失败时：返回包含错误信息、输出内容和执行统计的格式化文本

示例：
1. Bash脚本：echo "Hello"
2. Shell脚本：ls -la
3. 文件操作：cat file.txt
4. Git操作：git status

注意：谨慎使用，避免破坏性操作。确保脚本在项目目录内执行。`

	if cmdsDesc != "" {
		desc += "\n\n## 可用命令\n" + cmdsDesc
	}

	RegisterTool(ToolDef{
		Name:        "shell",
		Description: desc,
		Strict:      true,
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
别太长，40个字以内。

示例：
1. 查找包含Hello方法Go文件
2. 处理Json数据
3. 读文件
`,
				},
			},
			"required":             []string{"script", "summary"},
			"additionalProperties": false,
		},

		Category: "system",
		Timeout:  60 * time.Second, // 设置60秒超时
		Handler:  handleShell,
	})
}

// handleShell 执行Shell脚本，使用 internal/shell 解释器
func handleShell(ctx context.Context, args ToolArgs) (out string, user string, err error) {
	script := ToolArgsValue(args, "script", "")
	summary := ToolArgsValue(args, "summary", "")
	if summary == "" {
		summary = "(shell script)"
	}
	// 将 summary 注入 context，供 executor 作脚本名使用（语法错误消息中显示）
	ctx = context.WithValue(ctx, context.ShellSummaryKey, summary)
	outfmt.Notice("💻 执行Shell%s", TruncateString(summary, 100))

	// 使用 internal/shell 解释器执行
	config := ishell.DefaultConfig(ctx)
	executor := ishell.NewExecutor(ctx, config)
	result, execErr := executor.Execute(ctx, script)

	if execErr != nil {
		err = fmt.Errorf("shell executor error: %w", execErr)
		return
	}
	if result == nil {
		err = fmt.Errorf("shell executor returned nil result without error")
		return
	}

	// stderr → user (Suggestion)，供 AI 参考诊断信息
	user = result.Stderr
	out = result.Stdout
	if result.Err != nil {
		err = fmt.Errorf("shell script failed (exit=%d): %w", result.ExitCode, result.Err)
		return
	}
	return
}
