// Package mcphub tests for MCP hub functionality.
//
// Tests use in-memory MCP servers where possible, avoiding real subprocess
// launches. Pure logic tests (inputSchemaToMap, MCPToolError, config loading)
// need no MCP server at all.
package mcphub

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)


// testToolHandler is a concrete handler type for in-memory test servers.
// We use a wrapper function registerTool to convert to the generic SDK type.
type testToolHandler func(context.Context, *mcp.CallToolRequest, map[string]any) (*mcp.CallToolResult, any, error)

// registerTool wraps mcp.AddTool with explicit type parameters and a type
// conversion so we can pass testToolHandler values.
func registerTool(s *mcp.Server, t *mcp.Tool, h testToolHandler) {
	mcp.AddTool[map[string]any, any](s, t, mcp.ToolHandlerFor[map[string]any, any](h))
}

// ---------------------------------------------------------------------------
// inputSchemaToMap
// ---------------------------------------------------------------------------

func TestInputSchemaToMap_Nil(t *testing.T) {
	m := inputSchemaToMap(nil)
	if m == nil {
		t.Fatal("got nil")
	}
	if m["type"] != "object" {
		t.Errorf("type = %q, want object", m["type"])
	}
	if _, ok := m["properties"]; !ok {
		t.Error("missing properties")
	}
	if m["additionalProperties"] != false {
		t.Errorf("additionalProperties = %v, want false", m["additionalProperties"])
	}
}

func TestInputSchemaToMap_Map(t *testing.T) {
	in := map[string]any{
		"type":       "object",
		"properties": map[string]any{"foo": map[string]any{"type": "string"}},
	}
	m := inputSchemaToMap(in)
	if m["type"] != "object" {
		t.Errorf("type = %q, want object", m["type"])
	}
	props := m["properties"].(map[string]any)
	if _, ok := props["foo"]; !ok {
		t.Error("missing property foo")
	}
}

func TestInputSchemaToMap_JSONRaw(t *testing.T) {
	in := json.RawMessage(`{"type":"object","properties":{"bar":{"type":"number"}}}`)
	m := inputSchemaToMap(in)
	if m["type"] != "object" {
		t.Errorf("type = %q, want object", m["type"])
	}
	props := m["properties"].(map[string]any)
	if _, ok := props["bar"]; !ok {
		t.Error("missing property bar")
	}
}

func TestInputSchemaToMap_InvalidJSON(t *testing.T) {
	in := json.RawMessage(`invalid`)
	m := inputSchemaToMap(in)
	if m == nil {
		t.Fatal("got nil")
	}
	if m["type"] != "object" {
		t.Errorf("type = %q, want object", m["type"])
	}
}

func TestInputSchemaToMap_PreservesAdditionalProperties(t *testing.T) {
	in := map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": true,
	}
	m := inputSchemaToMap(in)
	if m["additionalProperties"] != true {
		t.Errorf("additionalProperties = %v, want true", m["additionalProperties"])
	}
}

func TestInputSchemaToMap_HandlesUnsupportedType(t *testing.T) {
	m := inputSchemaToMap(42)
	if m == nil {
		t.Fatal("got nil")
	}
	if m["type"] != "object" {
		t.Errorf("type = %q, want object", m["type"])
	}
}

// ---------------------------------------------------------------------------
// MCPToolError
// ---------------------------------------------------------------------------

func TestMCPToolError_NoContent(t *testing.T) {
	e := &MCPToolError{Tool: "test_tool"}
	msg := e.Error()
	if msg != "mcp tool test_tool returned error" {
		t.Errorf("got %q", msg)
	}
}

func TestMCPToolError_WithTextContent(t *testing.T) {
	e := &MCPToolError{
		Tool: "fail_tool",
		Content: []mcp.Content{
			&mcp.TextContent{Text: "something went wrong"},
		},
	}
	msg := e.Error()
	if msg != "mcp tool fail_tool returned error: something went wrong" {
		t.Errorf("got %q", msg)
	}
}

func TestMCPToolError_TruncatesLongText(t *testing.T) {
	long := strings.Repeat("x", 600)
	e := &MCPToolError{
		Tool: "long_tool",
		Content: []mcp.Content{
			&mcp.TextContent{Text: long},
		},
	}
	msg := e.Error()
	if len(msg) > 550 {
		t.Errorf("message too long: %d chars", len(msg))
	}
	if !strings.Contains(msg, "...") {
		t.Error("expected truncation marker")
	}
}

