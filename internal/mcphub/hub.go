package mcphub

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

// serverConn holds a running MCP client connection.
type serverConn struct {
	config ServerConfig
	client *MCPClient
}

// Hub manages multiple MCP server connections.
type Hub struct {
	mu         sync.RWMutex
	servers    map[string]*serverConn // keyed by server name
	allConfigs []ServerConfig         // all configs loaded, for reconnection
}

var globalHub = &Hub{
	servers: make(map[string]*serverConn),
}

// mcpTarget overrides the transport selection for servers that support
// both local (stdio) and cloud (SSE) modes. Set by the mcp_client tool.
var mcpTarget string // "local" or "cloud"

// SetMCPServerTarget sets the target transport mode for MCP servers.
func SetMCPServerTarget(target string) {
	mcpTarget = target
}

// getMCPServerTarget returns the current target, defaulting to "local".
func getMCPServerTarget() string {
	if mcpTarget == "" {
		return "local"
	}
	return mcpTarget
}

// doInit initializes the hub with built-in and user-configured servers.
// It loads server configs, connects to enabled servers, and registers
// their tools in the toolcall framework with prefixed names.
//
// doInit is safe to call multiple times — subsequent calls are no-ops.
func (h *Hub) doInit(ctx context.Context) error {
	span, ctx := clog.StartSpanFromContext(ctx, "mcphub.Init")
	defer span.Finish()

	h.mu.Lock()
	defer h.mu.Unlock()

	// Already initialized?
	if len(h.servers) > 0 {
		return nil
	}

	configs, err := loadServerConfigs()
	if err != nil {
		return fmt.Errorf("mcphub: loading server configs: %w", err)
	}

	// Store all configs for reconnection.
	h.allConfigs = configs

	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		if err := h.connectLocked(ctx, cfg); err != nil {
			// Log the error but continue — a failed server shouldn't block others.
			outfmt.Warn("mcphub: failed to connect to %s: %v", cfg.Name, err)
			continue
		}
	}

	return nil
}

// connectLocked connects to an MCP server and registers its tools.
// Must be called with h.mu held.
func (h *Hub) connectLocked(ctx context.Context, cfg ServerConfig) error {
	span, ctx := clog.StartSpanFromContext(ctx, "connectLocked")
	defer span.Finish()

	mc, err := newClientForConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect %s: %w", cfg.Name, err)
	}

	tools, err := mc.ListTools(ctx)
	if err != nil {
		mc.Close()
		return fmt.Errorf("list tools from %s: %w", cfg.Name, err)
	}

	conn := &serverConn{
		config: cfg,
		client: mc,
	}
	h.servers[cfg.Name] = conn

	// Register each tool with the "serverName_toolName" naming convention.
	for _, t := range tools {
		prefixedName := cfg.Name + "_" + t.Name
		params := inputSchemaToMap(t.InputSchema)

		toolName := t.Name // capture for closure
		handler := func(ctx context.Context, args toolcall.ToolArgs) (string, string, error) {
			return h.dispatchToServer(ctx, cfg.Name, toolName, args)
		}

		if err := toolcall.RegisterTool(toolcall.ToolDef{
			Name:        prefixedName,
			Description: t.Description,
			Strict:      true,
			Parameters:  params,
			Category:    "web",
			Handler:     handler,
		}); err != nil {
			// If a tool is already registered (e.g., duplicate name),
			// log a warning and continue.
			outfmt.Warn("mcphub: registering %s: %v", prefixedName, err)
		}
	}

	return nil
}

// newClientForConfig creates the appropriate MCP client for the given config.
// Transport is determined by the command prefix:
//   - http:// or https:// → SSE transport
//   - otherwise → stdio transport (subprocess)
func newClientForConfig(ctx context.Context, cfg ServerConfig) (*MCPClient, error) {
	if cfg.IsSSE() {
		return newSSEClient(ctx, cfg)
	}
	return NewMCPClient(ctx, cfg.Command, cfg.Args)
}

// newSSEClient creates an MCP client connected via SSE transport.
// It uses cfg.Command as the endpoint URL and cfg.Args as key=value
// query parameter pairs.
func newSSEClient(ctx context.Context, cfg ServerConfig) (*MCPClient, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "newSSEClient")
	defer span.Finish()

	endpoint := buildSSEEndpoint(cfg)
	return NewSSEMCPClient(ctx, endpoint)
}

// buildSSEEndpoint builds the SSE endpoint URL from a server config.
// It takes cfg.Command as the base URL and appends cfg.Args as
// key=value query parameter pairs. Each pair of args becomes
// "?key=value" or "&key=value" if query params already exist.
func buildSSEEndpoint(cfg ServerConfig) string {
	endpoint := cfg.Command

	// Append args as key=value query parameters (paired).
	for i := 0; i < len(cfg.Args)-1; i += 2 {
		key := cfg.Args[i]
		value := cfg.Args[i+1]
		if strings.Contains(endpoint, "?") {
			endpoint += "&"
		} else {
			endpoint += "?"
		}
		endpoint += url.QueryEscape(key) + "=" + url.QueryEscape(value)
	}
	// If odd trailing arg, treat as key with empty value.
	if len(cfg.Args)%2 == 1 {
		key := cfg.Args[len(cfg.Args)-1]
		if strings.Contains(endpoint, "?") {
			endpoint += "&"
		} else {
			endpoint += "?"
		}
		endpoint += url.QueryEscape(key) + "="
	}

	return endpoint
}

