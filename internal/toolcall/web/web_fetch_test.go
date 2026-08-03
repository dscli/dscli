package web

import (
	"context"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

// TestWebFetchToolSchema verifies the registered tool definition: the
// canonical name, and the parameter surface (url + dump + output only - no
// terminate-ms or proxy knobs to leak environment details to the AI).
func TestWebFetchToolSchema(t *testing.T) {
	def, ok := toolcall.GetToolDef(context.Background(), "web_fetch")
	if !ok {
		t.Fatal("tool web_fetch not registered")
	}
	// RegisterTool derives DisplayName from the name (GetToolDisplayName),
	// so the exact spelling is framework-generated.
	if def.DisplayName != "WebFetch" {
		t.Errorf("DisplayName = %q, want WebFetch", def.DisplayName)
	}
	if def.Category != "web" {
		t.Errorf("Category = %q, want web", def.Category)
	}
	props, ok := def.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("Parameters has no properties: %v", def.Parameters)
	}
	for _, key := range []string{"url", "dump", "output"} {
		if _, ok := props[key]; !ok {
			t.Errorf("missing parameter %q", key)
		}
	}
	for _, banned := range []string{"terminate-ms", "proxy"} {
		if _, ok := props[banned]; ok {
			t.Errorf("parameter %q should not be exposed", banned)
		}
	}
	required, ok := def.Parameters["required"].([]string)
	if !ok || len(required) != 1 || required[0] != "url" {
		t.Errorf("required = %v (type %T), want exactly [\"url\"]", def.Parameters["required"], def.Parameters["required"])
	}
}

// TestHandleWebFetchURLValidation verifies the handler rejects empty and
// scheme-less URLs before ever invoking lightpanda.
func TestHandleWebFetchURLValidation(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string // substring of the expected error
	}{
		{name: "empty url", url: "", want: "url is required"},
		{name: "no scheme", url: "example.com", want: "http:// or https://"},
		{name: "ftp scheme", url: "ftp://example.com", want: "http:// or https://"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := handleWebFetch(context.Background(), map[string]any{"url": tt.url})
			if err == nil {
				t.Fatalf("handleWebFetch(url=%q) error = nil, want %q", tt.url, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("handleWebFetch(url=%q) error = %v, want containing %q", tt.url, err, tt.want)
			}
		})
	}
}
