package main

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_status",
		Description: "查看 Git 仓库状态",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitStatus,
	})
}

// handleGitStatus git状态
func handleGitStatus(ctx context.Context, args ToolArgs) (string, error) {
	// 显示操作标题
	PrintGitSection("仓库状态")

	outfmt.Info("正在检查Git仓库状态...")

	out, err := gitCommand(ctx, "status", "--short")
	if err != nil {
		return "", err
	}

	// 格式化输出
	if out == "" || strings.Contains(out, "工作区干净，无变更") {
		outfmt.Success("工作区干净，无变更")
		return "工作区干净，无变更", nil
	}

	// 解析状态输出
	lines := strings.Split(strings.TrimSpace(out), "\n")

	// 统计不同类型的变更
	var staged, unstaged, untracked []string
	for _, line := range lines {
		if len(line) >= 3 {
			status := strings.TrimSpace(line[:2])
			file := strings.TrimSpace(line[3:])

			switch status {
			case "A", "M", "D", "R", "C":
				// 暂存区的变更
				staged = append(staged, fmt.Sprintf("%s %s", status, file))
			case "??":
				// 未跟踪的文件
				untracked = append(untracked, file)
			default:
				// 工作区的变更（修改但未暂存）
				if strings.Contains(status, "M") || strings.Contains(status, "D") {
					unstaged = append(unstaged, fmt.Sprintf("%s %s", status, file))
				}
			}
		}
	}

	// 显示统计信息
	outfmt.PrintSubSection("变更统计")
	if len(staged) > 0 {
		outfmt.Success("暂存区变更 (%d 个文件):", len(staged))
		for _, file := range staged {
			outfmt.PrintBullet(file)
		}
	}

	if len(unstaged) > 0 {
		outfmt.Warn("工作区变更 (%d 个文件):", len(unstaged))
		for _, file := range unstaged {
			outfmt.PrintBullet(file)
		}
	}

	if len(untracked) > 0 {
		outfmt.Notice("未跟踪文件 (%d 个):", len(untracked))
		for _, file := range untracked {
			outfmt.PrintBullet(file)
		}
	}

	// 显示原始输出
	outfmt.PrintSubSection("原始输出")
	outfmt.Println(out)

	return out, nil
}
