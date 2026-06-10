package lp

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMCPToolError(t *testing.T) {
	err := &MCPToolError{
		Tool: "markdown",
		Content: []mcp.Content{
			&mcp.TextContent{Text: "Navigating to https://example.com"},
			&mcp.TextContent{Text: "Reason: CouldntResolveHost"},
		},
	}
	msg := err.Error()
	if !strings.Contains(msg, "markdown") {
		t.Errorf("expected tool name in error, got: %s", msg)
	}
	// Error() shows the first text content only.
	if !strings.Contains(msg, "Navigating to") {
		t.Errorf("expected first text content in message, got: %s", msg)
	}

	// Content field should be preserved for callers that want the full data.
	if len(err.Content) != 2 {
		t.Errorf("expected 2 content items, got %d", len(err.Content))
	}
}

func TestMCPToolError_Truncated(t *testing.T) {
	long := strings.Repeat("A", 1000)
	err := &MCPToolError{
		Tool:    "evaluate",
		Content: []mcp.Content{&mcp.TextContent{Text: long}},
	}
	msg := err.Error()
	if len(msg) >= 550 {
		t.Errorf("expected truncated message, got %d chars", len(msg))
	}
}

func TestMCPToolError_NoTextContent(t *testing.T) {
	err := &MCPToolError{
		Tool:    "click",
		Content: nil,
	}
	msg := err.Error()
	if !strings.Contains(msg, "click") {
		t.Errorf("expected tool name in message, got: %s", msg)
	}
}

// TestMCPGetWithStub verifies that Get uses getFromMCP when transport is mcp
// (the default) and that getFromMCP is called with the correct URL.
func TestMCPGetWithStub(t *testing.T) {
	var capturedURL string
	oldFn := getFromMCP
	getFromMCP = func(ctx context.Context, rawURL string) (string, error) {
		capturedURL = rawURL
		return "# MCP result", nil
	}
	defer func() { getFromMCP = oldFn }()

	got, err := Get(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if capturedURL != "https://example.com" {
		t.Errorf("url = %q, want https://example.com", capturedURL)
	}
	if !strings.Contains(got, "MCP result") {
		t.Errorf("expected mcp result content, got: %s", got)
	}
}

// TestMCPGetErrorWithStub verifies that errors from getFromMCP are properly
// wrapped with the MCP-specific error prefix.
func TestMCPGetErrorWithStub(t *testing.T) {
	oldFn := getFromMCP
	getFromMCP = func(ctx context.Context, rawURL string) (string, error) {
		return "", errors.New("connection refused")
	}
	defer func() { getFromMCP = oldFn }()

	_, err := Get(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "lightpanda mcp 连接失败") {
		t.Errorf("expected mcp connection error wrapper, got: %v", err)
	}
	if !strings.Contains(err.Error(), "connection refused") {
		t.Errorf("expected original error in chain, got: %v", err)
	}
}
