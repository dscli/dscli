package main

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_log",
		Description: "查看提交历史",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_count": map[string]any{
					"type":        "string",
					"description": `最大显示数量，默认"10"`,
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
// handleGitLog git日志
func handleGitLog(ctx context.Context, args map[string]string) (string, error) {
	maxCount, ok := args["max_count"]
	if !ok {
		maxCount = "10"
	}

	// 显示操作标题
	PrintGitSection("提交历史")

	Info("显示最近 %s 条提交记录", maxCount)

	gitArgs := []string{"log", "--oneline", "--graph", "--decorate", fmt.Sprintf("-%s", maxCount)}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}

	// 格式化输出
	if out == "" {
		Warn("没有提交记录")
		return "没有提交记录", nil
	}

	// 解析提交记录
	lines := strings.Split(strings.TrimSpace(out), "\n")

	PrintSubSection("提交历史")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 为每行添加序号
		lineNum := fmt.Sprintf("[%d]", i+1)
		fmt.Fprintf(outputWriter, "%s %s\n", colorize(ColorBoldCyan, lineNum), line)
	}

	// 显示统计信息
	PrintSubSection("统计信息")
	Info("共显示 %d 条提交记录", len(lines))

	return out, nil
}
