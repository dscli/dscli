package web

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed lp_fetch.md
var lpFetchMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "lightpanda-fetch",
		DisplayName: "LightPanda Fetch",
		Description: lpFetchMd,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"url": map[string]any{
					"type":        "string",
					"description": "URL to fetch and convert to text (required)",
				},
				"dump": map[string]any{
					"type":        "string",
					"enum":        []string{"markdown", "html", "semantic_tree", "semantic_tree_text"},
					"description": "Output format (default: markdown)",
				},
				"terminate-ms": map[string]any{
					"type":        "integer",
					"description": "Hard deadline in milliseconds; aborts pages with endless scripts",
				},
				"proxy": map[string]any{
					"type":        "string",
					"description": "HTTP proxy URL, e.g. socks5h://localhost:8777. Use socks5h (proxy-side DNS), not socks5. Falls back to the lightpanda-proxy value from ~/.dscli/dscli.env when unset",
				},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
		Category: "web",
		Timeout:  330 * time.Second, // > http-timeout (300s); per-page JS deadline is terminate-ms
		Handler:  handleLPFetch,
	})
}

func handleLPFetch(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleLPFetch")
	defer span.Finish()

	rawURL := toolcall.ToolArgsValue(args, "url", "")
	if rawURL == "" {
		return "", "", fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return "", "", fmt.Errorf("url must start with http:// or https://, got %q", rawURL)
	}
	opts := lp.FetchOptions{
		Dump:        toolcall.ToolArgsValue(args, "dump", ""),
		TerminateMS: toolcall.ToolArgsValue(args, "terminate-ms", 0),
		Proxy:       toolcall.ToolArgsValue(args, "proxy", ""),
	}

	text, err := lp.Fetch(ctx, rawURL, opts)
	if err != nil {
		return "", "", err
	}
	return text, "", nil
}
