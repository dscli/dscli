package web

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/lp"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed web.md
var web_md string

// handleWebReader 通过 lightpanda 浏览器读取网页内容并返回 Markdown。
func handleWebReader(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	url := toolcall.ToolArgsValue(args, "url", "")
	if url == "" {
		err = fmt.Errorf("no URL or empty URL specified")
		return result, warning, err
	}

	// 确保URL以http://或https://开头
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "https://" + url
	}

	startTime := time.Now()
	markdown, err := lp.Get(ctx, url)
	elapsed := time.Since(startTime)
	if err != nil {
		return result, warning, fmt.Errorf("web_reader: %w", err)
	}

	outfmt.Notice("读取网页: %q", url)

	result = fmt.Sprintf(`📝 执行结果:
网页内容（Markdown格式）:
%s

网页信息:
- URL: %s
- 响应时间: %v
- 输出格式: Markdown（lightpanda）

📊 执行统计:
执行时间: %v
状态: 成功`,
		markdown,
		url,
		elapsed,
		elapsed)

	return result, warning, nil
}

func init() {
	// 注册网页读取工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "web_reader",
		Description: web_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "Web page URL, e.g. https://github.com/golang/go",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (default 120). Set longer (e.g. 300) for slow websites.",
				},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
		Category: "web",
		Timeout:  120 * time.Second,
		Handler:  handleWebReader,
	})
}
