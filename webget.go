package main

import (
	"context"
	"fmt"
	"time"

	"gitcode.com/dscli/dscli/internal/lp"
	"github.com/spf13/cobra"
)

func init() {
	webgetCmd := AddRootCommand(&cobra.Command{
		Use:   "webget <url>",
		Short: "读取网页并转为 Markdown",
		Long: `通过 lightpanda 浏览器读取指定 URL 的网页内容，并输出为 Markdown 格式。

对于 JavaScript 渲染的页面和墙外网站（如 google.com），效果优于直接 HTTP 请求。

示例：
  dscli web reader https://go.dev
  dscli web reader https://www.google.com`,
		Args: cobra.ExactArgs(1),
		RunE: webReaderRunE,
	})
	webgetCmd.Flags().Int("timeout", 60, "超时时间（秒）")
	webgetCmd.Flags().Bool("force-remote", false, "强制使用远端 lightpanda 抓取网页")
}

func webReaderRunE(cmd *cobra.Command, args []string) error {
	url := args[0]
	timeout, _ := cmd.Flags().GetInt("timeout")
	forceRemote, _ := cmd.Flags().GetBool("force-remote")

	ctx := cmd.Context()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	var (
		markdown string
		err      error
	)
	if forceRemote {
		markdown, err = lp.GetRemote(ctx, url)
	} else {
		markdown, err = lp.Get(ctx, url)
	}
	if err != nil {
		return fmt.Errorf("读取网页失败: %w", err)
	}

	fmt.Print(markdown)
	return nil
}
