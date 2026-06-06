package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/lp"
	"github.com/spf13/cobra"
)

func init() {
	mdtowxhtmlCmd := AddRootCommand(&cobra.Command{
		Use:   "mdtowxhtml <file.md>",
		Short: "将 Markdown 转换为微信公众号格式的 HTML",
		Long: `通过 Chrome 浏览器使用 quaily.com 的 Markdown 转微信公众号格式工具，
将 Markdown 文件转换为 WeChat 兼容的 HTML 富文本。

输出文件为同名的 .html 文件（例如 article.md → article.html），
内容为可直接粘贴到微信公众号编辑器中的 HTML 片段。

示例：
  dscli mdtowxhtml article.md
  dscli mdtowxhtml path/to/post.md`,
		Args: cobra.ExactArgs(1),
		RunE: mdtowxhtmlRunE,
	})
	mdtowxhtmlCmd.Flags().Duration("timeout", 60*time.Second, "超时时间（例如 30s, 2m）")
}

func mdtowxhtmlRunE(cmd *cobra.Command, args []string) error {
	mdPath := args[0]

	// Read the markdown file.
	mdContent, err := os.ReadFile(mdPath)
	if err != nil {
		return fmt.Errorf("读取文件 %s 失败: %w", mdPath, err)
	}

	// Trim whitespace and check for truly empty content before launching
	// a headless browser (which would time out with empty input).
	content := strings.TrimSpace(string(mdContent))
	if content == "" {
		return fmt.Errorf("文件 %s 为空", mdPath)
	}

	// Determine output path: same directory, .html extension.
	ext := filepath.Ext(mdPath)
	outPath := strings.TrimSuffix(mdPath, ext) + ".html"

	ctx := cmd.Context()

	timeout, _ := cmd.Flags().GetDuration("timeout")
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	startTime := time.Now()
	fmt.Fprintf(os.Stderr, "🔄 转换 Markdown → WeChat HTML...\n")

	html, err := lp.MdToWx(ctx, content)
	if err != nil {
		return fmt.Errorf("mdtowxhtml 失败: %w", err)
	}

	if err := os.WriteFile(outPath, []byte(html), 0o644); err != nil {
		return fmt.Errorf("写入输出文件 %s 失败: %w", outPath, err)
	}

	elapsed := time.Since(startTime)
	fmt.Fprintf(os.Stderr, "✅ 转换完成 (%.1fs): %s (%d 字节)\n",
		elapsed.Seconds(), outPath, len(html))

	return nil
}
