package web

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/mcphub"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed web.md
var mcp_client_md string

func init() {
	// Set the MCP dispatch function — routes unknown tool calls to mcphub.
	toolcall.DispatchMCP = mcphub.Dispatch

	// Register the mcp_client tool so the AI can switch between local/cloud MCP.
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "mcp_client",
		Description: mcp_client_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"enum":        []string{"local", "cloud"},
					"description": "MCP target: local (default) or cloud",
				},
			},
			"required":             []string{"target"},
			"additionalProperties": false,
		},
		Category: "web",
		Handler:  handleMCPClientTool,
	})

	// Initialize the MCP hub — connects to built-in and user-configured
	// MCP servers, discovers their tools, and registers each with the
	// "serverName_toolName" naming convention (e.g. "lightpanda_markdown").
	//
	// Init is safe to call multiple times; subsequent calls are no-ops.
	if err := mcphub.Init(context.Background()); err != nil {
		// MCP hub initialization errors are non-fatal — the application
		// continues working with native tools only.
		outfmt.Warn("web: mcphub init: %v", err)
	}
}

func handleMCPClientTool(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleMCPClientTool")
	defer span.Finish()
	target := toolcall.ToolArgsValue(args, "target", "local")

	result, warning, err = lp.HandleMCPClientTool(ctx, target)
	if err != nil {
		return "", "", err
	}

	// Update the hub's target and reconnect lightpanda.
	mcphub.SetMCPServerTarget(target)
	if err := mcphub.ReconnectLightpanda(ctx); err != nil {
		return "", warning, fmt.Errorf("mcphub: reconnect lightpanda: %w", err)
	}

	return result, warning, nil
}
