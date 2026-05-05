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
  dscli tool list --category file   按分类列出工具`,
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
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			return s[:idx]
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