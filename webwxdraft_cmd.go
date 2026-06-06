package main

import (
	"fmt"
	"os"

	"github.com/dscli/dscli/internal/lp"
	"github.com/spf13/cobra"
)

func init() {
	webwxdraftCmd := AddRootCommand(&cobra.Command{
		Use:   "webwxdraft <file.html> --title <标题> [--author <作者>]",
		Short: "将 HTML 上传到微信公众号作为草稿",
		Long: `通过 Chrome 浏览器将本地 HTML 文件上传到微信公众号平台创建草稿。

工作流程：
  1. 读取本地 HTML 文件，提取正文内容和图片引用
  2. 打开 Chrome 浏览器，导航到 mp.weixin.qq.com
  3. 自动等待扫码登录（如果尚未登录）
  4. 自动导航到文章编辑页
  5. 自动填入标题、作者、正文
  6. 自动上传图片
  7. 自动保存草稿

--debug 模式：
  浏览器保持打开，不会自动关闭，方便手动检查编辑器状态、探查 DOM 结构、
  手动保存或排查自动化保存不成功的原因。

示例：
  dscli webwxdraft article.html --title "我的文章" --author "作者名"
  dscli webwxdraft ghostty-memory-leak-fix.html --title "查找并修复 Ghostty 最大的内存泄漏" --author "MitchellH"
  dscli webwxdraft article.html --title "测试" --debug   # 调试模式：探查 DOM 并保持浏览器打开`,
		Args: cobra.ExactArgs(1),
		RunE: webwxdraftRunE,
	})
	webwxdraftCmd.Flags().String("title", "", "文章标题（必填）")
	webwxdraftCmd.Flags().String("author", "", "文章作者（可选）")
	webwxdraftCmd.Flags().Bool("debug", false, "调试模式：探查编辑器 DOM 结构，完成后保持浏览器打开")
	_ = webwxdraftCmd.MarkFlagRequired("title")
}

func webwxdraftRunE(cmd *cobra.Command, args []string) error {
	htmlPath := args[0]

	// Verify the HTML file exists.
	if _, err := os.Stat(htmlPath); err != nil {
		return fmt.Errorf("文件 %s 不存在: %w", htmlPath, err)
	}

	title, _ := cmd.Flags().GetString("title")
	if title == "" {
		return fmt.Errorf("--title 是必填参数")
	}
	author, _ := cmd.Flags().GetString("author")
	debug, _ := cmd.Flags().GetBool("debug")

	ctx := cmd.Context()

	params := lp.WeChatDraftParams{
		HTMLPath: htmlPath,
		Title:    title,
		Author:   author,
		Debug:    debug,
	}

	return lp.WebWxDraft(ctx, params)
}