func TestMCPToolError_NonTextContent(t *testing.T) {
	e := &MCPToolError{
		Tool: "img_tool",
		Content: []mcp.Content{
			&mcp.ImageContent{Data: []byte("fake-image")},
		},
	}
	msg := e.Error()
	if msg != "mcp tool img_tool returned error" {
		t.Errorf("got %q", msg)
	}
}

func TestMCPToolError_MultipleItems_UsesFirstText(t *testing.T) {
	e := &MCPToolError{
		Tool: "multi_tool",
		Content: []mcp.Content{
			&mcp.ImageContent{Data: []byte("img")},
			&mcp.TextContent{Text: "first text"},
			&mcp.TextContent{Text: "second text"},
		},
	}
	msg := e.Error()
	if msg != "mcp tool multi_tool returned error: first text" {
		t.Errorf("got %q", msg)
	}
}

// ---------------------------------------------------------------------------
// loadServerConfigs
// ---------------------------------------------------------------------------

func TestLoadServerConfigs_NoUserConfig(t *testing.T) {
	config.Set("mcp-servers", "")
	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) == 0 {
		t.Fatal("expected at least built-in servers")
	}
	found := false
	for _, s := range servers {
		if s.Name == "lightpanda" {
			found = true
			if !s.Enabled {
				t.Error("lightpanda should be enabled by default")
			}
			if s.Command != "lightpanda" {
				t.Errorf("command = %q, want lightpanda", s.Command)
			}
			if s.Type != "local" {
				t.Errorf("type = %q, want local", s.Type)
			}
		}
	}
	if !found {
		t.Error("lightpanda not found in built-in servers")
	}
}

func TestLoadServerConfigs_UserConfigMerges(t *testing.T) {
	config.SetValue("mcp-servers", map[string]any{
		"my-custom": map[string]any{
			"command": "my-server",
			"args":    []any{"--port", "8080"},
			"enabled": true,
		},
		"lightpanda": map[string]any{
			"command": "custom-lightpanda",
			"args":    []any{},
			"enabled": false,
		},
	})
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var lp, custom bool
	for _, s := range servers {
		switch s.Name {
		case "lightpanda":
			lp = true
			if s.Command != "custom-lightpanda" {
				t.Errorf("lightpanda command = %q, want custom-lightpanda", s.Command)
			}
			if s.Enabled {
				t.Error("lightpanda should be disabled per user config")
			}
			// Type should still be local (overrides built-in but type preserved)
			if s.Type != "local" {
				t.Errorf("lightpanda type = %q, want local", s.Type)
			}
		case "my-custom":
			custom = true
			if s.Command != "my-server" {
				t.Errorf("my-custom command = %q, want my-server", s.Command)
			}
			if !s.Enabled {
				t.Error("my-custom should be enabled")
			}
			if s.Type != "local" {
				t.Errorf("my-custom type = %q, want local", s.Type)
			}
		}
	}
	if !lp {
		t.Error("lightpanda not found")
	}
	if !custom {
		t.Error("my-custom not found")
	}
}

func TestLoadServerConfigs_StringValueIgnored(t *testing.T) {
	// Old-format string value should be silently ignored,
	// returning only built-in servers.
	config.SetValue("mcp-servers", "some-file.yaml")
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(servers) == 0 {
		t.Fatal("expected at least built-in servers")
	}
}

func TestLoadServerConfigs_InvalidType(t *testing.T) {
	// Non-map, non-string value should produce an error.
	config.SetValue("mcp-servers", 123)
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	_, err := loadServerConfigs()
	if err == nil {
		t.Fatal("expected error for invalid config type")
	}
}



// ---------------------------------------------------------------------------
// Hub dispatch (in-memory MCP server)
// ---------------------------------------------------------------------------

// newTestMCP creates an in-memory MCP server with the specified tools,
// connects a client session, and wraps it in an MCPClient.
// The returned stop function shuts down the server session.
func newTestMCP(t *testing.T, ctx context.Context, tools []*mcp.Tool, handlers map[string]testToolHandler) (*MCPClient, func()) {
	t.Helper()

	server := mcp.NewServer(&mcp.Implementation{Name: "test-server", Version: "1.0.0"}, nil)
	for _, tool := range tools {
		h, ok := handlers[tool.Name]
		if !ok {
			t.Fatalf("no handler for tool %q", tool.Name)
		}
		registerTool(server, tool, h)
	}

	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}

	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "1.0.0"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}

	mc := &MCPClient{
		session: clientSession,
	}

	return mc, func() {
		clientSession.Close()
		serverSession.Wait()
	}
}

