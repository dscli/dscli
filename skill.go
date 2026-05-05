package main

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
	"github.com/spf13/cobra"
)

var skillCmd *cobra.Command

func init() {
	skillCmd = AddRootCommand(&cobra.Command{
		Use:   "skill",
		Short: "技能管理 - 安装、显示和查询技能",
		Long: `skill 命令用于管理和使用技能。

技能是预定义的代码片段或模板，可以快速复用。
支持本地技能（项目目录）和全局技能（用户配置目录）。`,
	})

	// show 子命令
	showCmd := &cobra.Command{
		Use:   "show <skill name>",
		Short: "显示指定名称的技能",
		Long: `显示指定名称的技能。

技能名称可以是本地技能或全局技能。
本地技能优先于全局技能。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			skillName := args[0]
			content, err := skills.Use(skillName)
			if err != nil {
				return fmt.Errorf("使用技能失败: %w", err)
			}

			outfmt.Print(content)
			return nil
		},
	}
	skillCmd.AddCommand(showCmd)

	// query 子命令
	queryCmd := &cobra.Command{
		Use:   "query <query>",
		Short: "查询匹配关键词的技能",
		Long: `查询匹配关键词的技能。

查询会匹配技能的关键词，返回所有匹配的技能摘要。
关键词匹配是大小写不敏感的。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := args[0]
			result, err := skills.Query(query)
			if err != nil {
				return fmt.Errorf("查询技能失败: %w", err)
			}

			outfmt.Print(result)
			return nil
		},
	}
	skillCmd.AddCommand(queryCmd)

	// list 子命令
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有可用技能",
		Long: `列出所有可用技能。

显示本地和全局技能列表，按名称排序。
本地技能优先于全局技能（同名时只显示本地技能）。`,
		Args: cobra.NoArgs,
		RunE: SkillListRunE,
	}
	skillCmd.AddCommand(listCmd)

	// set-auto-inject 子命令
	setAutoInjectCmd := &cobra.Command{
		Use:   "set-auto-inject <name> <true|false>",
		Short: "设置技能的自动注入属性",
		Long: `设置技能的 auto_inject 属性。

当 auto_inject 为 true 时，技能内容会自动注入到每次对话的上下文中，无需 LLM 主动获取。
当为 false 时，技能仅在 skill 列表中展示，LLM 需要时才通过 skill_by_name 获取。

默认修改本地技能；使用 --global/-g 标志修改全局技能。`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			val := args[1]
			var autoInject bool
			switch val {
			case "true":
				autoInject = true
			case "false":
				autoInject = false
			default:
				return fmt.Errorf("无效的值 %q，必须为 true 或 false", val)
			}

			global, _ := cmd.Flags().GetBool("global")

			if err := skills.SetAutoInject(name, autoInject, global); err != nil {
				return fmt.Errorf("设置 auto_inject 失败: %w", err)
			}

			scope := "本地"
			if global {
				scope = "全局"
			}
			fmt.Printf("已将 %s 技能 %q 的 auto_inject 设置为 %v\n", scope, name, autoInject)
			return nil
		},
	}
	setAutoInjectCmd.Flags().BoolP("global", "g", false, "修改全局技能而非本地技能")
	skillCmd.AddCommand(setAutoInjectCmd)
}

// SkillListRunE 列出所有技能
func SkillListRunE(cmd *cobra.Command, args []string) error {
	skillInfos, err := skills.ListAll()
	if err != nil {
		return fmt.Errorf("列出技能失败: %w", err)
	}

	if len(skillInfos) == 0 {
		fmt.Fprint(os.Stderr, "没有找到任何技能。\n")
		return nil
	}

	// 转换为map数组，以便使用FormatOutput
	var skillMaps []map[string]string
	for _, info := range skillInfos {
		autoInject := "-"
		if info.AutoInject {
			autoInject = "是"
		}
		skillMaps = append(skillMaps, map[string]string{
			"name":        info.Name,
			"scope":       info.Scope,
			"auto_inject": autoInject,
		})
	}

	// 使用FormatOutput进行格式化输出
	headers := []string{"名称", "范围", "自动注入"}
	rowFunc := func(data any) []string {
		switch info := data.(type) {
		case map[string]string:
			return []string{info["name"], info["scope"], info["auto_inject"]}
		default:
			return []string{"", "", ""}
		}
	}

	// 使用默认的table格式
	err = FormatOutput(skillMaps, "table", headers, rowFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "格式化输出失败: %v\n", err)
		os.Exit(1)
	}

	return nil
}