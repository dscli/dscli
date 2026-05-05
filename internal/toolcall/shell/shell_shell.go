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

	desc := `Run shell scripts in the project root directory.

Output format:
- Success: formatted text with execution result and statistics
- Failure: formatted text with error info, output content, and execution statistics

Examples:
  1. Bash: echo "Hello"
  2. Shell: ls -la
  3. Files: cat file.txt
  4. Git: git status

Caution: Avoid destructive operations. Scripts run within the project directory.`

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
脚本执行结果会以格式化文本返回，包含执行统计信息。`,
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
func handleShell(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	script := ToolArgsValue(args, "script", "")
	summary := ToolArgsValue(args, "summary", "")
	if summary == "" {
		summary = "(shell script)"
	}
	// 将 summary 注入 context，供 executor 作脚本名使用（语法错误消息中显示）
	ctx = context.WithValue(ctx, context.ShellSummaryKey, summary)
	outfmt.Notice("📋 %s", summary)

	// 使用 internal/shell 解释器执行
	config := ishell.DefaultConfig(ctx)
	executor := ishell.NewExecutor(ctx, config)
	res, err := executor.Execute(ctx, script)
	if err != nil {
		err = fmt.Errorf("shell executor error: %w", err)
		return result, warning, err
	}

	if res == nil {
		err = fmt.Errorf("shell executor returned nil result without error")
		return result, warning, err
	}

	// stderr → user (Suggestion)，供 AI 参考诊断信息
	warning = res.Stderr
	result = res.Stdout
	if res.Err != nil {
		err = fmt.Errorf("shell script failed (exit=%d): %w", res.ExitCode, res.Err)
		return result, warning, err
	}
	return result, warning, err
}