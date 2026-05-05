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

	_ = AddCommand(promptCmd, &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit prompt (args[0]: prompt name, default: dev)",
		RunE:  promptEditRunE,
	})
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

	// 确定编辑目标路径
	p, err := prompt.ResolvePromptEditPath(name)
	if err != nil {
		return fmt.Errorf("确定提示词文件路径失败: %w", err)
	}

	// 若文件不存在，用默认内容创建
	if _, err := os.Stat(p); os.IsNotExist(err) {
		defaultContent := prompt.GetDefaultPromptTemplate(name)
		if err := os.WriteFile(p, []byte(defaultContent), 0o644); err != nil {
			return fmt.Errorf("创建初始提示词文件 %s 失败: %w", p, err)
		}
	} else if err != nil {
		return fmt.Errorf("访问提示词文件 %s 失败: %w", p, err)
	}

	// 打开编辑器
	if err := editor.Edit(cmd.Context(), p); err != nil {
		return fmt.Errorf("编辑器退出错误: %w", err)
	}
	return nil
}
