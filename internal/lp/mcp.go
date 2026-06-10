package lp

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
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
			// Truncate very long error messages — the full content is
			// available via type assertion if callers need it.
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

// MCPClient wraps a LightPanda MCP session. It manages the lifecycle of a
// lightpanda mcp subprocess, providing a simple interface for web page
// content extraction via MCP tools.
//
// The client is safe for concurrent use (calls are serialized through an
// internal mutex because stdio transport cannot safely multiplex writes).
type MCPClient struct {
	cmd     *exec.Cmd
	session *mcp.ClientSession
	mu      sync.Mutex
}

// NewMCPClient starts lightpanda mcp and connects to it.
//
// It looks up the lightpanda binary via exec.LookPath, spawns it with the
// "mcp" subcommand, and establishes an MCP session. The caller must call
// Close when done.
func NewMCPClient(ctx context.Context) (*MCPClient, error) {
	path, err := exec.LookPath("lightpanda")
	if err != nil {
		return nil, fmt.Errorf("lightpanda not found in PATH: %w", err)
	}

	cmd := exec.CommandContext(ctx, path, "mcp")
	transport := &mcp.CommandTransport{Command: cmd}
	client := mcp.NewClient(&mcp.Implementation{
		Name:    "dscli",
		Version: "0.8.5",
	}, nil)

	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp connect: %w", err)
	}

	return &MCPClient{
		cmd:     cmd,
		session: session,
	}, nil
}

// Close shuts down the MCP session and kills the subprocess.
func (c *MCPClient) Close() error {
	return c.session.Close()
}

// GetMarkdown navigates to a URL and returns the page content as markdown.
// If the returned markdown is empty and no error occurred, it means the
// page loaded but had no extractable text content.
func (c *MCPClient) GetMarkdown(ctx context.Context, url string) (string, error) {
	return c.callTool(ctx, "markdown", map[string]any{"url": url})
}

// GetSemanticTree navigates to a URL and returns the page's simplified
// semantic DOM tree, optimized for AI reasoning about page structure.
func (c *MCPClient) GetSemanticTree(ctx context.Context, url string) (string, error) {
	return c.callTool(ctx, "semantic_tree", map[string]any{"url": url})
}

// Evaluate runs JavaScript in the page context and returns the result.
// If url is non-empty, the page navigates there first before evaluation.
func (c *MCPClient) Evaluate(ctx context.Context, script, url string) (string, error) {
	args := map[string]any{"script": script}
	if url != "" {
		args["url"] = url
	}
	return c.callTool(ctx, "evaluate", args)
}

// ListTools returns the list of tools available on the MCP server.
func (c *MCPClient) ListTools(ctx context.Context) ([]*mcp.Tool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	result, err := c.session.ListTools(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("mcp list tools: %w", err)
	}
	return result.Tools, nil
}

// callTool is the internal workhorse: serializes access to the session,
// calls the named tool, and handles both transport errors and tool-level
// errors (isError).
func (c *MCPClient) callTool(ctx context.Context, name string, args map[string]any) (string, error) {
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
