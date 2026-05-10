package main

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/editor"
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
	filename := config.Get("filename", "")
	if filename == "" {
		err = fmt.Errorf("no config filename found")
		return err
	}
	ctx := cmd.Context()
	return editor.Edit(ctx, filename)
}
