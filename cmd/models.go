package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

var modelsCmd = &cobra.Command{
	Use:   "models",
	Short: "列出 DeepSeek 支持的模型",
	Run: func(cmd *cobra.Command, args []string) {
		resp, err := client.Models()
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取模型列表失败: %v\n", err)
			os.Exit(1)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\t对象\t拥有者")
		for _, m := range resp.Data {
			fmt.Fprintf(w, "%s\t%s\t%s\n", m.ID, m.Object, m.OwnedBy)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(modelsCmd)
}