package main

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Long:  `显示 dscli 的版本信息、构建信息和运行时信息。`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("dscli 版本: %s\n", Version)
		if Build != "" {
			fmt.Printf("构建标识: %s\n", Build)
		}
		fmt.Printf("Go 版本: %s\n", runtime.Version())
		fmt.Printf("操作系统: %s\n", runtime.GOOS)
		fmt.Printf("处理器架构: %s\n", runtime.GOARCH)
	},
}

func init() {
	RootCmd.AddCommand(versionCmd)
}
