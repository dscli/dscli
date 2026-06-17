package main

import (
	"fmt"
	"os"

	"github.com/nanjj/clog"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/editor"
	"github.com/spf13/cobra"
)

func init() {
	configCmd := AddRootCommand(&cobra.Command{
		Use:   "config",
		Short: "配置文件管理",
	})

	AddCommand(configCmd, &cobra.Command{
		Use:  "edit",
		RunE: configEditRunE,
	})
}

func configEditRunE(cmd *cobra.Command, args []string) (err error) {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "configEditRunE")
	defer span.Finish()
	filename := config.Get("filename", "")
	if filename == "" {
		fmt.Fprintln(os.Stderr, "未配置文件名，请检查 config 配置")
		return nil
	}
	if err := editor.Edit(ctx, filename); err != nil {
		fmt.Fprintf(os.Stderr, "编辑配置失败: %v\n", err)
		return nil
	}
	return nil
}
