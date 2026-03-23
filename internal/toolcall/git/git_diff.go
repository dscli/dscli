package git

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_diff",
		Description: "查看文件或暂存区的差异",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录），多个文件用空格分隔, 长度1-1024字符",
					"pattern":     TitleLikePattern(1024),
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitDiff,
	})
}

// handleGitDiff git差异
func handleGitDiff(ctx context.Context, args ToolArgs) (string, error) {
	path := ToolArgsValue(args, "path", "")
	path = strings.TrimSpace(path)

	// 显示操作标题
	PrintGitSection("文件差异")

	names := strings.Fields(path)
	gitArgs := []string{"diff", "--no-ext-diff"}

	// 检查是否有暂存的文件
	statusOut, err := gitCommand(ctx, "status", "--short")
	if err != nil {
		return "", err
	}

	// 分析状态：是否有已暂存的文件？
	hasStagedChanges, hasUnstagedChanges := hasStagedUnstagedChanges(statusOut)

	// 智能选择diff模式
	if hasStagedChanges && !hasUnstagedChanges {
		// 只有暂存的文件，没有工作区修改
		outfmt.Info("检测到只有暂存的文件，使用 --cached 查看暂存区与HEAD的差异")
		gitArgs = append(gitArgs, "--cached")
	} else if hasStagedChanges && hasUnstagedChanges {
		// 既有暂存的文件，又有工作区修改
		outfmt.Info("检测到既有暂存文件又有工作区修改")
		outfmt.Info("默认显示工作区与暂存区的差异")
		outfmt.Notice("使用 --cached 查看暂存区与HEAD的差异")
	} else if !hasStagedChanges && hasUnstagedChanges {
		// 只有工作区修改，没有暂存文件
		outfmt.Info("检测到只有工作区修改，显示工作区与HEAD的差异")
	}

	// 显示要比较的文件
	if len(names) > 0 {
		outfmt.Info("要比较的文件:")
		for i, name := range names {
			outfmt.PrintBullet(fmt.Sprintf("[%d] %s", i+1, name))
		}
		gitArgs = append(gitArgs, names...)
	} else {
		outfmt.Info("比较所有变更的文件")
	}

	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}

	// 格式化输出
	if out == "" {
		outfmt.Success("没有差异")
		return "没有差异", nil
	}

	// 解析差异输出
	lines := strings.Split(strings.TrimSpace(out), "\n")

	outfmt.PrintSubSection("差异详情")

	// 使用Markdown格式显示差异
	outfmt.Println("```diff")

	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// 根据差异类型着色（同时保持diff语法）
		var coloredLine string
		switch {
		case strings.HasPrefix(line, "diff --git"):
			// 文件差异标题
			coloredLine = outfmt.Colorize(outfmt.ColorBoldCyan, line)
		case strings.HasPrefix(line, "index "):
			// 索引信息
			coloredLine = outfmt.Colorize(outfmt.ColorGray, line)
		case strings.HasPrefix(line, "---"):
			// 旧文件
			coloredLine = outfmt.Colorize(outfmt.ColorRed, line)
		case strings.HasPrefix(line, "+++"):
			// 新文件
			coloredLine = outfmt.Colorize(outfmt.ColorGreen, line)
		case strings.HasPrefix(line, "@@"):
			// 差异块标题
			coloredLine = outfmt.Colorize(outfmt.ColorBoldBlue, line)
		case strings.HasPrefix(line, "+"):
			// 新增行
			coloredLine = outfmt.Colorize(outfmt.ColorGreen, line)
		case strings.HasPrefix(line, "-"):
			// 删除行
			coloredLine = outfmt.Colorize(outfmt.ColorRed, line)
		default:
			// 上下文行
			coloredLine = outfmt.Colorize(outfmt.ColorWhite, line)
		}

		outfmt.Println(coloredLine)
	}

	outfmt.Println("```")
	// 显示统计信息
	outfmt.PrintSubSection("统计信息")
	diffStats := analyzeDiffStats(out)
	if diffStats.files > 0 {
		outfmt.Info("共比较 %d 个文件", diffStats.files)
		outfmt.Success("新增行: %d", diffStats.additions)
		outfmt.Error("删除行: %d", diffStats.deletions)
		outfmt.Notice("变更行总计: %d", diffStats.additions+diffStats.deletions)
	}

	return out, nil
}

func hasStagedUnstagedChanges(statusOut string) (hasStagedChanges bool, hasUnstagedChanges bool) {
	lines := strings.Split(statusOut, "\n")
	for _, line := range lines {
		if len(line) >= 2 {
			// 第一个字符表示暂存区状态
			stagedStatus := line[0]
			// 第二个字符表示工作区状态
			unstagedStatus := line[1]

			if stagedStatus != ' ' && stagedStatus != '?' {
				hasStagedChanges = true
			}
			if unstagedStatus != ' ' && unstagedStatus != '?' {
				hasUnstagedChanges = true
			}
		}
	}
	return
}

// diffStats 差异统计
type diffStats struct {
	files     int
	additions int
	deletions int
}

// analyzeDiffStats 分析差异统计
// analyzeDiffStats 分析差异统计
// analyzeDiffStats 分析差异统计
func analyzeDiffStats(diffOutput string) diffStats {
	stats := diffStats{}

	// 使用高效的SplitSeq迭代，避免分配切片
	for line := range strings.SplitSeq(diffOutput, "\n") {
		if strings.HasPrefix(line, "diff --git") {
			stats.files++
		} else if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			stats.additions++
		} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			stats.deletions++
		}
	}

	return stats
}
