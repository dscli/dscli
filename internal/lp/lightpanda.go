// Package lp provides web page reading via LightPanda's MCP transport.
//
// # Architecture
//
//	url(req) +---------------+  stdio   +-------------+
//	-------->|  MCP Client    +--------->| lightpanda  |
//	        |  (mcp.go)      |          | mcp         |
//	<--------+                |<---------+             |
//	markdown +---------------+ markdown +-------------+
//
// The package uses LightPanda's native MCP server by default (lightpanda mcp
// subcommand, stdio transport). Each Get call spawns a fresh lightpanda mcp
// subprocess. For geo-restricted sites, use the mcp_client tool to switch
// to LightPanda Cloud's MCP/SSE endpoint.
//
// # Deprecated
//
// The older CDP transport (lightpanda serve + chromedp) was removed in v0.10.0.
// Use the MCP transport exclusively.
//
// # Usage
//
//	import "github.com/dscli/dscli/internal/lp"
//
//	markdown, err := lp.Get(ctx, "https://example.com")
package lp

import (
	"context"
	"fmt"
)

// ---- Function variables for test injection ----

// getFromMCP performs the actual MCP interaction to fetch markdown content.
var getFromMCP = defaultGetFromMCP

// getCloudMCP returns a cloud MCP client. Injectable for testing.
var getCloudMCP = func(ctx context.Context) (*MCPClient, error) {
	return getOrCreateCloudMCPClient()
}

// ---- Public API ----

// Get fetches a web page via LightPanda MCP and returns its markdown content.
// Each call spawns "lightpanda mcp" as a subprocess per call.
func Get(ctx context.Context, rawURL string) (string, error) {
	markdown, err := getFromMCP(ctx, rawURL)
	if err != nil {
		return "", fmt.Errorf("lightpanda mcp 连接失败: %w", err)
	}
	return prettyJSONInMarkdown(markdown), nil
}

// GetRemote fetches a web page via LightPanda Cloud MCP and returns its
// markdown content. Unlike Get, it uses the persistent cloud MCP singleton
// (SSE transport) rather than spawning a local subprocess.
// Requires lightpanda-remote-token to be configured.
func GetRemote(ctx context.Context, rawURL string) (string, error) {
	mc, err := getCloudMCP(ctx)
	if err != nil {
		return "", fmt.Errorf("lightpanda mcp 连接失败: %w", err)
	}
	text, err := mc.GetMarkdown(ctx, rawURL)
	if err != nil {
		return "", fmt.Errorf("lightpanda mcp 连接失败: %w", err)
	}
	return prettyJSONInMarkdown(text), nil
}
