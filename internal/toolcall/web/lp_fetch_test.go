package web

import (
	"context"
	"strings"
	"testing"
)

// TestHandleLPFetchURLValidation verifies the handler rejects empty and
// scheme-less URLs before ever invoking lightpanda.
func TestHandleLPFetchURLValidation(t *testing.T) {
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
			_, _, err := handleLPFetch(context.Background(), map[string]any{"url": tt.url})
			if err == nil {
				t.Fatalf("handleLPFetch(url=%q) error = nil, want %q", tt.url, tt.want)
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("handleLPFetch(url=%q) error = %v, want containing %q", tt.url, err, tt.want)
			}
		})
	}
}
