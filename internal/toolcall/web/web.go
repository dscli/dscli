package web

import (
	"context"
	_ "embed"
	"fmt"

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

	// Register the mcp_client tool so the AI can switch between local/cloud MCP
	// transports for any configured server.
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "mcp_client",
		Description: mcp_client_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"server": map[string]any{
					"type":        "string",
					"description": "MCP server name (e.g. \"code\")",
				},
				"target": map[string]any{
					"type":        "string",
					"enum":        []string{"local", "cloud"},
					"description": "MCP target: local (stdio) or cloud (SSE)",
				},
			},
			"required":             []string{"server"},
			"additionalProperties": false,
		},
		Category: "web",
		Handler:  handleMCPClientTool,
	})

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

func handleMCPClientTool(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleMCPClientTool")
	defer span.Finish()

	server := toolcall.ToolArgsValue(args, "server", "")
	target := toolcall.ToolArgsValue(args, "target", "")
	if server == "" {
		return "", "", fmt.Errorf("server is required (e.g. \"code\"); configure it in mcp-servers first")
	}

	// Switch the server transport in mcphub (generic — works for any server).
	if err := mcphub.SwitchServerTransport(ctx, server, target); err != nil {
		return "", "", err
	}

	return fmt.Sprintf("✅ 已切换到 %s MCP 模式（%s）", target, server), "", nil
}
