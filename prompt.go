package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nanjj/clog"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/editor"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/session"
	"github.com/spf13/cobra"
)

func init() {
	promptCmd := AddRootCommand(&cobra.Command{
		Use:   "prompt",
		Short: "提示词管理",
	})

	_ = AddCommand(promptCmd, &cobra.Command{
		Use:   "list",
		Short: "List available prompts",
		RunE:  promptListRunE,
	})

	_ = AddCommand(promptCmd, &cobra.Command{
		Use:   "show <name>",
		Short: "Show prompt content",
		RunE:  promptShowRunE,
	})

	editCmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit prompt",
		RunE:  promptEditRunE,
	}
	editCmd.Flags().Bool("global", false, "Edit global prompt")
	_ = AddCommand(promptCmd, editCmd)

	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a prompt and fix dangling role references",
		RunE:  promptRemoveRunE,
	}
	removeCmd.Flags().Bool("global", false, "Remove global prompt")
	_ = AddCommand(promptCmd, removeCmd)

	addCmd := &cobra.Command{
		Use:   "add <name>",
		Short: "Add a prompt from stdin",
		RunE:  promptAddRunE,
	}
	addCmd.Flags().Bool("global", false, "Add global prompt")
	_ = AddCommand(promptCmd, addCmd)
}

// promptName 从 args 获取提示词名称，为空时返回错误
func promptName(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("需要指定提示词名称")
	}
	return args[0], nil
}

// promptListRunE 列出所有可用提示词
func promptListRunE(cmd *cobra.Command, args []string) error {
	span, _ := clog.StartSpanFromContext(cmd.Context(), "promptListRunE")
	defer span.Finish()
	infos := prompt.ListPrompts()
	if len(infos) == 0 {
		outfmt.Println("没有可用的提示词")
		return nil
	}
	for _, info := range infos {
		outfmt.Printf("%s\t%s\t%s\n", info.Name, info.Source, info.Description)
	}
	return nil
}

// promptShowRunE 显示提示词内容
func promptShowRunE(cmd *cobra.Command, args []string) error {
	span, _ := clog.StartSpanFromContext(cmd.Context(), "promptShowRunE")
	defer span.Finish()
	name, err := promptName(args)
	if err != nil {
		return err
	}
	content := prompt.GetPromptTemplate(cmd.Context(), name)
	outfmt.Println(content)
	return nil
}

// promptEditRunE 编辑提示词
// 若目标文件不存在，自动从更高作用域（全局/内建）拷贝内容作为编辑起点。
func promptEditRunE(cmd *cobra.Command, args []string) error {
	span, _ := clog.StartSpanFromContext(cmd.Context(), "promptEditRunE")
	defer span.Finish()
	name, err := promptName(args)
	if err != nil {
		return err
	}
	global, _ := cmd.Flags().GetBool("global")

	var p string
	if global {
		p, err = prompt.GetPromptPath(name, true)
	} else {
		p, err = prompt.ResolvePromptEditPath(name)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "确定提示词文件路径失败: %v\n", err)
		return nil
	}

	if _, err := os.Stat(p); os.IsNotExist(err) {
		seed := prompt.GetPromptSourceContent(name, global)
		if err := os.WriteFile(p, []byte(seed+"\n"), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "创建提示词文件 %s 失败: %v\n", p, err)
			return nil
		}
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "访问提示词文件 %s 失败: %v\n", p, err)
		return nil
	}

	if err := editor.Edit(cmd.Context(), p); err != nil {
		fmt.Fprintf(os.Stderr, "编辑器退出错误: %v\n", err)
		return nil
	}
	return nil
}

