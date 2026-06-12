package lp

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestGet(t *testing.T) {
	// Replace getFromMCP with a spy that returns known content.
	oldFn := getFromMCP
	getFromMCP = func(ctx context.Context, rawURL string) (string, error) {
		return "# Test\n\ncontent for " + rawURL, nil
	}
	defer func() { getFromMCP = oldFn }()

	got, err := Get(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !strings.Contains(got, "content for https://example.com") {
		t.Errorf("expected markdown content, got: %s", got)
	}
}

func TestGetRemote_ErrorPath(t *testing.T) {
	// Replace getCloudMCP with a spy that returns an error.
	oldGetCloud := getCloudMCP
	called := false
	getCloudMCP = func(ctx context.Context) (*MCPClient, error) {
		called = true
		return nil, fmt.Errorf("cloud MCP unavailable")
	}
	defer func() { getCloudMCP = oldGetCloud }()

	_, err := GetRemote(context.Background(), "https://go.dev")
	if err == nil {
		t.Fatal("expected error from mock cloud client")
	}
	if !strings.Contains(err.Error(), "lightpanda mcp 连接失败") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
	if !called {
		t.Error("getCloudMCP was not called")
	}
}

func TestGetError(t *testing.T) {
	oldFn := getFromMCP
	getFromMCP = func(ctx context.Context, rawURL string) (string, error) {
		return "", fmt.Errorf("MCP connection refused")
	}
	defer func() { getFromMCP = oldFn }()

	_, err := Get(context.Background(), "https://example.com")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "lightpanda mcp 连接失败") {
		t.Errorf("expected wrapped error, got: %v", err)
	}
}