func TestHub_DoDispatch_Prefixed(t *testing.T) {
	ctx := context.Background()

	tools := []*mcp.Tool{
		{Name: "echo", Description: "echoes back the input", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"msg": map[string]any{"type": "string"}},
		}},
	}
	handlers := map[string]testToolHandler{
		"echo": func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			msg, _ := args["msg"].(string)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "echo: " + msg}},
			}, nil, nil
		},
	}

	mc, stop := newTestMCP(t, ctx, tools, handlers)
	defer stop()

	hub := &Hub{servers: map[string]*serverConn{
		"testecho": {client: mc, config: ServerConfig{Name: "testecho"}},
	}}

	result, warning, err := hub.doDispatch(ctx, "testecho_echo", `{"msg":"hello"}`)
	if err != nil {
		t.Fatalf("doDispatch error: %v", err)
	}
	if warning != "" {
		t.Errorf("unexpected warning: %s", warning)
	}
	if result != "echo: hello" {
		t.Errorf("result = %q, want %q", result, "echo: hello")
	}
}

func TestHub_DoDispatch_UnknownServer(t *testing.T) {
	ctx := context.Background()
	hub := &Hub{servers: map[string]*serverConn{
		"alpha": {config: ServerConfig{Name: "alpha"}},
	}}
	_, _, err := hub.doDispatch(ctx, "bravo_foo", `{}`)
	if err == nil {
		t.Fatal("expected error for unknown server")
	}
}

func TestHub_DoDispatch_NoServers(t *testing.T) {
	ctx := context.Background()
	hub := &Hub{servers: map[string]*serverConn{}}
	_, _, err := hub.doDispatch(ctx, "anything", `{}`)
	if err == nil {
		t.Fatal("expected error when no servers connected")
	}
}

func TestHub_DoDispatch_InvalidArgs(t *testing.T) {
	ctx := context.Background()
	hub := &Hub{servers: map[string]*serverConn{
		"testx": {config: ServerConfig{Name: "testx"}},
	}}
	_, _, err := hub.doDispatch(ctx, "testx_foo", `not json`)
	if err == nil {
		t.Fatal("expected error for invalid JSON args")
	}
}

func TestHub_DispatchFallback(t *testing.T) {
	ctx := context.Background()

	tools := []*mcp.Tool{
		{Name: "greet", Description: "greet someone", InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"name": map[string]any{"type": "string"}},
		}},
	}
	handlers := map[string]testToolHandler{
		"greet": func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			name, _ := args["name"].(string)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "hello " + name}},
			}, nil, nil
		},
	}

	mc, stop := newTestMCP(t, ctx, tools, handlers)
	defer stop()

	hub := &Hub{servers: map[string]*serverConn{
		"greeter": {client: mc, config: ServerConfig{Name: "greeter"}},
	}}

	// Call without prefix (fallback).
	result, warning, err := hub.doDispatch(ctx, "greet", `{"name":"world"}`)
	if err != nil {
		t.Fatalf("fallback dispatch error: %v", err)
	}
	if warning != "" {
		t.Errorf("unexpected warning: %s", warning)
	}
	if result != "hello world" {
		t.Errorf("result = %q, want %q", result, "hello world")
	}
}

func TestHub_DispatchFallback_TriesAllServers(t *testing.T) {
	ctx := context.Background()

	// Server 1: no matching tool.
	tools1 := []*mcp.Tool{
		{Name: "other", Description: "other tool"},
	}
	handlers1 := map[string]testToolHandler{
		"other": func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "other"}}}, nil, nil
		},
	}
	mc1, stop1 := newTestMCP(t, ctx, tools1, handlers1)
	defer stop1()

	// Server 2: has the matching tool.
	tools2 := []*mcp.Tool{
		{Name: "ping", Description: "ping"},
	}
	handlers2 := map[string]testToolHandler{
		"ping": func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "pong"}}}, nil, nil
		},
	}
	mc2, stop2 := newTestMCP(t, ctx, tools2, handlers2)
	defer stop2()

	hub := &Hub{servers: map[string]*serverConn{
		"svc1": {client: mc1, config: ServerConfig{Name: "svc1"}},
		"svc2": {client: mc2, config: ServerConfig{Name: "svc2"}},
	}}

	result, _, err := hub.doDispatch(ctx, "ping", `{}`)
	if err != nil {
		t.Fatalf("fallback dispatch error: %v", err)
	}
	if result != "pong" {
		t.Errorf("result = %q, want %q", result, "pong")
	}
}

