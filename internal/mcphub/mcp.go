// Package mcphub manages multiple MCP server connections and provides
// unified tool discovery and dispatch for the dscli tool framework.
//
// # Architecture
//
//	Agent tool call      toolcall.HandleToolCall
//	       │                      │
//	       ▼                      ▼
//	┌──────────────────────────────────────┐
//	│              mcphub.Dispatch         │
//	│                                      │
//	│  "lightpanda_markdown" → parse       │
//	│    server="lightpanda"               │
//	│    tool="markdown"                   │
//	│       │                              │
//	│       ▼                              │
//	│  lightpanda MCPClient.CallTool       │
//	└──────────────────────────────────────┘
//
// # Configuration
//
// MCP server definitions are in the `mcp-servers` block of config.dscli:
//
//	mcp-servers {
//	  server-id {
//	    name = lightpanda
//	    type = local
//	    command = lightpanda
//	    args = [mcp]
//	    enabled = true
//	  }
//	}
//
// Built-in servers:
//   - lightpanda: web page interaction via LightPanda MCP
//
// User-defined servers are defined in the mcp-servers config block.
//
package mcphub

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/dscli/dscli/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/nanjj/clog"
)

// MCPToolError is returned when an MCP tool call completes with IsError=true.
// Unlike a transport-level error (which would be returned as a Go error directly),
// this indicates the tool ran but its operation failed (e.g., URL unreachable).
type MCPToolError struct {
	Tool    string        // name of the tool that failed
	Content []mcp.Content // original response content, preserved for debugging
}

func (e *MCPToolError) Error() string {
	var b strings.Builder
	b.WriteString("mcp tool ")
	b.WriteString(e.Tool)
	b.WriteString(" returned error")
	for _, c := range e.Content {
		if tc, ok := c.(*mcp.TextContent); ok && tc.Text != "" {
			b.WriteString(": ")
			msg := tc.Text
			if len(msg) > 500 {
				msg = msg[:500] + "..."
			}
			b.WriteString(msg)
			break
		}
	}
	return b.String()
}

// MCPClient wraps an MCP session. It manages the lifecycle of an MCP server
// subprocess, providing a simple interface for tool calls.
//
// The client is safe for concurrent use (calls are serialized through an
// internal mutex because stdio transport cannot safely multiplex writes).
type MCPClient struct {
	cmd     *exec.Cmd
	session *mcp.ClientSession
	mu      sync.Mutex
}

// NewMCPClient starts an MCP server subprocess and connects to it.
// command is the executable path, args are the command-line arguments.
// The caller must call Close when done.
func NewMCPClient(ctx context.Context, command string, args []string) (*MCPClient, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "NewMCPClient")
	defer span.Finish()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append(cmd.Environ(), clog.TraceEnv(span)...)

	transport := &mcp.CommandTransport{Command: cmd}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "dscli",
		Version: version.Version,
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect to %s: %w", command, err)
	}

	return &MCPClient{
		cmd:     cmd,
		session: session,
	}, nil
}

// NewSSEMCPClient connects to an MCP server over SSE.
// The caller must call Close when done.
func NewSSEMCPClient(ctx context.Context, endpoint string) (*MCPClient, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "NewSSEMCPClient")
	defer span.Finish()

	transport := &mcp.SSEClientTransport{Endpoint: endpoint}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "dscli",
		Version: version.Version,
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp sse connect: %w", err)
	}

	return &MCPClient{
		session: session,
	}, nil
}

// Close shuts down the MCP session and kills the subprocess.
func (c *MCPClient) Close() error {
	span, _ := clog.StartSpanFromContext(context.Background(), "MCPClient.Close")
	defer span.Finish()
	return c.session.Close()
}

// ListTools returns the list of tools available on the MCP server.
func (c *MCPClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "ListTools")
	defer span.Finish()
	c.mu.Lock()
	defer c.mu.Unlock()
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}
	return result.Tools, nil
}

// CallTool calls a tool by name with the given arguments.
// The caller is responsible for calling Close after finishing.
func (c *MCPClient) CallTool(ctx context.Context, name string, args map[string]any) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "CallTool")
	defer span.Finish()
	return c.callTool(ctx, name, args)
}

// callTool is the internal workhorse: serializes access to the session,
// calls the named tool, and handles both transport errors and tool-level
// errors (isError).
func (c *MCPClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "callTool")
	defer span.Finish()
	c.mu.Lock()
	defer c.mu.Unlock()

	result, err := c.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		return "", fmt.Errorf("mcp call %s: %w", name, err)
	}

	if result.IsError {
		return "", &MCPToolError{Tool: name, Content: result.Content}
	}

	// Extract the first text content item.
	for _, c := range result.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			return tc.Text, nil
		}
	}

	return "", fmt.Errorf("mcp call %s: no text content in result", name)
}
