package web

import (
	"context"

	"github.com/dscli/dscli/internal/mcphub"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
)

func init() {
	// Set the MCP dispatch function — routes unknown tool calls to mcphub.
	toolcall.DispatchMCP = mcphub.Dispatch

	// Initialize the MCP hub — connects to user-configured MCP servers,
	// discovers their tools, and registers each with the
	// "serverName_toolName" naming convention.
	//
	// Init is safe to call multiple times; subsequent calls are no-ops.
	if err := mcphub.Init(context.Background()); err != nil {
		// MCP hub initialization errors are non-fatal — the application
		// continues working with native tools only.
		outfmt.Warn("web: mcphub init: %v", err)
	}
}
