package main

import (
	"fmt"
	"os"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
	"github.com/spf13/cobra"
)

func init() {
	skillCmd := AddRootCommand(&cobra.Command{
		Use:   "skill",
		Short: "技能管理 - 使用和查询技能",
		Long: `skill 命令用于管理和使用技能。

技能是预定义的代码片段或模板，可以快速复用。
支持本地技能（项目目录）和全局技能（用户配置目录）。

子命令：
  use    使用指定名称的技能
  query  查询匹配关键词的技能
  list   列出所有可用技能`,
	})

	// use 子命令
	useCmd := &cobra.Command{
		Use:   "use <skill name>",
		Short: "使用指定名称的技能",
		Long: `使用指定名称的技能。

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
	skillCmd.AddCommand(useCmd)

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
		skillMaps = append(skillMaps, map[string]string{
			"name":  info.Name,
			"scope": info.Scope,
		})
	}

	// 使用FormatOutput进行格式化输出
	headers := []string{"名称", "范围"}
	rowFunc := func(data any) []string {
		switch info := data.(type) {
		case map[string]string:
			return []string{info["name"], info["scope"]}
		default:
			return []string{"", ""}
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
