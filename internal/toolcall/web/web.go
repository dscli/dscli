package web

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/toolcall"
)

//go:embed web.md
var mcp_client_md string

func init() {
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

	// Register MCP tools from the LightPanda server as regular tools.
	// This makes them appear alongside other built-in tools with deterministic
	// ordering, and gives them usage tracking via the standard tool pipeline.
	mcpTools := lp.MCPToolList(context.Background())
	for _, t := range mcpTools {
		// Skip tools that may already be registered (safety check).
		name := t.Function.Name
		if _, exists := toolcall.GetToolDef(context.Background(), name); exists {
			continue
		}
		desc := t.Function.Description
		params := t.Function.Parameters
		toolcall.RegisterTool(toolcall.ToolDef{
			Name:        name,
			Description: desc,
			Strict:      true,
			Parameters:  params,
			Category:    "web",
			Handler: func(ctx context.Context, args toolcall.ToolArgs) (string, string, error) {
				argsRaw, _ := json.Marshal(args)
				if toolcall.HandleMCPCall != nil {
					return toolcall.HandleMCPCall(ctx, name, string(argsRaw))
				}
				return "", "", fmt.Errorf("MCP not available: %s", name)
			},
		})
	}
}

func handleMCPClientTool(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	target := toolcall.ToolArgsValue(args, "target", "local")
	return lp.HandleMCPClientTool(ctx, target)
}
