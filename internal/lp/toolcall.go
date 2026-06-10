// Package lp provides LightPanda integration for web page interaction.
//
// MCP tool integration for the toolcall framework.
// The init function registers callbacks with toolcall so that MCP tools
// (markdown, semantic_tree, evaluate, goto, etc.) are included in GetAllTools
// and dispatched via a persistent MCPClient singleton in HandleToolCall.
package lp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/dscli/dscli/internal/toolcall"
)

var (
	mcpToolsMu    sync.Mutex
	mcpToolsDone  bool
	mcpToolsCache []toolcall.Tool

	// mcpClientSingleton is the shared MCP client reused across all tool calls.
	// Created lazily on first handleMCPCall; lives for the process lifetime.
	// Using a singleton preserves LightPanda frame state (goto -> evaluate, etc.).
	mcpClientMu         sync.Mutex
	mcpClientSingleton  *MCPClient
)

func init() {
	toolcall.MCPToolList = getMCPTools
	toolcall.HandleMCPCall = handleMCPCall
}

// getMCPTools lazily discovers tools from the LightPanda MCP server.
// It is called once per process lifetime; results are cached.
// If MCP transport is not active or discovery fails, it returns nil
// (tools are silently omitted from GetAllTools).
func getMCPTools(ctx context.Context) []toolcall.Tool {
	mcpToolsMu.Lock()
	defer mcpToolsMu.Unlock()
	if mcpToolsDone {
		return mcpToolsCache
	}
	mcpToolsDone = true

	if defaultTransport() != TransportMCP {
		return nil
	}

	mc, err := NewMCPClient(ctx)
	if err != nil {
		return nil
	}
	defer mc.Close()

	tools, err := mc.ListTools(ctx)
	if err != nil {
		return nil
	}

	mcpToolsCache = make([]toolcall.Tool, 0, len(tools))
	for _, t := range tools {
		params := inputSchemaToMap(t.InputSchema)
		mcpToolsCache = append(mcpToolsCache, toolcall.Tool{
			Type: "function",
			Function: toolcall.Function{
				Name:        t.Name,
				Description: t.Description,
				Strict:      true,
				Parameters:  params,
			},
		})
	}
	return mcpToolsCache
}

// getOrCreateMCPClient returns the shared MCP client singleton.
// Created on first call; reused for all subsequent calls so that
// LightPanda frame state persists (e.g., goto then evaluate without url).
func getOrCreateMCPClient() (*MCPClient, error) {
	mcpClientMu.Lock()
	defer mcpClientMu.Unlock()

	if mcpClientSingleton != nil {
		return mcpClientSingleton, nil
	}

	// Use background context so the subprocess outlives any single request.
	mc, err := NewMCPClient(context.Background())
	if err != nil {
		return nil, err
	}
	mcpClientSingleton = mc
	return mc, nil
}

// handleMCPCall dispatches a tool call to the LightPanda MCP server.
// It uses a persistent singleton MCP client so that frame state (navigation,
// scroll position, etc.) is preserved across consecutive tool calls.
// If MCP transport is not active, it returns an "unknown tool" error.
func handleMCPCall(ctx context.Context, toolName, argsRaw string) (result, warning string, err error) {
	if defaultTransport() != TransportMCP {
		err = fmt.Errorf("unknown tool: %s (not a registered dscli tool and MCP transport is not active)", toolName)
		return
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		return "", "", fmt.Errorf("mcp call %s: invalid args: %w", toolName, err)
	}

	mc, mcErr := getOrCreateMCPClient()
	if mcErr != nil {
		return "", "", fmt.Errorf("mcp %s: %w", toolName, mcErr)
	}

	text, callErr := mc.CallTool(ctx, toolName, args)
	if callErr != nil {
		return "", "", callErr
	}

	return text, "", nil
}

// inputSchemaToMap converts an MCP InputSchema (any) to a JSON Schema map.
// The MCP SDK returns InputSchema as map[string]any from the server.
// This handles both the common case and edge cases, and ensures
// additionalProperties=false to match dscli tool conventions.
func inputSchemaToMap(schema any) map[string]any {
	if schema == nil {
		return map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		}
	}
	var m map[string]any
	switch s := schema.(type) {
	case map[string]any:
		m = s
	case json.RawMessage:
		if err := json.Unmarshal(s, &m); err != nil {
			m = nil
		}
	}
	if m == nil {
		m = map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		}
	}
	// Ensure additionalProperties=false for dscli tool convention.
	if _, exists := m["additionalProperties"]; !exists {
		m["additionalProperties"] = false
	}
	return m
}
