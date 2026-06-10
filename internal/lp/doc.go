// Package lp provides web page reading via lightpanda browser with MCP (default)
// or CDP (deprecated) transport.
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
// subcommand, stdio transport). The older CDP transport (lightpanda serve +
// chromedp) remains available via the lightpanda_transport = "cdp" config key
// but is deprecated and will be removed in v0.10.0.
//
// MCP transport advantages over CDP:
//   - Self-contained: no need for a running serve process
//   - Simpler: no WebSocket, no chromedp dependency
//   - Same engine: calls the same Zig conversion code internally
//
// # Transport Configuration
//
// Config key (~/.dscli/config.dscli):
//
//	lightpanda_transport = mcp   # "mcp" (default) or "cdp" (deprecated)
//
// # Remote vs Local
//
// This distinction only applies to the CDP transport. With MCP, every call
// spawns a local lightpanda mcp subprocess. For geo-restricted sites, use
// LightPanda Cloud's MCP/SSE endpoint when available.
//
// # CDP-only config keys (deprecated)
//
// These keys are only used when lightpanda_transport = "cdp":
//
//	lightpanda-local-url   = ws://127.2.2.9:9227
//	lightpanda-remote-url  = wss://euwest.cloud.lightpanda.io/ws
//	lightpanda-remote-token = <token>
//
// # Usage
//
//	import "github.com/dscli/dscli/internal/lp"
//
//	markdown, err := lp.Get(ctx, "https://example.com")
package lp
