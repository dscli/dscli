package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/editor"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
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
		Short: "Remove a prompt",
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
		return fmt.Errorf("确定提示词文件路径失败: %w", err)
	}

	// 若文件不存在，从更高作用域获取种子内容；完全新建时内容为空
	if _, err := os.Stat(p); os.IsNotExist(err) {
		seed := prompt.GetPromptSourceContent(name, global)
		if err := os.WriteFile(p, []byte(seed+"\n"), 0o644); err != nil {
			return fmt.Errorf("创建提示词文件 %s 失败: %w", p, err)
		}
	} else if err != nil {
		return fmt.Errorf("访问提示词文件 %s 失败: %w", p, err)
	}

	if err := editor.Edit(cmd.Context(), p); err != nil {
		return fmt.Errorf("编辑器退出错误: %w", err)
	}
	return nil
}

// promptRemoveRunE 删除提示词
func promptRemoveRunE(cmd *cobra.Command, args []string) error {
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
		return fmt.Errorf("确定提示词文件路径失败: %w", err)
	}

	if _, err := os.Stat(p); os.IsNotExist(err) {
		return fmt.Errorf("提示词 %s 不存在", name)
	} else if err != nil {
		return fmt.Errorf("访问提示词文件失败: %w", err)
	}

	if err := os.Remove(p); err != nil {
		return fmt.Errorf("删除失败: %w", err)
	}
	outfmt.Printf("已删除: %s\n", p)
	return nil
}

// promptAddRunE 从标准输入读取内容创建提示词
func promptAddRunE(cmd *cobra.Command, args []string) error {
	name, err := promptName(args)
	if err != nil {
		return err
	}
	global, _ := cmd.Flags().GetBool("global")

	// 读取 stdin
	input, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("读取标准输入失败: %w", err)
	}
	content := strings.TrimSpace(string(input))
	if content == "" {
		return fmt.Errorf("输入内容为空")
	}

	// 确定目标路径：项目优先，--global 强制全局
	var p string
	if global {
		p, err = prompt.GetPromptPath(name, true)
	} else if context.ProjectRoot != "" {
		p, err = prompt.GetPromptPath(name, false)
	} else {
		p, err = prompt.GetPromptPath(name, true)
	}
	if err != nil {
		return fmt.Errorf("确定提示词文件路径失败: %w", err)
	}

	if err := os.WriteFile(p, []byte(content+"\n"), 0o644); err != nil {
		return fmt.Errorf("写入提示词文件失败: %w", err)
	}
	outfmt.Printf("已添加: %s\n", p)
	return nil
}
