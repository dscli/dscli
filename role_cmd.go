package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/skills"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/spf13/cobra"
)

func init() {
	roleCmd := AddRootCommand(&cobra.Command{
		Use:   "role",
		Short: "角色配置管理 - 管理角色与技能、工具、提示词的映射",
		Long: `role 命令用于管理角色的技能、工具和提示词映射配置。

当前支持 4 个角色（dev / expert / review / test），每个角色可以针对当前项目
配置其可用的技能列表、工具列表以及对应的系统提示词模板。

示例：
  dscli role list                      列出当前项目所有角色配置
  dscli role show dev                  查看 dev 角色的配置
  dscli role update review --skills all --tools "shell,file_read" --prompt editor
  dscli role reset review              重置 review 角色的自定义配置`,
	})

	// list 子命令
	roleCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出当前项目的所有角色配置",
		Long:  `列出当前项目下所有角色的技能、工具、提示词映射（含默认值）。`,
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

	// update 子命令
	updateCmd := &cobra.Command{
		Use:   "update <role>",
		Short: "更新或创建角色的配置",
		Long: `更新或创建指定角色的配置。通过 --skills、--tools、--prompt 标志指定对应值。

未指定的标志保持原值不变；新建时未指定的标志默认为 "all"。

示例：
  dscli role update review --skills "go-fix,gofumpt" --tools "shell,file_read"
  dscli role update expert --tools "" --prompt editor
  dscli role update dev --skills all --tools "shell,file_read,markdown"`,
		Args: cobra.ExactArgs(1),
		RunE: roleUpdateRunE,
	}
	updateCmd.Flags().String("skills", "", "技能列表：all（全部）、空（无）、或逗号分隔的技能名")
	updateCmd.Flags().String("tools", "", "工具列表：all（全部）、空（无）、或逗号分隔的工具名")
	updateCmd.Flags().String("prompt", "", "提示词模板名称（空表示与角色同名）")
	roleCmd.AddCommand(updateCmd)

	// reset 子命令
	roleCmd.AddCommand(&cobra.Command{
		Use:   "reset <role>",
		Short: "重置角色的自定义配置（恢复默认行为）",
		Long:  `重置指定角色在当前项目的自定义配置，恢复为系统默认行为。`,
		Args:  cobra.ExactArgs(1),
		RunE:  roleResetRunE,
	})
}

// roleDefaults 定义四个内置角色的默认配置。
var roleDefaults = []struct {
	Role   string
	Skills string
	Tools  string
	Prompt string
}{
	{"dev", "all", "all", "dev"},
	{"expert", "none", "none", "expert"},
	{"review", "none", "none", "expert"},
	{"test", "all", "all", "test"},

}

func roleListRunE(cmd *cobra.Command, _ []string) error {
	sessionID := session.GetCurrentSessionID(cmd.Context())
	configs, err := roles.ListRoleConfigs(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出角色配置失败: %v\n", err)
		return nil
	}

	custom := make(map[string]roles.RoleConfig)
	for _, cfg := range configs {
		custom[cfg.Role] = cfg
	}

	type row struct {
		Role   string
		Skills string
		Tools  string
		Prompt string
	}
	var rows []row
	for _, d := range roleDefaults {
		skills := d.Skills
		tools := d.Tools
		prompt := d.Prompt
		if cfg, ok := custom[d.Role]; ok {
			skills = cfg.Skills
			if skills == "" {
				skills = "none"
			}
			tools = cfg.Tools
			if tools == "" {
				tools = "none"
			}
			prompt = cfg.Prompt
			if prompt == "" {
				prompt = d.Prompt + "（默认）"
			}
		}
		rows = append(rows, row{Role: d.Role, Skills: skills, Tools: tools, Prompt: prompt})
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
		fmt.Fprintf(os.Stderr, "查询角色配置失败: %v\n", err)
		return nil
	}

	if cfg == nil {
		for _, d := range roleDefaults {
			if d.Role == roleName {
				fmt.Printf("角色 %q 在当前项目没有自定义配置，使用默认行为。\n", roleName)
				fmt.Println()
				fmt.Printf("默认配置：skills=%s, tools=%s, prompt=%s\n", d.Skills, d.Tools, d.Prompt)
				return nil
			}
		}
		fmt.Printf("角色 %q 未识别。支持的角色：dev, expert, review, test\n", roleName)
		return nil
	}

	prompt := cfg.Prompt
	if prompt == "" {
		defaultPrompt := roleName
		for _, d := range roleDefaults {
			if d.Role == roleName {
				defaultPrompt = d.Prompt
				break
			}
		}
		prompt = defaultPrompt + "（默认）"
	}

	fmt.Printf("角色:     %s\n", cfg.Role)
	fmt.Printf("技能:     %s\n", cfg.Skills)
	fmt.Printf("工具:     %s\n", cfg.Tools)
	fmt.Printf("提示词:   %s\n", prompt)
	fmt.Printf("会话ID:   %d\n", cfg.SessionID)
	return nil
}


