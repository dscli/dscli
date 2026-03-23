package git

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_format_patch",
		Description: "生成指定Git提交的patch格式描述（RFC 2822标准格式）。patch包含完整的提交信息、作者、日期和代码差异，可用于代码审查、变更记录或通过`git apply`应用补丁。默认生成当前HEAD提交的patch。",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"revision": map[string]any{
					"type": "string",
					"description": `Git revision标识符，支持多种格式：
1. commit ID（如5d5e1a6）
2. 分支名（如main、HEAD）
3. 相对引用（如HEAD~1、HEAD~2）
4. 标签名（如v1.0.0）
5. 空字符串：生成当前HEAD的patch
示例：'HEAD'、'5d5e1a6'、'HEAD~1'、''（空字符串）`,
				},
			},
			"required":             []string{"revision"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitFormatPatch,
	})
}

// handleGitFormatPatch 生成指定commit的patch格式描述
// 支持参数：
//
//	revision: 指定commit哈希或-n格式（如-1表示最新提交），默认为"-1"
func handleGitFormatPatch(ctx context.Context, args ToolArgs) (string, error) {
	// 获取revision参数，默认为"-1"（最新提交）
	revision := ToolArgsValue(args, "revision", "")

	// 显示操作标题
	PrintGitSection("生成Patch")

	// 显示要生成patch的提交
	if revision == "" {
		outfmt.Info("生成当前HEAD提交的patch")
	} else {
		outfmt.Info("生成提交 %s 的patch", revision)
	}

	// 构建git format-patch命令参数
	gitArgs := []string{"format-patch", "-1", "--stdout"}
	if revision != "" {
		gitArgs = append(gitArgs, revision)
	}

	// 执行git命令
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", fmt.Errorf("git format-patch failed: %w", err)
	}

	// 如果输出为空，返回提示信息
	if out == "" {
		outfmt.Warn("git format-patch成功但没有输出（可能没有变更？）")
		return "git format-patch succeed without output (maybe no changes?)", nil
	}

	// 解析patch内容
	lines := strings.Split(strings.TrimSpace(out), "\n")

	outfmt.PrintSubSection("Patch信息")

	// 提取patch头部信息
	var patchInfo []string
	for i, line := range lines {
		if i < 20 { // 只显示前20行作为摘要
			if strings.HasPrefix(line, "From ") {
				outfmt.Info("提交: %s", strings.TrimSpace(line[5:]))
			} else if strings.HasPrefix(line, "Date: ") {
				outfmt.Info("日期: %s", strings.TrimSpace(line[6:]))
			} else if strings.HasPrefix(line, "Subject: ") {
				outfmt.Info("主题: %s", strings.TrimSpace(line[9:]))
			} else if strings.HasPrefix(line, "diff --git") {
				break
			}
		}
		patchInfo = append(patchInfo, line)
	}

	// 显示patch统计
	outfmt.PrintSubSection("Patch统计")
	diffStats := analyzeDiffStats(out)
	outfmt.Info("Patch包含 %d 个文件", diffStats.files)
	outfmt.Success("新增行: %d", diffStats.additions)
	outfmt.Error("删除行: %d", diffStats.deletions)
	outfmt.Notice("变更行总计: %d", diffStats.additions+diffStats.deletions)

	// 显示完整的patch内容
	outfmt.PrintSubSection("完整Patch内容")
	return out, nil
}
