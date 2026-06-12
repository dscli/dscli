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
// The package uses LightPanda's native MCP server (lightpanda mcp subcommand,
// stdio transport). Each call spawns a fresh subprocess.
//
// # Modes
//
//   - local (default): spawns "lightpanda mcp" subprocess locally.
//   - cloud: connects to LightPanda Cloud MCP/SSE endpoint. Switch via
//     the mcp_client tool (target="cloud").
//
// # Cloud Configuration
//
//	lightpanda-cloud-url    = https://euwest.cloud.lightpanda.io/mcp/sse
//	lightpanda-remote-token = <token>
//
// # Deprecated
//
// The older CDP transport (lightpanda serve + chromedp) was removed in v0.10.0.
// Config keys lightpanda-local-url, lightpanda-remote-url, and
// lightpanda_transport are no longer used.
package lp

