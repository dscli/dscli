package main

import (
	"fmt"

	"github.com/dscli/dscli/internal/lp"
	"github.com/spf13/cobra"
)

func init() {
	chromiumCmd := AddRootCommand(&cobra.Command{
		Use:   "chromium",
		Short: "管理 Chromium 后台服务",
		Long: `管理 dscli 的 Chromium 后台服务。

Chromium 后台服务为 dscli webwxdraft 提供持久运行的浏览器实例，
使得自动化流程完成后浏览器仍保持打开，方便检查。

服务监听地址: 127.2.2.9:9228（与 lightpanda 9227 错开）`,
	})

	chromiumCmd.AddCommand(&cobra.Command{
		Use:   "service",
		Short: "创建并启动 Chromium 用户服务",
		Long: `创建（或更新）systemd 用户服务并启动 Chromium。

服务使用 --keep-alive-for-test 标志，确保在 CDP 客户端断开后
浏览器进程不退出。创建后可运行：

  dscli webwxdraft article.html --title "标题"

dscli 会自动检测正在运行的 Chromium 服务并连接，无需额外参数。`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := lp.SetupChromiumService(); err != nil {
				return fmt.Errorf("chromium service 设置失败: %w", err)
			}
			fmt.Println("✅ Chromium 服务已创建并启动")
			fmt.Println("   监听地址: 127.2.2.9:9228")
			return nil
		},
	})
}
