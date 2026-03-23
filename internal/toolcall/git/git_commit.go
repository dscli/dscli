package git

import (
	"fmt"
	"os/exec"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "git_commit",
		Description: "提交暂存区更改，需要提供提交信息。注意：不要在options中包含-m或--message参数。",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "提交信息（不要在options中包含-m或--message参数）,长度1-1024字符",
					"pattern":     ContentLikePattern(1024),
				},
				"options": map[string]any{
					"type": "string",
					"description": `其他git commit选项，例如：-a（提交所有更改）、
--amend（修改上次提交）、--no-edit（使用原提交信息）、
--allow-empty（允许空提交）。
多个选项用空格分隔，例如：-a --amend --no-edit`,
				},
			},
			"required":             []string{"message"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitCommit,
	})
}

// handleGitCommit git提交
func handleGitCommit(ctx context.Context, args ToolArgs) (string, error) {
	message, ok := args["message"].(string)
	if !ok || message == "" {
		return "", fmt.Errorf("必须提供提交信息")
	}

	options, _ := args["options"].(string)
	options = strings.TrimSpace(options)

	// 更健壮的-m参数检查
	// 检查 -m、-m[空格]、--message 等变体
	for word := range strings.FieldsSeq(options) {
		if word == "-m" || word == "--message" || strings.HasPrefix(word, "-m") {
			outfmt.Error("检测到-m或--message参数")
			outfmt.Warn("提示: message参数已通过message字段提供，不要在options中包含-m或--message")
			return "", fmt.Errorf("message参数已通过message字段提供，不要在options中包含-m或--message")
		}
	}

	gitArgs := []string{"commit", "-m", message}
	if options != "" {
		gitArgs = append(gitArgs, strings.Fields(options)...)
	}

	outfmt.Info("执行: git %s", strings.Join(gitArgs, " "))
	outfmt.Info("执行: git %s", strings.Join(gitArgs, " "))

	cmd := exec.Command("git", gitArgs...)
	cmd.Dir = context.ProjectRoot
	output, err := cmd.CombinedOutput()
	out := string(output)

	if err != nil {
		outfmt.Error("Git提交失败: %v", err)
		if out != "" {
			outfmt.Error("输出: %s", out)
		}
		return "", fmt.Errorf("git commit失败: %v", err)
	}

	// 提取提交哈希（如果可能）
	if strings.Contains(out, "[") && strings.Contains(out, "]") {
		for line := range strings.SplitSeq(out, "\n") {
			if strings.Contains(line, "[") && strings.Contains(line, "]") {
				outfmt.Success("提交成功: %s", strings.TrimSpace(line))
				break
			}
		}
	} else {
		outfmt.Success("提交成功")
	}

	return out, nil
}
