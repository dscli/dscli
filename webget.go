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
失败后自动经代理重试。

示例：
  dscli webget https://go.dev
  dscli webget https://www.google.com
  dscli webget https://example.com --dump html`,
		Args: cobra.ExactArgs(1),
		RunE: webReaderRunE,
	})
	// 默认 330s 与 web_fetch 工具一致：lp.Fetch 内部直连探测 20s、代理重试
	// HTTP 上限 300s、页面 JS 上限 60s，默认 60s 会把走代理的慢站误杀。
	// 传 0 表示不设总超时，仍受 lp.Fetch 内部 300s 上限保护。
	webgetCmd.Flags().Int("timeout", 330, "总超时时间（秒），0 表示不设上限")
	webgetCmd.Flags().String("dump", "markdown", "输出格式：markdown | html | semantic_tree | semantic_tree_text")
}

func webReaderRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "webReaderRunE")
	defer span.Finish()

	rawURL := args[0]
	// url.Parse accepts case-insensitive schemes (RFC 3986) and rejects
	// malformed URLs, unlike a plain HasPrefix check.
	u, err := url.Parse(rawURL)
	scheme := strings.ToLower(u.Scheme)
	if err != nil || (scheme != "http" && scheme != "https") {
		fmt.Fprintln(cmd.ErrOrStderr(), "Error: URL 必须以 http:// 或 https:// 开头:", rawURL)
		return nil
	}

	dump, _ := cmd.Flags().GetString("dump")
	if !webDumps[dump] {
		fmt.Fprintln(cmd.ErrOrStderr(), "Error: --dump 仅支持 markdown | html | semantic_tree | semantic_tree_text，got", dump)
		return nil
	}

	timeout, _ := cmd.Flags().GetInt("timeout")
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
		defer cancel()
	}

	text, err := lp.Fetch(ctx, rawURL, lp.FetchOptions{Dump: dump})
	if err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Error: 读取网页失败:", err)
		return nil
	}

	fmt.Fprint(cmd.OutOrStdout(), text)
	return nil
}
