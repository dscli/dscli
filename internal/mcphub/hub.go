package mcphub

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"

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
	allConfigs []ServerConfig         // all configs loaded, for reconnection/switching
}

var globalHub = &Hub{
	servers: make(map[string]*serverConn),
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

	// Store all configs for reconnection/switching.
	h.allConfigs = configs

	if len(configs) == 0 {
		outfmt.Warn("mcphub: no MCP servers configured; MCP tools are unavailable. Define mcp-servers in config.dscli to enable them.")
		return nil
	}

	// Connect each logical server once (first enabled config wins).
	// With the Type field, a server can have local and cloud variants.
	// During init we connect the first enabled one — typically the local variant.
	for _, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		if _, exists := h.servers[cfg.Name]; exists {
			continue // already connected this server name
		}
		if err := h.connectLocked(ctx, cfg); err != nil {
			// Log the error but continue — a failed server shouldn't block others.
			outfmt.Warn("mcphub: failed to connect to %s: %v", cfg.Name, err)
			continue
		}
	}

	return nil
}

// maxToolDescriptionRunes caps MCP-provided tool descriptions. External MCP
// servers may ship very long descriptions that bloat every LLM context;
// the alltools "not too large" test asserts the same budget.
const maxToolDescriptionRunes = 1600

// truncateToolDescription caps an MCP-provided tool description at
// maxToolDescriptionRunes runes, keeping both ends (head + tail) with an
// ellipsis in the middle.
func truncateToolDescription(desc string) string {
	return toolcall.TruncateHeadTail(desc, maxToolDescriptionRunes)
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

		// Follow the server's schema strictness: lenient schemas (open
		// additionalProperties / no required) must not be forced into
		// strict mode, which OpenAI-compatible APIs would reject.
		strict := !isLenientSchema(params)

		toolName := t.Name // capture for closure
		handler := func(ctx context.Context, args toolcall.ToolArgs) (string, string, error) {
			return h.dispatchToServer(ctx, cfg.Name, toolName, args)
		}

		if err := toolcall.RegisterTool(toolcall.ToolDef{
			Name:        prefixedName,
			Description: truncateToolDescription(t.Description),
			Strict:      strict,
			Parameters:  params,
			Category:    cfg.Name,
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
// Transport is determined by the Type field:
//   - Type "cloud" → SSE transport
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
// It takes cfg.Command as the base URL and appends each element of
// cfg.Args as a query parameter parsed from "key=value" format.
// If an arg has no "=" it is treated as a bare key with no value
// (appended as "?key" or "&key").
func buildSSEEndpoint(cfg ServerConfig) string {
	endpoint := cfg.Command

	for _, arg := range cfg.Args {
		key, value, hasEquals := strings.Cut(arg, "=")
		if strings.Contains(endpoint, "?") {
			endpoint += "&"
		} else {
			endpoint += "?"
		}
		endpoint += url.QueryEscape(key)
		if hasEquals {
			endpoint += "=" + url.QueryEscape(value)
		}
	}

	return endpoint
}

// SwitchServerTransport replaces a server's connection with a new one
// that matches the specified target transport type ("local" or "cloud").
// This allows switching between stdio and SSE transports at runtime
// without restarting the application.
//
// The new connection is validated by listing tools before replacing the old one.
// If validation fails, the old connection is preserved.
func (h *Hub) SwitchServerTransport(ctx context.Context, name, target string) error {
	span, ctx := clog.StartSpanFromContext(ctx, "SwitchServerTransport")
	defer span.Finish()

	h.mu.Lock()
	defer h.mu.Unlock()

	conn, ok := h.servers[name]
	if !ok {
		return fmt.Errorf("mcphub: server %q not connected", name)
	}

	// Skip if already using the target transport type.
	if conn.config.Type == target {
		return nil
	}

	// Find a config matching the server name + target type.
	var cfg *ServerConfig
	for i := range h.allConfigs {
		if h.allConfigs[i].Name == name && h.allConfigs[i].Type == target {
			cfg = &h.allConfigs[i]
			break
		}
	}
	if cfg == nil {
		return fmt.Errorf("mcphub: no %q transport config for server %q; "+
			"define it in your mcp-servers config file", target, name)
	}

	mc, err := newClientForConfig(ctx, *cfg)
	if err != nil {
		return fmt.Errorf("mcphub: connect %s %s: %w", name, target, err)
	}

	// Validate the new connection before swapping.
	if _, err := mc.ListTools(ctx); err != nil {
		mc.Close()
		return fmt.Errorf("mcphub: validate %s %s: %w", name, target, err)
	}

	oldClient := conn.client
	conn.client = mc
	conn.config = *cfg
	oldClient.Close()
	return nil
}

// reconnect replaces the client for a disconnected server.
// It acquires the write lock internally and validates the new connection
// before swapping. If validation fails, the old (dead) client is preserved.
func (h *Hub) reconnect(ctx context.Context, serverName string) error {
	span, ctx := clog.StartSpanFromContext(ctx, "reconnect")
	defer span.Finish()

	h.mu.Lock()
	defer h.mu.Unlock()

	conn, ok := h.servers[serverName]
	if !ok {
		return fmt.Errorf("mcphub: server %q not connected", serverName)
	}

	// Find the config matching the current transport type.
	var cfg *ServerConfig
	for i := range h.allConfigs {
		if h.allConfigs[i].Name == serverName && h.allConfigs[i].Type == conn.config.Type {
			cfg = &h.allConfigs[i]
			break
		}
	}
	if cfg == nil {
		return fmt.Errorf("mcphub: no config for server %q", serverName)
	}

	// Close old client.
	conn.client.Close()

	// Create new client.
	mc, err := newClientForConfig(ctx, *cfg)
	if err != nil {
		return fmt.Errorf("mcphub: reconnect %s: %w", serverName, err)
	}

	// Validate the new connection before swapping.
	if _, err := mc.ListTools(ctx); err != nil {
		mc.Close()
		return fmt.Errorf("mcphub: reconnect validate %s: %w", serverName, err)
	}

	outfmt.Info("mcphub: reconnected %s (%s transport)", serverName, cfg.Type)
	conn.client = mc
	return nil
}

// dispatchToServer routes a tool call to the specified server.
// If the call fails with a transport-level error, it attempts a lazy reconnect
// and retries once. Tool-level errors (MCPToolError) are returned as-is.
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
	if err == nil {
		return text, "", nil
	}

	// Distinguish tool-level errors from transport-level errors.
	// MCPToolError means the tool ran but returned an error (e.g., URL unreachable).
	// A plain Go error means the connection itself is broken.
	var toolErr *MCPToolError
	if errors.As(err, &toolErr) {
		return "", "", err
	}

	// Transport-level error — attempt lazy reconnect and retry once.
	outfmt.Warn("mcphub: %s disconnected (call %s: %v); reconnecting...", serverName, toolName, err)
	if rerr := h.reconnect(ctx, serverName); rerr != nil {
		outfmt.Warn("mcphub: reconnect %s failed: %v", serverName, rerr)
		return "", "", err // return original error
	}

	// conn.client now points to the reconnected client (through pointer).
	text, err = conn.client.CallTool(ctx, toolName, argsRaw)
	return text, "", err
}

// doDispatch routes a tool call to the correct MCP server based on the
// tool name prefix. The tool name format is "serverName_toolName".
//
// For example, "code_search" dispatches to the "code" server with the
// tool name "search".
//
// If the tool name has no underscore prefix, all registered servers are
// tried in order (best-effort fallback for backward compatibility).
//
// Like dispatchToServer, transport-level errors trigger a lazy reconnect.
func (h *Hub) doDispatch(ctx context.Context, toolName, argsRaw string) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "mcphub.Dispatch")
	defer span.Finish()

	h.mu.RLock()
	if len(h.servers) == 0 {
		h.mu.RUnlock()
		return "", "", fmt.Errorf("mcphub: no MCP servers connected")
	}

	// Parse "serverName_toolName".
	serverName, actualToolName, found := strings.Cut(toolName, "_")
	if !found {
		h.mu.RUnlock()
		// No underscore — try all servers in order (backward compat fallback).
		return h.dispatchFallback(ctx, toolName, argsRaw)
	}

	conn, ok := h.servers[serverName]
	h.mu.RUnlock()

	if !ok {
		return "", "", fmt.Errorf("mcphub: unknown server %q in tool name %q", serverName, toolName)
	}

	// LLMs may emit an empty argument string for parameterless tools.
	if argsRaw == "" {
		argsRaw = "{}"
	}
	var args map[string]any
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		return "", "", fmt.Errorf("mcphub %s: invalid args: %w", toolName, err)
	}

	text, callErr := conn.client.CallTool(ctx, actualToolName, args)
	if callErr == nil {
		return text, "", nil
	}

	// Tool-level errors are returned directly.
	var toolErr *MCPToolError
	if errors.As(callErr, &toolErr) {
		return "", "", callErr
	}

	// Transport-level error — attempt lazy reconnect and retry once.
	outfmt.Warn("mcphub: %s disconnected (dispatch %s: %v); reconnecting...", serverName, toolName, callErr)
	if rerr := h.reconnect(ctx, serverName); rerr != nil {
		outfmt.Warn("mcphub: reconnect %s failed: %v", serverName, rerr)
		return "", "", callErr
	}

	text, callErr = conn.client.CallTool(ctx, actualToolName, args)
	return text, "", callErr
}

// dispatchFallback tries to call a tool on all connected servers in order.
// This is used for backward compatibility when the tool name doesn't have
// a server prefix.
func (h *Hub) dispatchFallback(ctx context.Context, toolName, argsRaw string) (string, string, error) {
	// LLMs may emit an empty argument string for parameterless tools.
	if argsRaw == "" {
		argsRaw = "{}"
	}
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

// isLenientSchema reports whether a server's tool schema is lenient, i.e. it
// accepts additional properties or omits required fields. Lenient servers
// (e.g. slingshot code MCP's lenientSchema) relax the schema so LLMs fail
// less often. dscli must not force strict mode for such tools: OpenAI-
// compatible APIs reject strict=true combined with additionalProperties
// that is not literally false.
func isLenientSchema(m map[string]any) bool {
	v, ok := m["additionalProperties"]
	if !ok {
		return false // dscli convention: absent → false → strict-compatible
	}
	switch t := v.(type) {
	case bool:
		return t
	default:
		// true, or a schema object ({}) — open to extra properties.
		return true
	}
}
