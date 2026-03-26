package git

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_add",
		Description: "将文件添加到 Git 暂存区",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录），多个文件用空格分隔",
					"pattern":     TitleLikePattern(128),
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitAdd,
	})
}

// handleGitAdd git添加
func handleGitAdd(ctx context.Context, args ToolArgs) (result string, user string, err error) {
	path := ToolArgsValue(args, "path", "")
	path = strings.TrimSpace(path)

	// 显示操作标题
	PrintGitSection("添加文件到暂存区")

	names := strings.Fields(path)
	gitArgs := []string{"add"}
	gitArgs = append(gitArgs, names...)

	// 显示要添加的文件
	if len(names) > 0 {
		outfmt.Info("要添加的文件:")
		for i, name := range names {
			outfmt.PrintBullet(fmt.Sprintf("[%d] %s", i+1, name))
		}
	} else {
		outfmt.Warn("未指定要添加的文件路径")
		err = fmt.Errorf("必须指定要添加的文件路径")
		return
	}

	result, err = gitCommand(ctx, gitArgs...)
	if err != nil {
		err = fmt.Errorf("failed to run git command: %w", err)
		return
	}

	// 如果输出为空，显示成功消息
	if result == "" || strings.Contains(result, "命令执行成功（无输出）") {
		if len(names) == 1 {
			outfmt.Success("文件 %s 已成功添加到暂存区", names[0])
		} else {
			outfmt.Success("%d 个文件已成功添加到暂存区", len(names))
		}
		result = "文件已添加到暂存区"
		return
	}

	return
}