func TestHub_DispatchFallback_AllFail(t *testing.T) {
	ctx := context.Background()

	tools := []*mcp.Tool{
		{Name: "exists", Description: "exists"},
	}
	handlers := map[string]testToolHandler{
		"exists": func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: "ok"}}}, nil, nil
		},
	}
	mc, stop := newTestMCP(t, ctx, tools, handlers)
	defer stop()

	hub := &Hub{servers: map[string]*serverConn{
		"s1": {client: mc, config: ServerConfig{Name: "s1"}},
	}}

	_, _, err := hub.doDispatch(ctx, "nope", `{}`)
	if err == nil {
		t.Fatal("expected error when no server can handle the tool")
	}
}

func TestHub_DoInit_AlreadyInitialized(t *testing.T) {
	hub := &Hub{servers: map[string]*serverConn{
		"already": {config: ServerConfig{Name: "already"}},
	}}
	err := hub.doInit(context.Background())
	if err != nil {
		t.Fatalf("doInit on already-initialized hub: %v", err)
	}
	if len(hub.servers) != 1 {
		t.Errorf("servers = %d, want 1", len(hub.servers))
	}
}

// ---------------------------------------------------------------------------
// dispatchToServer
// ---------------------------------------------------------------------------

func TestDispatchToServer_NotFound(t *testing.T) {
	hub := &Hub{servers: map[string]*serverConn{
		"alpha": {config: ServerConfig{Name: "alpha"}},
	}}
	_, _, err := hub.dispatchToServer(context.Background(), "nonexistent", "foo", nil)
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

func TestDispatchToServer_Success(t *testing.T) {
	ctx := context.Background()

	tools := []*mcp.Tool{
		{Name: "add", Description: "add two numbers"},
	}
	handlers := map[string]testToolHandler{
		"add": func(ctx context.Context, req *mcp.CallToolRequest, args map[string]any) (*mcp.CallToolResult, any, error) {
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: formatFloat(a + b)}},
			}, nil, nil
		},
	}

	mc, stop := newTestMCP(t, ctx, tools, handlers)
	defer stop()

	hub := &Hub{servers: map[string]*serverConn{
		"calc": {client: mc, config: ServerConfig{Name: "calc"}},
	}}

	result, _, err := hub.dispatchToServer(ctx, "calc", "add", toolcall.ToolArgs{"a": float64(1), "b": float64(2)})
	if err != nil {
		t.Fatalf("dispatchToServer error: %v", err)
	}
	if result != "3" {
		t.Errorf("result = %q, want %q", result, "3")
	}
}

// formatFloat formats a float64 without unnecessary trailing zeros.
func formatFloat(v float64) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ---------------------------------------------------------------------------
// Tool-level error handling (MCPToolError)
// ---------------------------------------------------------------------------

func TestHub_CallTool_IsError(t *testing.T) {
	ctx := context.Background()

	tools := []*mcp.Tool{
		{Name: "failable", Description: "may fail"},
	}
	handlers := map[string]testToolHandler{
		"failable": func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "something bad happened"}},
			}, nil, nil
		},
	}

	mc, stop := newTestMCP(t, ctx, tools, handlers)
	defer stop()

	_, err := mc.CallTool(ctx, "failable", nil)
	if err == nil {
		t.Fatal("expected MCPToolError")
	}
	var mcpErr *MCPToolError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("error type = %T, want *MCPToolError", err)
	}
	if mcpErr.Tool != "failable" {
		t.Errorf("tool = %q, want %q", mcpErr.Tool, "failable")
	}
}

// ---------------------------------------------------------------------------
// ServerConfig Type / IsSSE
// ---------------------------------------------------------------------------