func promptRemoveRunE(cmd *cobra.Command, args []string) error {
	span, _ := clog.StartSpanFromContext(cmd.Context(), "promptRemoveRunE")
	defer span.Finish()
	name, err := promptName(args)
	if err != nil {
		return err
	}
	global, _ := cmd.Flags().GetBool("global")

	var p string
	if global {
		p, err = prompt.GetPromptPath(name, true)
	} else {
		p, err = prompt.ResolvePromptRemovePath(name)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "确定提示词文件路径失败: %v\n", err)
		return nil
	}

	if _, err := os.Stat(p); os.IsNotExist(err) {
		return fmt.Errorf("提示词 %s 不存在", name)
	} else if err != nil {
		fmt.Fprintf(os.Stderr, "访问提示词文件失败: %v\n", err)
		return nil
	}

	if err := os.Remove(p); err != nil {
		fmt.Fprintf(os.Stderr, "删除失败: %v\n", err)
		return nil
	}
	outfmt.Printf("已删除: %s\n", p)

	// Issue #24: 删除提示词后清理 role_configs 中的悬空引用。
	// 项目级提示词只影响当前会话；全局提示词（或项目文件不存在时
	// 回退删除的全局文件）影响所有会话。
	ctx := cmd.Context()
	// 未使用 --global 且解析出的路径位于当前项目目录下才算项目级删除。
	projectScope := !global && context.ProjectRoot != "" &&
		strings.HasPrefix(p, filepath.Join(context.ProjectRoot, ".dscli", "prompt"))
	var sessionID int64 // 0 表示扫描所有会话
	if projectScope {
		sessionID = session.GetCurrentSessionID(ctx)
	}
	refs, err := roles.ListRoleConfigsByPrompt(ctx, name, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "警告: 检查角色配置引用失败: %v\n", err)
		return nil
	}

	// 全局删除时，各引用行的项目可能仍有同名提示词文件，
	// 需要按会话判断引用是否依然有效。
	projectBySession := map[int64]string{}
	if !projectScope && len(refs) > 0 {
		projects, listErr := session.ListProjects(ctx)
		if listErr != nil {
			// 无法确定各项目的提示词文件时保守处理：不重置任何行，
			// 避免误删仍由项目提示词文件服务的有效配置。
			fmt.Fprintf(os.Stderr, "警告: 无法加载项目列表 (%v)，跳过角色配置清理\n", listErr)
			return nil
		}
		for _, pr := range projects {
			projectBySession[pr.ID] = pr.ProjectPath
		}
	}

	for _, cfg := range refs {
		// 另一作用域仍有同名提示词文件时引用依然有效，无需处理。
		if prompt.PromptFileExists(name, projectBySession[cfg.SessionID]) {
			continue
		}
		if cfg.Skills == "" || cfg.Tools == "" {
			// 污染特征：skills/tools 空串（INSERT 分支曾把未指定字段写成
			// 空串，见 PR #23）。角色已失去技能与工具，重置恢复默认行为，
			// 让一条 prompt remove 命令即可完成事故恢复。
			if err := roles.DeleteRoleConfig(ctx, cfg.Role, cfg.SessionID); err != nil {
				fmt.Fprintf(os.Stderr, "警告: 重置角色 %s 的配置失败: %v\n", cfg.Role, err)
				continue
			}
			outfmt.Printf("已重置角色 %s 的配置（引用已删除的提示词 %s 且 skills/tools 为空）\n", cfg.Role, name)
		} else {
			fmt.Fprintf(os.Stderr, "警告: 角色 %s 仍引用已删除的提示词 %s，将回退默认模板；如需清理请执行 dscli role reset %s\n", cfg.Role, name, cfg.Role)
		}
	}
	return nil
}

func promptAddRunE(cmd *cobra.Command, args []string) error {
	span, _ := clog.StartSpanFromContext(cmd.Context(), "promptAddRunE")
	defer span.Finish()
	name, err := promptName(args)
	if err != nil {
		return err
	}
	global, _ := cmd.Flags().GetBool("global")

	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取标准输入失败: %v\n", err)
		return nil
	}
	content := strings.TrimSpace(string(input))
	if content == "" {
		return fmt.Errorf("输入内容为空")
	}

	var p string
	if global {
		p, err = prompt.GetPromptPath(name, true)
	} else if context.ProjectRoot != "" {
		p, err = prompt.GetPromptPath(name, false)
	} else {
		p, err = prompt.GetPromptPath(name, true)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "确定提示词文件路径失败: %v\n", err)
		return nil
	}

	if err := os.WriteFile(p, []byte(content+"\n"), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "写入提示词文件失败: %v\n", err)
		return nil
	}
	outfmt.Printf("已添加: %s\n", p)
	return nil
}
