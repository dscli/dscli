package main

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_format_patch",
		Description: "生成指定Git提交的patch格式描述（RFC 2822标准格式）。patch包含完整的提交信息、作者、日期和代码差异，可用于代码审查、变更记录或通过`git apply`应用补丁。默认生成当前HEAD提交的patch。",
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
//	commit: 指定commit哈希或-n格式（如-1表示最新提交），默认为"-1"
//	stdout: 是否输出到标准输出，默认为true（实际总是输出到stdout）
func handleGitFormatPatch(ctx context.Context, args map[string]string) (string, error) {
	// 获取revision参数，默认为"-1"（最新提交）
	revision := args["revision"]

	// 构建git format-patch命令参数
	gitArgs := []string{"format-patch", "-1", "--stdout"}
	if revision != "" {
		gitArgs = append(gitArgs, revision)
	}

	// 输出执行的命令
	Println("git", strings.Join(gitArgs, " "))

	// 执行git命令
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", fmt.Errorf("git format-patch failed: %w", err)
	}

	// 如果输出为空，返回提示信息
	if out == "" {
		return "git format-patch succeed without output (maybe no changes?)", nil
	}

	return out, nil
}