func TestServerConfig_Type(t *testing.T) {
	tests := []struct {
		name       string
		cfg        ServerConfig
		wantIsSSE  bool
	}{
		{name: "cloud-type", cfg: ServerConfig{Type: "cloud"}, wantIsSSE: true},
		{name: "local-type", cfg: ServerConfig{Type: "local"}, wantIsSSE: false},
		{name: "empty-type", cfg: ServerConfig{Type: ""}, wantIsSSE: false},
		{name: "unknown-type", cfg: ServerConfig{Type: "foo"}, wantIsSSE: false},
		{name: "cloud-with-command", cfg: ServerConfig{Type: "cloud", Command: "https://example.com/mcp"}, wantIsSSE: true},
		{name: "local-with-url", cfg: ServerConfig{Type: "local", Command: "https://example.com/mcp"}, wantIsSSE: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.cfg.IsSSE()
			if got != tt.wantIsSSE {
				t.Errorf("IsSSE(%+v) = %v, want %v", tt.cfg, got, tt.wantIsSSE)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// loadServerConfigs with Type field
// ---------------------------------------------------------------------------

func TestLoadServerConfigs_TypeDefaults(t *testing.T) {
	config.SetValue("mcp-servers", map[string]any{
		"my-custom": map[string]any{
			"command": "my-server",
			"enabled": true,
		},
		"my-cloud": map[string]any{
			"type":    "cloud",
			"command": "https://cloud.example.com/mcp",
			"enabled": true,
		},
	})
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, s := range servers {
		switch s.Name {
		case "my-custom":
			if s.Type != "local" {
				t.Errorf("my-custom type = %q, want local", s.Type)
			}
		case "my-cloud":
			if s.Type != "cloud" {
				t.Errorf("my-cloud type = %q, want cloud", s.Type)
			}
			if !s.IsSSE() {
				t.Error("my-cloud should be SSE")
			}
		}
	}
}

func TestLoadServerConfigs_AllowsSameNameDifferentType(t *testing.T) {
	config.SetValue("mcp-servers", map[string]any{
		"lp-local": map[string]any{
			"name":    "my-server",
			"type":    "local",
			"command": "local-cmd",
			"args":    []any{"local"},
			"enabled": true,
		},
		"lp-cloud": map[string]any{
			"name":    "my-server",
			"type":    "cloud",
			"command": "https://cloud.example.com/mcp",
			"enabled": true,
		},
	})
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var localFound, cloudFound bool
	for _, s := range servers {
		if s.Name == "my-server" {
			switch s.Type {
			case "local":
				localFound = true
				if s.Command != "local-cmd" {
					t.Errorf("local command = %q, want local-cmd", s.Command)
				}
			case "cloud":
				cloudFound = true
				if s.Command != "https://cloud.example.com/mcp" {
					t.Errorf("cloud command = %q, want https://cloud.example.com/mcp", s.Command)
				}
			}
		}
	}
	if !localFound {
		t.Error("local variant not found")
	}
	if !cloudFound {
		t.Error("cloud variant not found")
	}
}

func TestLoadServerConfigs_NameFromMapKey(t *testing.T) {
	config.SetValue("mcp-servers", map[string]any{
		"my-custom-entry": map[string]any{
			"command": "my-server",
			"args":    []any{},
			"enabled": true,
		},
	})
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Name should come from the map key when no `name` field provided.
	found := false
	for _, s := range servers {
		if s.Name == "my-custom-entry" {
			found = true
			if s.Command != "my-server" {
				t.Errorf("command = %q, want %q", s.Command, "my-server")
			}
		}
	}
	if !found {
		t.Error("server with name 'my-custom-entry' not found")
	}
}

func TestLoadServerConfigs_MultipleServers(t *testing.T) {
	config.SetValue("mcp-servers", map[string]any{
		"lp-local": map[string]any{
			"name":    "lightpanda",
			"type":    "local",
			"command": "lightpanda",
			"args":    []any{"mcp"},
			"enabled": true,
		},
		"lp-cloud": map[string]any{
			"name":    "lightpanda",
			"type":    "cloud",
			"command": "https://euwest.cloud.lightpanda.io/mcp/sse",
			"args":    []any{"token=secret123"},
			"enabled": true,
		},
	})
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var localFound, cloudFound bool
	for _, s := range servers {
		if s.Name == "lightpanda" {
			switch s.Type {
			case "local":
				localFound = true
				if s.IsSSE() {
					t.Error("lightpanda local should not be SSE")
				}
			case "cloud":
				cloudFound = true
				if !s.IsSSE() {
					t.Error("lightpanda cloud should be SSE")
				}
			}
		}
	}
	if !localFound {
		t.Error("lightpanda local not found")
	}
	if !cloudFound {
		t.Error("lightpanda cloud not found")
	}
}

func TestLoadServerConfigs_NameOverridesMapKey(t *testing.T) {
	config.SetValue("mcp-servers", map[string]any{
		"some-key": map[string]any{
			"name":    "logical-name",
			"command": "my-server",
			"enabled": true,
		},
	})
	t.Cleanup(func() { config.SetValue("mcp-servers", nil) })

	servers, err := loadServerConfigs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The logical name should be "logical-name", not "some-key".
	found := false
	for _, s := range servers {
		if s.Name == "logical-name" {
			found = true
			if s.Command != "my-server" {
				t.Errorf("command = %q, want %q", s.Command, "my-server")
			}
		}
	}
	if !found {
		t.Error("server with name 'logical-name' not found")
	}

	// Ensure the map key "some-key" is NOT used as the name.
	for _, s := range servers {
		if s.Name == "some-key" {
			t.Error("map key should not be used as name when `name` field is provided")
		}
	}
}

// ---------------------------------------------------------------------------
// buildSSEEndpoint
// ---------------------------------------------------------------------------

func TestBuildSSEEndpoint(t *testing.T) {
	tests := []struct {
		name   string
		cfg    ServerConfig
		wantEP string
	}{
		{
			name: "no-args",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse",
			},
			wantEP: "https://example.com/mcp/sse",
		},
		{
			name: "with-key-value",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse",
				Args:    []string{"token=secret123"},
			},
			wantEP: "https://example.com/mcp/sse?token=secret123",
		},
		{
			name: "multiple-pairs",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse",
				Args:    []string{"key1=val1", "key2=val2"},
			},
			wantEP: "https://example.com/mcp/sse?key1=val1&key2=val2",
		},
		{
			name: "bare-key",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse",
				Args:    []string{"debug"},
			},
			wantEP: "https://example.com/mcp/sse?debug",
		},
		{
			name: "existing-params",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse?version=1",
				Args:    []string{"token=x"},
			},
			wantEP: "https://example.com/mcp/sse?version=1&token=x",
		},
		{
			name: "url-encode",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse",
				Args:    []string{"token=a b/c"},
			},
			wantEP: "https://example.com/mcp/sse?token=a+b%2Fc",
		},
		{
			name: "value-with-equals",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse",
				Args:    []string{"key=value=with=equals"},
			},
			wantEP: "https://example.com/mcp/sse?key=value%3Dwith%3Dequals",
		},
		{
			name: "empty-value",
			cfg: ServerConfig{
				Command: "https://example.com/mcp/sse",
				Args:    []string{"key="},
			},
			wantEP: "https://example.com/mcp/sse?key=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSSEEndpoint(tt.cfg)
			if got != tt.wantEP {
				t.Errorf("buildSSEEndpoint() = %q, want %q", got, tt.wantEP)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// reconnect
// ---------------------------------------------------------------------------

func TestHub_Reconnect_ServerNotFound(t *testing.T) {
	hub := &Hub{servers: map[string]*serverConn{}}
	err := hub.reconnect(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent server")
	}
}

func TestHub_Reconnect_NoConfig(t *testing.T) {
	hub := &Hub{
		servers: map[string]*serverConn{
			"test": {config: ServerConfig{Name: "test", Type: "local"}},
		},
		allConfigs: nil,
	}
	err := hub.reconnect(context.Background(), "test")
	if err == nil {
		t.Fatal("expected error when no config found")
	}
}

// dispatchToServer with a tool-level error (IsError=true) should return the
// error directly without attempting reconnect.
func TestDispatchToServer_ToolError(t *testing.T) {
	ctx := context.Background()

	tools := []*mcp.Tool{
		{Name: "failable", Description: "may fail"},
	}
	handlers := map[string]testToolHandler{
		"failable": func(ctx context.Context, req *mcp.CallToolRequest, _ map[string]any) (*mcp.CallToolResult, any, error) {
			return &mcp.CallToolResult{
				IsError: true,
				Content: []mcp.Content{&mcp.TextContent{Text: "tool level error"}},
			}, nil, nil
		},
	}

	mc, stop := newTestMCP(t, ctx, tools, handlers)
	defer stop()

	hub := &Hub{servers: map[string]*serverConn{
		"test": {client: mc, config: ServerConfig{Name: "test"}},
	}}

	_, _, err := hub.dispatchToServer(ctx, "test", "failable", nil)
	if err == nil {
		t.Fatal("expected error for tool-level failure")
	}
	var mcpErr *MCPToolError
	if !errors.As(err, &mcpErr) {
		t.Fatalf("error type = %T, want *MCPToolError", err)
	}
}
