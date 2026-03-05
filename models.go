package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var modelsFormat string

func init() {
	modelsCmd := AddRootCommand(&cobra.Command{
		Use:   "models",
		Short: "列出 DeepSeek 支持的模型",
		Run:   ModelsRun,
	})
	modelsCmd.Flags().StringVarP(&modelsFormat, "format", "f", "table", "输出格式：table（表格）、json（JSON）")
}

func ModelsRun(cmd *cobra.Command, args []string) {
	resp, err := DeepseekClient.Models()
	if err != nil {
		fmt.Fprintf(os.Stderr, "获取模型列表失败: %v\n", err)
		os.Exit(1)
	}

	// 使用新的格式化接口
	headers := []string{"ID", "对象", "拥有者"}
	rowFunc := func(data any) []string {
		switch m := data.(type) {
		case Model:
			return []string{m.ID, m.Object, m.OwnedBy}
		default:
			return []string{"", "", ""}
		}
	}

	err = FormatOutput(resp.Data, modelsFormat, headers, rowFunc)
	if err != nil {
		fmt.Fprintf(os.Stderr, "格式化输出失败: %v\n", err)
		os.Exit(1)
	}
}
