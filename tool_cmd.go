package main

import (
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/toolcall"
	"github.com/spf13/cobra"
)

func init() {
	toolCmd := AddRootCommand(&cobra.Command{
		Use:   "tool",
		Short: "工具管理 - 列出和查询可用工具",
		Long: `tool 命令用于管理可用的工具。

示例：
  dscli tool list                    列出所有工具
  dscli tool list --category file   按分类列出工具
  dscli tool stats                   显示工具使用统计
  dscli tool stats --days 7         显示最近 7 天的统计
  dscli tool stats --project        显示当前项目的统计`,
	})

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有可用工具",
		Long:  `列出系统中所有可用的工具，包括名称、分类和描述。`,
		Args:  cobra.NoArgs,
		RunE:  toolListRunE,
	}
	listCmd.Flags().String("category", "", "按分类过滤（如 file, code, shell, skill 等）")
	toolCmd.AddCommand(listCmd)

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "显示工具使用统计",
		Long: `显示工具的使用次数、成功率和最后使用时间。

默认显示所有项目的全局统计。使用 --project 仅显示当前项目统计。
使用 --days 限制统计最近 N 天的数据。`,
		Args: cobra.NoArgs,
		RunE: toolStatsRunE,
	}
	statsCmd.Flags().Int("days", 0, "统计最近 N 天的数据（0 表示全部）")
	statsCmd.Flags().Bool("project", false, "仅显示当前项目的使用情况")
	toolCmd.AddCommand(statsCmd)
}

func toolListRunE(cmd *cobra.Command, _ []string) error {
	category, _ := cmd.Flags().GetString("category")

	tools, err := toolcall.ListTools(category)
	if err != nil {
		return fmt.Errorf("列出工具失败: %w", err)
	}

	if len(tools) == 0 {
		if category != "" {
			fmt.Printf("没有找到分类为 %q 的工具。\n", category)
		} else {
			fmt.Println("没有找到任何工具。")
		}
		return nil
	}

	type row struct {
		Name     string
		Category string
		Desc     string
	}

	// firstLine 取描述的首行（到第一个换行符为止）
	firstLine := func(s string) string {
		if before, _, found := strings.Cut(s, "\n"); found {
			return before
		}
		return s
	}
	var rows []row
	for _, t := range tools {
		cat := t.Category
		if cat == "" {
			cat = "-"
		}
		desc := strings.TrimSuffix(firstLine(t.Description), ".")
		if len([]rune(desc)) > 48 {
			desc = toolcall.TruncateHead(desc, 48)
		}
		rows = append(rows, row{
			Name:     t.Name,
			Category: cat,
			Desc:     desc,
		})
	}

	headers := []string{"名称", "分类", "描述"}
	rowFunc := func(data any) []string {
		if r, ok := data.(row); ok {
			return []string{r.Name, r.Category, r.Desc}
		}
		return nil
	}

	return FormatOutput(rows, "table", headers, rowFunc)
}

func toolStatsRunE(cmd *cobra.Command, _ []string) error {
	days, _ := cmd.Flags().GetInt("days")
	project, _ := cmd.Flags().GetBool("project")

	var stats []toolcall.ToolUsageStat
	var err error

	if project {
		stats, err = toolcall.GetProjectToolUsage(cmd.Context(), days)
	} else {
		stats, err = toolcall.GetToolUsageStats(days)
	}

	if err != nil {
		return fmt.Errorf("获取工具统计失败: %w", err)
	}

	if len(stats) == 0 {
		fmt.Println("没有工具使用记录。")
		return nil
	}

	type row struct {
		Name        string
		UsageCount  int
		SuccessRate string
		LastUsed    string
	}

	var rows []row
	for _, s := range stats {
		lastUsed := "-"
		if !s.LastUsed.IsZero() {
			lastUsed = s.LastUsed.Format("2006-01-02 15:04")
		}
		rows = append(rows, row{
			Name:        s.Name,
			UsageCount:  s.UsageCount,
			SuccessRate: fmt.Sprintf("%.1f%%", s.SuccessRate),
			LastUsed:    lastUsed,
		})
	}

	headers := []string{"工具名称", "使用次数", "成功率", "最后使用"}
	rowFunc := func(data any) []string {
		if r, ok := data.(row); ok {
			return []string{r.Name, fmt.Sprintf("%d", r.UsageCount), r.SuccessRate, r.LastUsed}
		}
		return nil
	}

	return FormatOutput(rows, "table", headers, rowFunc)
}