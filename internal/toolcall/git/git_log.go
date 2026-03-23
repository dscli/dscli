package git

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_log",
		Description: "查看提交历史",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_count": map[string]any{
					"type":        "integer",
					"description": `最大显示数量，默认10`,
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitLog,
	})
}

// handleGitLog git日志
func handleGitLog(ctx context.Context, args ToolArgs) (string, error) {
	maxCount := ToolArgsValue(args, "max_count", 10)

	// 显示操作标题
	PrintGitSection("提交历史")

	outfmt.Info("显示最近 %d 条提交记录", maxCount)

	gitArgs := []string{"log", "--oneline", "--graph", "--decorate", fmt.Sprintf("-%d", maxCount)}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}

	// 格式化输出
	if out == "" {
		outfmt.Warn("没有提交记录")
		return "没有提交记录", nil
	}

	// 解析提交记录
	lines := strings.Split(strings.TrimSpace(out), "\n")

	outfmt.PrintSubSection("提交历史")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 为每行添加序号
		lineNum := fmt.Sprintf("[%d]", i+1)
		outfmt.Printf("%s %s\n", outfmt.Colorize(outfmt.ColorBoldCyan, lineNum), line)
	}

	// 显示统计信息
	outfmt.PrintSubSection("统计信息")
	outfmt.Info("共显示 %d 条提交记录", len(lines))

	return out, nil
}
