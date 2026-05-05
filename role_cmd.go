package main

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/roles"
	"gitcode.com/dscli/dscli/internal/session"
	"github.com/spf13/cobra"
)

func init() {
	roleCmd := AddRootCommand(&cobra.Command{
		Use:   "role",
		Short: "角色配置管理 - 管理角色与技能、工具、提示词的映射",
		Long: `role 命令用于管理角色的技能、工具和提示词映射配置。

每个角色（dev/expert/review/writer/editor 等）可以针对当前项目
配置其可用的技能列表、工具列表以及对应的系统提示词模板。

示例：
  dscli role list                    列出当前项目所有角色配置
  dscli role show dev                查看 dev 角色的配置
  dscli role edit review --skills all --tools "shell,file_read" --prompt editor
  dscli role delete writer           删除 writer 角色的自定义配置`,
	})

	// list 子命令
	roleCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出当前项目的所有角色配置",
		Long:  `列出当前项目下所有已配置的角色及其技能、工具、提示词映射。`,
		Args:  cobra.NoArgs,
		RunE:  roleListRunE,
	})

	// show 子命令
	roleCmd.AddCommand(&cobra.Command{
		Use:   "show <role>",
		Short: "查看指定角色的配置详情",
		Long:  `显示指定角色在当前项目下的完整配置，包括技能列表、工具列表和提示词模板名称。`,
		Args:  cobra.ExactArgs(1),
		RunE:  roleShowRunE,
	})

	// edit 子命令
	editCmd := &cobra.Command{
		Use:   "edit <role>",
		Short: "编辑或创建角色的配置",
		Long: `编辑或创建指定角色的配置。通过 --skills、--tools、--prompt 标志指定对应值。

未指定的标志保持原值不变；新建时未指定的标志默认为 "all"。

示例：
  dscli role edit review --skills "go-fix,gofumpt" --tools "shell,file_read"
  dscli role edit expert --tools "" --prompt editor
  dscli role edit writer --skills all --tools "shell,file_read,web_reader"`,
		Args: cobra.ExactArgs(1),
		RunE: roleEditRunE,
	}
	editCmd.Flags().String("skills", "", "技能列表：all（全部）、空（无）、或逗号分隔的技能名")
	editCmd.Flags().String("tools", "", "工具列表：all（全部）、空（无）、或逗号分隔的工具名")
	editCmd.Flags().String("prompt", "", "提示词模板名称（空表示与角色同名）")
	roleCmd.AddCommand(editCmd)

	// delete 子命令
	roleCmd.AddCommand(&cobra.Command{
		Use:   "delete <role>",
		Short: "删除角色的自定义配置（恢复默认行为）",
		Long:  `删除指定角色在当前项目的自定义配置，恢复为系统默认行为。`,
		Args:  cobra.ExactArgs(1),
		RunE:  roleDeleteRunE,
	})
}

func roleListRunE(cmd *cobra.Command, _ []string) error {
	sessionID := session.GetCurrentSessionID(cmd.Context())
	configs, err := roles.ListRoleConfigs(sessionID)
	if err != nil {
		return fmt.Errorf("列出角色配置失败: %w", err)
	}

	if len(configs) == 0 {
		fmt.Println("当前项目没有角色配置（使用默认行为）。")
		fmt.Println()
		fmt.Println("默认行为：")
		fmt.Println("  dev     → 所有技能、所有工具、dev.md 提示词")
		fmt.Println("  expert  → 无技能、无工具、expert.md 提示词")
		fmt.Println("  review  → 无技能、无工具、review.md 提示词")
		return nil
	}

	// 构建表格数据
	type row struct {
		Role   string
		Skills string
		Tools  string
		Prompt string
	}
	var rows []row
	for _, cfg := range configs {
		prompt := cfg.Prompt
		if prompt == "" {
			prompt = cfg.Role + "（默认）"
		}
		rows = append(rows, row{
			Role:   cfg.Role,
			Skills: cfg.Skills,
			Tools:  cfg.Tools,
			Prompt: prompt,
		})
	}

	headers := []string{"角色", "技能", "工具", "提示词"}
	rowFunc := func(data any) []string {
		if r, ok := data.(row); ok {
			return []string{r.Role, r.Skills, r.Tools, r.Prompt}
		}
		return nil
	}

	return FormatOutput(rows, "table", headers, rowFunc)
}

func roleShowRunE(cmd *cobra.Command, args []string) error {
	roleName := args[0]

	sessionID := session.GetCurrentSessionID(cmd.Context())
	cfg, err := roles.GetRoleConfig(roleName, sessionID)
	if err != nil {
		return fmt.Errorf("查询角色配置失败: %w", err)
	}

	if cfg == nil {
		fmt.Printf("角色 %q 在当前项目没有自定义配置，使用默认行为。\n", roleName)
		fmt.Println()
		switch roleName {
		case "dev":
			fmt.Println("默认配置：skills=all, tools=all, prompt=dev")
		default:
			fmt.Printf("默认配置：skills=, tools=, prompt=%s\n", roleName)
		}
		return nil
	}

	prompt := cfg.Prompt
	if prompt == "" {
		prompt = roleName + "（默认）"
	}

	fmt.Printf("角色:     %s\n", cfg.Role)
	fmt.Printf("技能:     %s\n", cfg.Skills)
	fmt.Printf("工具:     %s\n", cfg.Tools)
	fmt.Printf("提示词:   %s\n", prompt)
	fmt.Printf("会话ID:   %d\n", cfg.SessionID)
	return nil
}

func roleEditRunE(cmd *cobra.Command, args []string) error {
	roleName := args[0]

	skills, _ := cmd.Flags().GetString("skills")
	tools, _ := cmd.Flags().GetString("tools")
	prompt, _ := cmd.Flags().GetString("prompt")

	// Validate: at least one flag must be set
	if skills == "" && tools == "" && prompt == "" {
		return fmt.Errorf("至少需要指定 --skills、--tools 或 --prompt 之一")
	}

	sessionID := session.GetCurrentSessionID(cmd.Context())
	if err := roles.UpsertRoleConfig(roleName, sessionID, skills, tools, prompt); err != nil {
		return fmt.Errorf("保存角色配置失败: %w", err)
	}

	fmt.Printf("已更新角色 %q 的配置。\n", roleName)
	if skills != "" {
		fmt.Printf("  技能: %s\n", skills)
	}
	if tools != "" {
		fmt.Printf("  工具: %s\n", tools)
	}
	if prompt != "" {
		fmt.Printf("  提示词: %s\n", prompt)
	}
	return nil
}

func roleDeleteRunE(cmd *cobra.Command, args []string) error {
	roleName := args[0]

	sessionID := session.GetCurrentSessionID(cmd.Context())
	if err := roles.DeleteRoleConfig(roleName, sessionID); err != nil {
		return fmt.Errorf("删除角色配置失败: %w", err)
	}

	fmt.Printf("已删除角色 %q 的自定义配置，恢复默认行为。\n", roleName)
	return nil
}
