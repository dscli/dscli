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
				"strip-mode": map[string]any{
					"type":        "string",
					"description": "Comma-separated tag groups to remove from the dump: js, css, ui, invisible, full (e.g. \"js,css,ui\")",
				},
				"wait-until": map[string]any{
					"type":        "string",
					"enum":        []string{"load", "domcontentloaded", "networkalmostidle", "networkidle", "done"},
					"description": "Event to wait for before dumping (default: done). Use domcontentloaded for search engines, networkidle for dynamic pages",
				},
				"wait-ms": map[string]any{
					"type":        "integer",
					"description": "Wait time in milliseconds (default 5000)",
				},
				"terminate-ms": map[string]any{
					"type":        "integer",
					"description": "Hard deadline in milliseconds; aborts pages with endless scripts",
				},
				"proxy": map[string]any{
					"type":        "string",
					"description": "HTTP proxy URL, e.g. socks5h://localhost:8777. Use socks5h (proxy-side DNS), not socks5. Falls back to the lightpanda-proxy config",
				},
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
		Category: "web",
		Timeout:  120 * time.Second, // process-level backstop; per-page JS deadline is terminate-ms
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
		StripMode:   toolcall.ToolArgsValue(args, "strip-mode", ""),
		WaitUntil:   toolcall.ToolArgsValue(args, "wait-until", ""),
		WaitMS:      toolcall.ToolArgsValue(args, "wait-ms", 0),
		TerminateMS: toolcall.ToolArgsValue(args, "terminate-ms", 0),
		Proxy:       toolcall.ToolArgsValue(args, "proxy", ""),
	}

	text, err := lp.Fetch(ctx, rawURL, opts)
	if err != nil {
		return "", "", err
	}
	return text, "", nil
}
