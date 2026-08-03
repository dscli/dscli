package main

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/lp"
	"github.com/nanjj/clog"
	"github.com/spf13/cobra"
)

// webDumps are the accepted --dump values, mirroring lightpanda fetch.
var webDumps = map[string]bool{
	"markdown":           true,
	"html":               true,
	"semantic_tree":      true,
	"semantic_tree_text": true,
}

func init() {
	webgetCmd := AddRootCommand(&cobra.Command{
		Use:   "webget <url>",
		Short: "读取网页并转为 Markdown",
		Long: `通过 lightpanda 浏览器读取指定 URL 的网页内容，并输出为 Markdown 格式。

对于 JavaScript 渲染的页面和墙外网站（如 google.com），效果优于直接 HTTP 请求。
已知封锁域名自动经配置的代理（lightpanda-http-proxy）抓取，其他域名先直连、
失败后自动经代理重试。可用配置 lightpanda-additional-proxy-domains 添加额外
的代理域名（数组或逗号分隔字符串），或加 --force-proxy 强制经代理抓取。

示例：
  dscli webget https://go.dev
  dscli webget https://www.google.com
  dscli webget https://example.com --dump html
  dscli webget https://example.org --force-proxy
  dscli webget https://example.com --output page.md
  dscli webget https://example.com --output notes.md:10   # 插入到第 10 行`,
		// 错误直接返回：main.go 统一打印并 exit 1。Silence* 避免 cobra 再刷
		// usage 和重复的 "Error:" 前缀，让失败看起来不像命令行用法错误。
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          webReaderRunE,
	})
	// 默认 330s 与 web_fetch 工具一致：lp.Fetch 内部直连探测 20s、代理重试
	// HTTP 上限 300s、页面 JS 上限 60s，默认 60s 会把走代理的慢站误杀。
	// 传 0 表示不设总超时，仍受 lp.Fetch 内部 300s 上限保护。
	webgetCmd.Flags().Int("timeout", 330, "总超时时间（秒），0 表示不设上限")
	webgetCmd.Flags().String("dump", "markdown", "输出格式：markdown | html | semantic_tree | semantic_tree_text")
	webgetCmd.Flags().Bool("force-proxy", false, "强制经配置的代理抓取（跳过直连探测）")
	webgetCmd.Flags().String("output", "", "把结果写入文件；带 :N 行号时插入到第 N 行（否则覆盖写入，文件不存在则创建）")
}

func webReaderRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "webReaderRunE")
	defer span.Finish()

	// 不用 cobra.ExactArgs：SilenceErrors 会吞掉参数校验错误，手动校验
	// 才能让错误消息正常返回给 main.go 打印。
	if len(args) != 1 {
		return fmt.Errorf("需要恰好 1 个 URL 参数，got %d", len(args))
	}
	rawURL := args[0]
	// url.Parse accepts case-insensitive schemes (RFC 3986) and rejects
	// malformed URLs, unlike a plain HasPrefix check.
	u, err := url.Parse(rawURL)
	scheme := strings.ToLower(u.Scheme)
	if err != nil || (scheme != "http" && scheme != "https") {
		return fmt.Errorf("URL 必须以 http:// 或 https:// 开头: %s", rawURL)
	}

	dump, _ := cmd.Flags().GetString("dump")
	if !webDumps[dump] {
		return fmt.Errorf("--dump 仅支持 markdown | html | semantic_tree | semantic_tree_text，got %s", dump)
	}

	forceProxy, _ := cmd.Flags().GetBool("force-proxy")
	output, _ := cmd.Flags().GetString("output")

	timeout, _ := cmd.Flags().GetInt("timeout")
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	text, err := lp.Fetch(ctx, rawURL, lp.FetchOptions{
		Dump:       dump,
		ForceProxy: forceProxy,
		Output:     output,
	})
	if err != nil {
		return fmt.Errorf("读取网页失败: %w", err)
	}

	fmt.Fprint(cmd.OutOrStdout(), text)
	return nil
}
