package lp

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/config"
	"github.com/chromedp/chromedp"
)

// remoteHosts lists hosts that must be accessed via remote lightpanda
// (geo-restricted sites inaccessible from local network).
var remoteHosts = []string{
	"google.com",
	"www.google.com",
}

// isRemoteURL reports whether rawURL should be fetched via remote lightpanda.
func isRemoteURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return slices.Contains(remoteHosts, strings.ToLower(u.Host))
}

// getFromCDP performs the actual CDP interaction to fetch markdown content.
// It is a variable so that tests can replace it with a mock.
var getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
	// Create remote allocator pointing to lightpanda.
	allocatorCtx, allocatorCancel := chromedp.NewRemoteAllocator(ctx, cdpURL, chromedp.NoModifyURL)
	defer allocatorCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocatorCtx)
	defer tabCancel()

	// Apply timeout.
	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 60*time.Second)
	defer timeoutCancel()

	var markdownContent string

	err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			target := chromedp.FromContext(ctx).Target
			if target == nil {
				return fmt.Errorf("failed to get CDP target from context")
			}
			var result map[string]any
			if err := target.Execute(ctx, "LP.getMarkdown", nil, &result); err != nil {
				return fmt.Errorf("LP.getMarkdown: %w", err)
			}
			md, ok := result["markdown"].(string)
			if !ok {
				return fmt.Errorf("LP.getMarkdown response missing markdown field")
			}
			markdownContent = md
			return nil
		}),
	)
	if err != nil {
		return "", err
	}

	return markdownContent, nil
}

// Get fetches a web page via lightpanda CDP and returns its markdown content.
//
// It automatically routes to remote lightpanda for geo-restricted hosts
// (listed in remoteHosts), and uses local lightpanda for all other URLs.
//
// Config keys used (from ~/.dscli/config.dscli):
//   - lightpanda-local-url   (default: ws://127.2.2.9:9227)
//   - lightpanda-remote-url  (default: "")
//   - lightpanda-remote-token (default: "")
func Get(ctx context.Context, rawURL string) (string, error) {
	cdpURL := cdpEndpoint(rawURL)
	if cdpURL == "" {
		return "", fmt.Errorf("lightpanda remote URL not configured for %s, "+
			"set lightpanda-remote-url in ~/.dscli/config.dscli", rawURL)
	}

	markdown, err := getFromCDP(ctx, rawURL, cdpURL)
	if err != nil {
		if !isRemoteURL(rawURL) {
			return "", fmt.Errorf(
				"lightpanda 连接失败: %w\n\n请确保 lightpanda 已启动:\n"+
					"  lightpanda serve --obey-robots --host 127.2.2.9 --port 9227",
				err,
			)
		}
		return "", fmt.Errorf("lightpanda remote 连接失败: %w", err)
	}

	return markdown, nil
}

// cdpEndpoint returns the WebSocket URL for the lightpanda CDP endpoint,
// with remote token appended for remote connections.
func cdpEndpoint(rawURL string) string {
	if isRemoteURL(rawURL) {
		remoteURL := config.Get("lightpanda-remote-url", "")
		remoteToken := config.Get("lightpanda-remote-token", "")
		if remoteURL == "" {
			return ""
		}
		if remoteToken != "" {
			if strings.Contains(remoteURL, "?") {
				return remoteURL + "&token=" + remoteToken
			}
			return remoteURL + "?token=" + remoteToken
		}
		return remoteURL
	}
	return config.Get("lightpanda-local-url", "ws://127.2.2.9:9227")
}
