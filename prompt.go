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
	showCmd := AddCommand(promptCmd, &cobra.Command{
		Use:  "show",
		RunE: promptShowRunE,
	})

	editCmd := AddCommand(promptCmd, &cobra.Command{
		Use:  "edit",
		RunE: promptEditRunE,
	})
	editCmd.Flags().Bool("global", false, "global")
	editCmd.Flags().String("role", "dev", "role: dev|expert|review")
	showCmd.Flags().String("role", "dev", "role: dev|expert|review")
}

// getRoleFromFlags 从命令行标志获取角色类型
func getRoleFromFlags(cmd *cobra.Command) (string, error) {
	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return "", fmt.Errorf("获取role标志失败: %w", err)
	}
	switch role {
	case "dev", "expert", "review":
		return role, nil
	default:
		return "", fmt.Errorf("无效角色 %q，支持: dev|expert|review", role)
	}
}

func promptShowRunE(cmd *cobra.Command, args []string) (err error) {
	role, err := getRoleFromFlags(cmd)
	if err != nil {
		return fmt.Errorf("获取角色类型失败: %w", err)
	}

	promptTemplate := prompt.GetPromptTemplate(cmd.Context(), role)
	outfmt.Println(promptTemplate)
	return nil
}

func promptEditRunE(cmd *cobra.Command, args []string) (err error) {
	global, err := cmd.Flags().GetBool("global")
	if err != nil {
		return fmt.Errorf("获取global标志失败: %w", err)
	}

	role, err := getRoleFromFlags(cmd)
	if err != nil {
		return fmt.Errorf("获取角色类型失败: %w", err)
	}

	// 获取目标文件路径
	p, err := prompt.GetPromptPath(role, global)
	if err != nil {
		return fmt.Errorf("确定提示词文件路径失败: %w", err)
	}

	// 检查文件是否存在，若不存在则用默认内容创建
	if _, err := os.Stat(p); os.IsNotExist(err) {
		// 使用内嵌的默认模板内容
		defaultContent := prompt.GetDefaultPromptTemplate(role)
		if err := os.WriteFile(p, []byte(defaultContent), 0o644); err != nil {
			return fmt.Errorf("创建初始提示词文件 %s 失败: %w", p, err)
		}
	} else if err != nil {
		// 处理 Stat 的其他错误（如权限）
		return fmt.Errorf("访问提示词文件 %s 失败: %w", p, err)
	}

	// 文件已存在或已成功创建，开始编辑
	ctx := cmd.Context()
	if err := editor.Edit(ctx, p); err != nil {
		return fmt.Errorf("编辑器退出错误: %w", err)
	}

	return nil
}
