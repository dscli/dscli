package main

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func init() {
	RegisterTool(ToolDef{
		Name: "git_am",
		Description: `应用通过git format-patch生成的patch文件。
支持将patch内容通过标准输入传递给git am命令。

主要功能：
1. 应用patch：将patch内容应用到当前分支
2. 错误处理：支持--continue、--skip、--abort等恢复选项
3. 简单易用：只需提供patch内容即可

参数说明：
- patch: patch内容（必需），通过git format-patch生成的RFC 2822格式patch
- options: git am选项（可选），如--continue、--skip、--abort、--quit、--show-current-patch

使用示例：
1. 应用patch：git_am(patch="从patch内容...")
2. 继续应用：git_am(options="--continue")
3. 放弃应用：git_am(options="--abort")

注意：patch内容较长时建议通过标准输入传递，避免命令行长度限制。`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"patch": map[string]any{
					"type":        "string",
					"description": "patch内容（RFC 2822格式），通过git format-patch生成",
				},
				"options": map[string]any{
					"type": "string",
					"description": `git am选项，支持：
1. 应用选项：--signoff、--keep、--3way等（默认无选项）
2. 恢复选项：--continue、--skip、--abort、--quit、--show-current-patch
多个选项用空格分隔，例如：--signoff --3way`,
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Timeout:  120 * time.Second, // git am可能需要较长时间
		Handler:  handleGitAm,
	})
}

// handleGitAm 处理git am命令
// handleGitAm 处理git am命令
// handleGitAm 处理git am命令（apply patch from mail）
// git am 全称是 "apply patch from mail"，用于应用邮件格式的patch
// handleGitAm 处理git am命令
// handleGitAm 处理git am命令
// handleGitAm 处理git am命令（apply patch from mail）
// git am 全称是 "apply patch from mail"，用于应用邮件格式的patch
func handleGitAm(ctx context.Context, args map[string]string) (string, error) {
	patch, hasPatch := args["patch"]
	options, hasOptions := args["options"]

	// 显示操作标题
	PrintGitSection("应用Patch")

	// 如果没有提供patch和options，返回错误
	if !hasPatch && !hasOptions {
		Error("必须提供patch内容或options参数")
		return "", fmt.Errorf("必须提供patch内容或options参数")
	}

	// 构建git am命令
	gitArgs := []string{"am"}

	// 添加选项
	if hasOptions && options != "" {
		optionList := strings.Fields(options)
		gitArgs = append(gitArgs, optionList...)
		Info("应用选项: %s", options)
	}

	// 如果提供了patch内容，通过标准输入传递
	if hasPatch && patch != "" {
		// 显示patch信息
		PrintSubSection("Patch信息")

		// 解析patch头部信息
		lines := strings.SplitN(patch, "\n", 10)
		for i := 0; i < min(8, len(lines)); i++ {
			line := lines[i]
			if strings.HasPrefix(line, "From ") {
				Info("提交: %s", strings.TrimSpace(line[5:]))
			} else if strings.HasPrefix(line, "Date: ") {
				Info("日期: %s", strings.TrimSpace(line[6:]))
			} else if strings.HasPrefix(line, "Subject: ") {
				Info("主题: %s", strings.TrimSpace(line[9:]))
			}
		}

		// 分析patch统计
		PrintSubSection("Patch统计")
		diffStats := analyzeDiffStats(patch)
		Info("Patch包含 %d 个文件", diffStats.files)
		Success("新增行: %d", diffStats.additions)
		Error("删除行: %d", diffStats.deletions)
		Notice("变更行总计: %d", diffStats.additions+diffStats.deletions)

		Info("正在应用patch...")

		// 设置context值，指定使用git作为解释器
		ctx = context.WithValue(ctx, ShellName, "git")
		ctx = context.WithValue(ctx, ShellArgs, gitArgs)

		PrintGitCommand(gitArgs...)
		return ShellExec(ctx, patch)
	} else {
		// 如果没有patch内容，直接执行git am命令（用于--continue等操作）
		Info("执行git am命令: %s", strings.Join(gitArgs, " "))
		PrintGitCommand(gitArgs...)
		return gitCommand(ctx, gitArgs...)
	}
}
