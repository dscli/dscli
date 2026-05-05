package main

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/editor"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"github.com/spf13/cobra"
)

func init() {
	promptCmd := AddRootCommand(&cobra.Command{
		Use: "prompt",
	})

	_ = AddCommand(promptCmd, &cobra.Command{
		Use:   "list",
		Short: "List available prompts",
		RunE:  promptListRunE,
	})

	_ = AddCommand(promptCmd, &cobra.Command{
		Use:   "show <name>",
		Short: "Show prompt content (args[0]: prompt name, default: dev)",
		RunE:  promptShowRunE,
	})

	editCmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit prompt (args[0]: prompt name, default: dev)",
		RunE:  promptEditRunE,
	}
	editCmd.Flags().Bool("global", false, "Edit global prompt")
	_ = AddCommand(promptCmd, editCmd)

	removeCmd := &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a prompt (args[0]: prompt name, default: dev)",
		RunE:  promptRemoveRunE,
	}
	removeCmd.Flags().Bool("global", false, "Remove global prompt")
	_ = AddCommand(promptCmd, removeCmd)
}

// promptName 从 args 获取提示词名称，默认 "dev"
func promptName(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return "dev"
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
	name := promptName(args)
	content := prompt.GetPromptTemplate(cmd.Context(), name)
	outfmt.Println(content)
	return nil
}

// promptEditRunE 编辑提示词
func promptEditRunE(cmd *cobra.Command, args []string) error {
	name := promptName(args)
	global, _ := cmd.Flags().GetBool("global")

	var p string
	var err error
	if global {
		p, err = prompt.GetPromptPath(name, true)
	} else {
		p, err = prompt.ResolvePromptEditPath(name)
	}
	if err != nil {
		return fmt.Errorf("确定提示词文件路径失败: %w", err)
	}

	// 若文件不存在，创建空文件；若存在则继续编辑
	if _, err := os.Stat(p); os.IsNotExist(err) {
		if err := os.WriteFile(p, []byte{}, 0o644); err != nil {
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
	name := promptName(args)
	global, _ := cmd.Flags().GetBool("global")

	var p string
	var err error
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