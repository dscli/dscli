package shell

import (
	_ "embed"
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	ishell "gitcode.com/dscli/dscli/internal/shell"
)

//go:embed shell_shell.md
var shell_shell_md string

func init() { // 注册shell工具
	// 获取系统可用命令列表（init 阶段验证一次，后续复用）
	cmdsDesc := ishell.GetAvailableCommandsDescription(context.Background())

	desc := shell_shell_md
	if cmdsDesc != "" {
		desc += "\n\n## Available Commands\n"
		desc += cmdsDesc
	}

	RegisterTool(ToolDef{
		Name:        "shell",
		Description: desc,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type":        "string",
					"description": "Shell script content to execute. Results returned as formatted text with execution stats.",
				},
				"summary": map[string]any{
					"type":        "string",
					"description": "Brief summary of what the script does, max 40 chars. E.g. 'List Go files', 'Parse JSON', 'Read file'.",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (default 120). Set a value to override the default — use a short timeout for quick commands, or a long timeout (e.g. 1200) for lengthy tasks like running tests.",
				},
			},
			"required":             []string{"script", "summary"},
			"additionalProperties": false,
		},

		Category: "system",
		Timeout:  120 * time.Second, // 设置120秒超时
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

	// stderr → warning (yellow), 供 AI 参考诊断信息
	warning = res.Stderr
	result = res.Stdout
	if res.Err != nil {
		// Include stderr in the error so the LLM can diagnose the failure.
		// res.Err alone is just "exit status N" — useless without stderr.
		if res.Stderr != "" {
			err = fmt.Errorf("shell script failed (exit=%d): %s", res.ExitCode, res.Stderr)
		} else {
			err = fmt.Errorf("shell script failed (exit=%d): %w", res.ExitCode, res.Err)
		}
		return result, warning, err
	}
	return result, warning, err
}
