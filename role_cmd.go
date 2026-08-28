package main

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/skills"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
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
仅显式传入的标志会被修改；其余字段保持不变。新建配置时未指定的字段使用
该角色的默认配置（dev/test：all；expert/review：none）。

技能与工具接受三个取值：
  all  全部（默认）
  none 无（等价于空字符串）
  a,b  逗号分隔的名称列表

示例：
  dscli role update review --skills "go-fix,gofumpt" --tools "shell,file_read"
  dscli role update expert --tools none --prompt editor
  dscli role update test --tools "" --skills all
  dscli role update dev --skills all --tools "shell,file_read,markdown"`,
		Args: cobra.ExactArgs(1),
		RunE: roleUpdateRunE,
	}
	updateCmd.Flags().String("skills", "", "技能列表：all（全部）、none（无）或逗号分隔的技能名；省略表示不修改（新建时按角色默认）")
	updateCmd.Flags().String("tools", "", "工具列表：all（全部）、none（无）或逗号分隔的工具名；省略表示不修改（新建时按角色默认）")
	updateCmd.Flags().String("prompt", "", "提示词模板名称；空表示使用与角色同名的默认模板")
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

// roleNames 是 role 系统支持的四个内置角色（顺序即显示顺序）。
var roleNames = []string{"dev", "expert", "review", "test"}

// displaySpec converts a stored skills/tools value to its display form:
// "" (none) renders as "none", everything else verbatim.
func displaySpec(v string) string {
	if v == "" {
		return "none"
	}
	return v
}

func roleListRunE(cmd *cobra.Command, _ []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "roleListRunE")
	defer span.Finish()

	sessionID := session.GetCurrentSessionID(ctx)
	configs, err := roles.ListRoleConfigs(ctx, sessionID)
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
	for _, name := range roleNames {
		d := roles.DefaultFor(name)
		skills, tools, prompt := displaySpec(d.Skills), displaySpec(d.Tools), d.Prompt
		if cfg, ok := custom[name]; ok {
			skills = displaySpec(cfg.Skills)
			tools = displaySpec(cfg.Tools)
			prompt = cfg.Prompt
			if prompt == "" {
				// 未指定 → 使用与角色同名的默认模板。直接显示模板名，
				// 不带“（默认）”后缀，避免自定义配置被误读为回退默认。
				prompt = d.Prompt
			}
		}
		rows = append(rows, row{Role: name, Skills: skills, Tools: tools, Prompt: prompt})
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
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "roleShowRunE")
	defer span.Finish()

	roleName := args[0]

	sessionID := session.GetCurrentSessionID(ctx)
	cfg, err := roles.GetRoleConfig(ctx, roleName, sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "查询角色配置失败: %v\n", err)
		return nil
	}

	if cfg == nil {
		valid := false
		for _, name := range roleNames {
			if name == roleName {
				def := roles.DefaultFor(roleName)
				fmt.Printf("角色 %q 在当前项目没有自定义配置，使用默认行为。\n", roleName)
				fmt.Println()
				fmt.Printf("默认配置：skills=%s, tools=%s, prompt=%s\n", displaySpec(def.Skills), displaySpec(def.Tools), def.Prompt)
				valid = true
				break
			}
		}
		if !valid {
			fmt.Printf("角色 %q 未识别。支持的角色：%s\n", roleName, strings.Join(roleNames, ", "))
		}
		return nil
	}

	prompt := cfg.Prompt
	if prompt == "" {
		prompt = roleName + "（默认模板）"
	}
	fmt.Printf("角色:     %s\n", cfg.Role)
	fmt.Printf("技能:     %s\n", displaySpec(cfg.Skills))
	fmt.Printf("工具:     %s\n", displaySpec(cfg.Tools))
	fmt.Printf("提示词:   %s\n", prompt)
	fmt.Printf("会话ID:   %d\n", cfg.SessionID)
	return nil
}

func roleUpdateRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "roleUpdateRunE")
	defer span.Finish()

	roleName := args[0]

	// Validate role name
	if !slices.Contains(roleNames, roleName) {
		return fmt.Errorf("无效的角色名 %q，支持的角色：%s", roleName, strings.Join(roleNames, ", "))
	}

	// Changed() distinguishes "flag not passed" (keep) from "flag passed as
	// empty/none" (explicitly clear): `--tools ""` and `--tools none` both
	// mean "set to no tools", while omitting --tools keeps the old value.
	skillsFlag, _ := cmd.Flags().GetString("skills")
	toolsFlag, _ := cmd.Flags().GetString("tools")
	promptFlag, _ := cmd.Flags().GetString("prompt")
	skillsSet := cmd.Flags().Changed("skills")
	toolsSet := cmd.Flags().Changed("tools")
	promptSet := cmd.Flags().Changed("prompt")

	if !skillsSet && !toolsSet && !promptSet {
		return fmt.Errorf("至少需要指定 --skills、--tools 或 --prompt 之一")
	}

	var skills, tools, prompt *string
	if skillsSet {
		v := normalizeRoleSpec(skillsFlag)
		if v != "" && v != "all" {
			if err := validateSkills(ctx, v); err != nil {
				return err
			}
		}
		skills = &v
	}
	if toolsSet {
		v := normalizeRoleSpec(toolsFlag)
		if v != "" && v != "all" {
			if err := validateTools(v); err != nil {
				return err
			}
		}
		tools = &v
	}
	if promptSet {
		v := strings.TrimSpace(promptFlag)
		prompt = &v
	}

	sessionID := session.GetCurrentSessionID(ctx)
	if err := roles.UpsertRoleConfig(ctx, roleName, sessionID, skills, tools, prompt); err != nil {
		fmt.Fprintf(os.Stderr, "保存角色配置失败: %v\n", err)
		return nil
	}

	fmt.Printf("已更新角色 %q 的配置。\n", roleName)
	if skillsSet {
		fmt.Printf("  技能: %s\n", displaySpec(*skills))
	}
	if toolsSet {
		fmt.Printf("  工具: %s\n", displaySpec(*tools))
	}
	if promptSet {
		fmt.Printf("  提示词: %s\n", *prompt)
	}
	return nil
}

// normalizeRoleSpec maps a user-supplied --skills/--tools value to storage
// form: "none" (case-insensitive) or "" → "" (explicitly nothing), "all"
// stays "all", anything else is kept verbatim for name validation.
func normalizeRoleSpec(v string) string {
	v = strings.TrimSpace(v)
	if strings.EqualFold(v, "none") {
		return ""
	}
	return v
}

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
func validateSkills(ctx context.Context, skillsStr string) error {
	span, ctx := clog.StartSpanFromContext(ctx, "validateSkills")
	defer span.Finish()
	skillInfos, err := skills.ListAll(ctx)
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
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "roleResetRunE")
	defer span.Finish()

	roleName := args[0]

	// Validate role name
	if !slices.Contains(roleNames, roleName) {
		return fmt.Errorf("无效的角色名 %q，支持的角色：%s", roleName, strings.Join(roleNames, ", "))
	}

	sessionID := session.GetCurrentSessionID(ctx)
	if err := roles.DeleteRoleConfig(ctx, roleName, sessionID); err != nil {
		fmt.Fprintf(os.Stderr, "重置角色配置失败: %v\n", err)
		return nil
	}

	fmt.Printf("已重置角色 %q 的配置，恢复默认行为。\n", roleName)
	return nil
}
