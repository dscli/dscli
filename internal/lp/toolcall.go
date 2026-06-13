// Package lp provides LightPanda integration for web page interaction.
//
// MCP tool integration for the toolcall framework.
// The init function registers callbacks with toolcall so that MCP tools
// are dispatched via a persistent MCPClient singleton in HandleToolCall.
//
// Two MCP modes are supported:
//   - local: spawns "lightpanda mcp" subprocess (stdio). Default.
//   - cloud: connects to LightPanda Cloud SSE endpoint.
//
// The AI switches between modes via the mcp_client tool.
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

	// mcpClientSingleton is the shared local MCP client reused across all tool calls.
	// Created lazily on first activeMCPClient call; lives for the process lifetime.
	// Using a singleton preserves LightPanda frame state (goto -> evaluate, etc.).
	mcpClientMu        sync.Mutex
	mcpClientSingleton *MCPClient

	// cloudMCPClientSingleton is the shared cloud MCP client.
	// Created on first mcp_client(target="cloud") call.
	cloudMCPClientMu        sync.Mutex
	cloudMCPClientSingleton *MCPClient

	// mcpClientTarget controls which MCP client to use for tool calls.
	// Default "local". Set by the mcp_client tool.
	mcpClientTarget   string
	mcpClientTargetMu sync.Mutex
)

func init() {
	toolcall.HandleMCPCall = handleMCPCall
}

// activeMCPClient returns the MCP client for the current target ("local" or "cloud").
func activeMCPClient() (*MCPClient, error) {
	mcpClientTargetMu.Lock()
	target := mcpClientTarget
	mcpClientTargetMu.Unlock()

	switch target {
	case "cloud":
		return getOrCreateCloudMCPClient()
	default:
		return getOrCreateMCPClient()
	}
}

// MCPToolList lazily discovers tools from the LightPanda MCP server.
// It is called once per process lifetime; results are cached.
// Always discovers via LOCAL MCP regardless of active target.
// If discovery fails, it returns nil (tools are silently omitted).
func MCPToolList(ctx context.Context) []toolcall.Tool {
	mcpToolsMu.Lock()
	defer mcpToolsMu.Unlock()
	if mcpToolsDone {
		return mcpToolsCache
	}
	mcpToolsDone = true

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

// getOrCreateMCPClient returns the shared local MCP client singleton.
// Created on first call; reused for all subsequent calls.
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

// getOrCreateCloudMCPClient returns the shared cloud MCP client singleton.
// Created on first call; reused for all subsequent calls.
func getOrCreateCloudMCPClient() (*MCPClient, error) {
	cloudMCPClientMu.Lock()
	defer cloudMCPClientMu.Unlock()

	if cloudMCPClientSingleton != nil {
		return cloudMCPClientSingleton, nil
	}

	mc, err := NewCloudMCPClient(context.Background())
	if err != nil {
		return nil, err
	}
	cloudMCPClientSingleton = mc
	return mc, nil
}

// handleMCPCall dispatches a tool call to the active MCP client.
// Uses a persistent singleton so frame state persists across calls.
func handleMCPCall(ctx context.Context, toolName, argsRaw string) (result, warning string, err error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		return "", "", fmt.Errorf("mcp call %s: invalid args: %w", toolName, err)
	}

	mc, mcErr := activeMCPClient()
	if mcErr != nil {
		return "", "", fmt.Errorf("mcp %s: %w", toolName, mcErr)
	}

	text, callErr := mc.CallTool(ctx, toolName, args)
	if callErr != nil {
		return "", "", callErr
	}

	return text, "", nil
}

// HandleMCPClientTool is the handler for the mcp_client tool.
// It switches the active MCP target between "local" and "cloud".
func HandleMCPClientTool(ctx context.Context, target string) (result, warning string, err error) {
	switch target {
	case "local":
		mcpClientTargetMu.Lock()
		mcpClientTarget = "local"
		mcpClientTargetMu.Unlock()
		return "✅ 已切换到本地 MCP 模式，适用于访问无需代理的网站", "", nil

	case "cloud":
		// Initialize to validate connectivity before switching.
		if _, err := getOrCreateCloudMCPClient(); err != nil {
			return "", "", fmt.Errorf("❌ 云端 MCP 连接失败: %w", err)
		}
		mcpClientTargetMu.Lock()
		mcpClientTarget = "cloud"
		mcpClientTargetMu.Unlock()
		return "✅ 已切换到云端 MCP 模式，适用于 Google、Wikimedia 等需要代理的网站", "", nil

	default:
		return "", "", fmt.Errorf("无效的 target: %q，可选: local, cloud", target)
	}
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