func roleUpdateRunE(cmd *cobra.Command, args []string) error {
	roleName := args[0]

	// Validate role name
	validRoles := map[string]bool{"dev": true, "expert": true, "review": true, "test": true}
	if !validRoles[roleName] {
		return fmt.Errorf("无效的角色名 %q，支持的角色：dev, expert, review, test", roleName)
	}

	skills, _ := cmd.Flags().GetString("skills")
	tools, _ := cmd.Flags().GetString("tools")
	prompt, _ := cmd.Flags().GetString("prompt")

	// Validate: at least one flag must be set
	if skills == "" && tools == "" && prompt == "" {
		return fmt.Errorf("至少需要指定 --skills、--tools 或 --prompt 之一")
	}

	// Validate tools
	if tools != "" && tools != "all" {
		if err := validateTools(tools); err != nil {
			return err
		}
	}

	// Validate skills
	if skills != "" && skills != "all" {
		if err := validateSkills(skills); err != nil {
			return err
		}
	}

	sessionID := session.GetCurrentSessionID(cmd.Context())
	if err := roles.UpsertRoleConfig(roleName, sessionID, skills, tools, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "保存角色配置失败: %v\n", err)
		return nil
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


// validateTools checks that all tool names in the comma-separated list are known.
func validateTools(tools string) error {
	known := toolcall.KnownToolNames()
	knownSet := make(map[string]bool, len(known))
	for _, t := range known {
		knownSet[t] = true
	}
	for t := range strings.SplitSeq(tools, ",") {
		t = strings.TrimSpace(t)
		if t != "" && !knownSet[t] {
			return fmt.Errorf("未知的工具 %q", t)
		}
	}
	return nil
}

// validateSkills checks that all skill names in the comma-separated list are known.
func validateSkills(skillsStr string) error {
	skillInfos, err := skills.ListAll()
	if err != nil {
		// If we can't list skills, skip validation but warn the user.
		fmt.Fprintf(os.Stderr, "警告: 无法验证技能列表: %v\n", err)
		return nil
	}
	knownSet := make(map[string]bool, len(skillInfos))
	for _, s := range skillInfos {
		knownSet[s.Name] = true
	}
	for s := range strings.SplitSeq(skillsStr, ",") {
		s = strings.TrimSpace(s)
		if s != "" && !knownSet[s] {
			return fmt.Errorf("未知的技能 %q", s)
		}
	}
	return nil
}

func roleResetRunE(cmd *cobra.Command, args []string) error {
	roleName := args[0]

	// Validate role name
	validRoles := map[string]bool{"dev": true, "expert": true, "review": true, "test": true}
	if !validRoles[roleName] {
		return fmt.Errorf("无效的角色名 %q，支持的角色：dev, expert, review, test", roleName)
	}

	sessionID := session.GetCurrentSessionID(cmd.Context())
	if err := roles.DeleteRoleConfig(roleName, sessionID); err != nil {
		fmt.Fprintf(os.Stderr, "重置角色配置失败: %v\n", err)
		return nil
	}

	fmt.Printf("已重置角色 %q 的配置，恢复默认行为。\n", roleName)
	return nil
}

