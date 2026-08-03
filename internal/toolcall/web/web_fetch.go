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

//go:embed web_fetch.md
var webFetchMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "web_fetch",
		DisplayName: "Web Fetch",
		Description: webFetchMd,
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
			},
			"required":             []string{"url"},
			"additionalProperties": false,
		},
		Category: "web",
		// Tool call timeout must exceed http-timeout (300s); JS is capped at
		// 60s internally, so a hung page fails well before the deadline.
		Timeout: 330 * time.Second,
		Handler: handleWebFetch,
	})
}

func handleWebFetch(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleWebFetch")
	defer span.Finish()

	rawURL := toolcall.ToolArgsValue(args, "url", "")
	if rawURL == "" {
		return "", "", fmt.Errorf("url is required")
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return "", "", fmt.Errorf("url must start with http:// or https://, got %q", rawURL)
	}

	text, err := lp.Fetch(ctx, rawURL, lp.FetchOptions{
		Dump: toolcall.ToolArgsValue(args, "dump", ""),
	})
	if err != nil {
		return "", "", err
	}
	return text, "", nil
}
