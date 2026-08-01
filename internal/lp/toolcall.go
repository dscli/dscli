// Package lp provides LightPanda integration for web page interaction.
//
// MCP tool integration for the toolcall framework.
package lp

import (
	"context"
	"fmt"
	"sync"

	"github.com/nanjj/clog"
)

var (
	// cloudMCPClientSingleton is the shared cloud MCP client.
	// Created on first mcp_client(target="cloud") call.
	cloudMCPClientMu        sync.Mutex
	cloudMCPClientSingleton *MCPClient

	// mcpClientTarget controls which MCP client to use for tool calls.
	// Default "local". Set by the mcp_client tool.
	mcpClientTarget   = "local"
	mcpClientTargetMu sync.Mutex
)

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

// HandleMCPClientTool is the handler for the mcp_client tool.
// It switches the active MCP target between "local" and "cloud".
func HandleMCPClientTool(ctx context.Context, target string) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleMCPClientTool")
	defer span.Finish()
	switch target {
	case "":
		mcpClientTargetMu.Lock()
		current := mcpClientTarget
		mcpClientTargetMu.Unlock()
		return fmt.Sprintf("当前 MCP 模式: %s（未执行切换）", current), "", nil

	case "local":
		mcpClientTargetMu.Lock()
		mcpClientTarget = "local"
		mcpClientTargetMu.Unlock()
		return "✅ 已切换到本地 MCP 模式，适用于访问无需代理的网站", "", nil

	case "cloud":
		mcpClientTargetMu.Lock()
		mcpClientTarget = "cloud"
		mcpClientTargetMu.Unlock()
		return "✅ 已切换到云端 MCP 模式，适用于 Google、Wikimedia 等需要代理的网站", "", nil

	default:
		return "", "", fmt.Errorf("无效的 target: %q，可选: local, cloud", target)
	}
}