// ReconnectLightpanda replaces the lightpanda server connection with a new one
// that matches the current cloud/local target. This allows switching transports
// at runtime without restarting the application.
//
// The new connection is validated by listing tools before replacing the old one.
// If validation fails, the old connection is preserved.
func (h *Hub) ReconnectLightpanda(ctx context.Context) error {
	span, ctx := clog.StartSpanFromContext(ctx, "ReconnectLightpanda")
	defer span.Finish()

	conn, ok := h.servers["lightpanda"]
	if !ok {
		return fmt.Errorf("mcphub: lightpanda not connected")
	}

	// Select the right config based on target.
	target := getMCPServerTarget()
	cfg := conn.config
	if target == "cloud" && !cfg.IsSSE() {
		// Look for an SSE config for lightpanda in allConfigs.
		if found := h.findConfig("lightpanda", true); found != nil {
			cfg = *found
		} else {
			// Fall back to legacy config keys for backward compatibility.
			endpoint := config.Get("lightpanda-cloud-url",
				"https://euwest.cloud.lightpanda.io/mcp/sse")
			token := config.Get("lightpanda-remote-token", "")
			if token != "" {
				if strings.Contains(endpoint, "?") {
					endpoint += "&token=" + url.QueryEscape(token)
				} else {
					endpoint += "?token=" + url.QueryEscape(token)
				}
			}
			cfg = ServerConfig{
				Name:    "lightpanda",
				Command: endpoint,
			}
		}
	} else if target != "cloud" && cfg.IsSSE() {
		// Switching back to local — find the stdio config.
		if found := h.findConfig("lightpanda", false); found != nil {
			cfg = *found
		} else {
			// Use built-in default.
			cfg = ServerConfig{
				Name:    "lightpanda",
				Command: "lightpanda",
				Args:    []string{"mcp"},
			}
		}
	}

	mc, err := newClientForConfig(ctx, cfg)
	if err != nil {
		return fmt.Errorf("reconnect lightpanda: %w", err)
	}

	// Validate the new connection before swapping.
	if _, err := mc.ListTools(ctx); err != nil {
		mc.Close()
		return fmt.Errorf("reconnect lightpanda: validate: %w", err)
	}

	oldClient := conn.client
	conn.client = mc
	conn.config = cfg
	oldClient.Close()
	return nil
}

// findConfig finds a server config by name and SSE status from allConfigs.
// It matches both exact name (e.g., "lightpanda") and prefixed names
// (e.g., "lightpanda-local", "lightpanda-cloud").
func (h *Hub) findConfig(name string, sse bool) *ServerConfig {
	for i := range h.allConfigs {
		if h.allConfigs[i].IsSSE() == sse &&
			(h.allConfigs[i].Name == name ||
				strings.HasPrefix(h.allConfigs[i].Name, name+"-")) {
			return &h.allConfigs[i]
		}
	}
	return nil
}

// dispatchToServer routes a tool call to the specified server.
func (h *Hub) dispatchToServer(ctx context.Context, serverName, toolName string, args toolcall.ToolArgs) (string, string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "dispatchToServer")
	defer span.Finish()

	h.mu.RLock()
	conn, ok := h.servers[serverName]
	h.mu.RUnlock()

	if !ok {
		return "", "", fmt.Errorf("mcphub: server %q not connected", serverName)
	}

	argsRaw := make(map[string]any)
	for k, v := range args {
		argsRaw[k] = v
	}

	text, err := conn.client.CallTool(ctx, toolName, argsRaw)
	if err != nil {
		return "", "", err
	}
	return text, "", nil
}

// doDispatch routes a tool call to the correct MCP server based on the
// tool name prefix. The tool name format is "serverName_toolName".
//
// For example, "lightpanda_markdown" dispatches to the "lightpanda" server
// with the tool name "markdown".
//
// If the tool name has no underscore prefix, all registered servers are
// tried in order (best-effort fallback for backward compatibility).
func (h *Hub) doDispatch(ctx context.Context, toolName, argsRaw string) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "mcphub.Dispatch")
	defer span.Finish()

	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.servers) == 0 {
		return "", "", fmt.Errorf("mcphub: no MCP servers connected")
	}

	// Parse "serverName_toolName".
	serverName, actualToolName, found := strings.Cut(toolName, "_")
	if !found {
		// No underscore — try all servers in order (backward compat fallback).
		return h.dispatchFallback(ctx, toolName, argsRaw)
	}

	conn, ok := h.servers[serverName]
	if !ok {
		return "", "", fmt.Errorf("mcphub: unknown server %q in tool name %q", serverName, toolName)
	}

	var args map[string]any
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		return "", "", fmt.Errorf("mcphub %s: invalid args: %w", toolName, err)
	}

	text, callErr := conn.client.CallTool(ctx, actualToolName, args)
	if callErr != nil {
		return "", "", callErr
	}
	return text, "", nil
}

// dispatchFallback tries to call a tool on all connected servers in order.
// This is used for backward compatibility when the tool name doesn't have
// a server prefix.
func (h *Hub) dispatchFallback(ctx context.Context, toolName, argsRaw string) (string, string, error) {
	var args map[string]any
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		return "", "", fmt.Errorf("mcphub %s: invalid args: %w", toolName, err)
	}

	var firstErr error
	for _, conn := range h.servers {
		text, err := conn.client.CallTool(ctx, toolName, args)
		if err == nil {
			return text, "", nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return "", "", fmt.Errorf("mcphub: no server could handle %q: %w", toolName, firstErr)
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
