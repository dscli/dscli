package lp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/config"
)

func TestIsRemoteURL(t *testing.T) {
	tests := []struct {
		rawURL string
		want   bool
	}{
		// Remote hosts — should return true
		{"https://google.com/search?q=golang", true},
		{"https://www.google.com/", true},
		{"http://google.com", true},
		{"https://GOOGLE.COM", true},

		// Non-remote hosts — should return false
		{"https://example.com", false},
		{"https://go.dev", false},
		{"https://github.com", false},
		{"https://www.google.cn", false}, // different domain

		// Edge cases
		{"", false},
		{"invalid-url", false},
		{":invalid:", false},
	}
	for _, tt := range tests {
		t.Run(tt.rawURL, func(t *testing.T) {
			got := isRemoteURL(tt.rawURL)
			if got != tt.want {
				t.Errorf("isRemoteURL(%q) = %v, want %v", tt.rawURL, got, tt.want)
			}
		})
	}
}

func TestCdpEndpoint(t *testing.T) {
	// Save and restore config state.
	saveCfg := func(name string) string { return config.Get(name, "__unset__") }
	restore := func(name, val string) {
		if val == "__unset__" {
			config.Set(name, "") // clear
		} else {
			config.Set(name, val)
		}
	}

	t.Run("local", func(t *testing.T) {
		tests := []struct {
			name   string
			rawURL string
			want   string
		}{
			{"default local", "https://example.com", "ws://127.2.2.9:9227"},
			{"go.dev", "https://go.dev", "ws://127.2.2.9:9227"},
			{"github", "https://github.com", "ws://127.2.2.9:9227"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := cdpEndpoint(tt.rawURL)
				if got != tt.want {
					t.Errorf("cdpEndpoint(%q) = %q, want %q", tt.rawURL, got, tt.want)
				}
			})
		}
	})

	t.Run("local-custom", func(t *testing.T) {
		oldLocal := saveCfg("lightpanda-local-url")
		defer restore("lightpanda-local-url", oldLocal)

		config.Set("lightpanda-local-url", "ws://localhost:9222")
		got := cdpEndpoint("https://example.com")
		if got != "ws://localhost:9222" {
			t.Errorf("expected custom local URL, got %q", got)
		}
	})

	t.Run("remote-not-configured", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		defer restore("lightpanda-remote-url", oldRemote)

		config.Set("lightpanda-remote-url", "") // clear
		got := cdpEndpoint("https://google.com")
		if got != "" {
			t.Errorf("expected empty URL when remote not configured, got %q", got)
		}
	})

	t.Run("remote-with-token", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		oldToken := saveCfg("lightpanda-remote-token")
		defer restore("lightpanda-remote-url", oldRemote)
		defer restore("lightpanda-remote-token", oldToken)

		config.Set("lightpanda-remote-url", "wss://euwest.cloud.lightpanda.io/ws")
		config.Set("lightpanda-remote-token", "secret123")

		got := cdpEndpoint("https://google.com")
		want := "wss://euwest.cloud.lightpanda.io/ws?token=secret123"
		if got != want {
			t.Errorf("cdpEndpoint(google) = %q, want %q", got, want)
		}
	})

	t.Run("remote-with-token-existing-query", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		oldToken := saveCfg("lightpanda-remote-token")
		defer restore("lightpanda-remote-url", oldRemote)
		defer restore("lightpanda-remote-token", oldToken)

		config.Set("lightpanda-remote-url", "wss://example.com/ws?key=val")
		config.Set("lightpanda-remote-token", "tok")

		got := cdpEndpoint("https://google.com")
		want := "wss://example.com/ws?key=val&token=tok"
		if got != want {
			t.Errorf("cdpEndpoint(google) = %q, want %q", got, want)
		}
	})

	t.Run("remote-no-token", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		oldToken := saveCfg("lightpanda-remote-token")
		defer restore("lightpanda-remote-url", oldRemote)
		defer restore("lightpanda-remote-token", oldToken)

		config.Set("lightpanda-remote-url", "wss://example.com/ws")
		config.Set("lightpanda-remote-token", "")

		got := cdpEndpoint("https://www.google.com")
		if got != "wss://example.com/ws" {
			t.Errorf("expected URL without token, got %q", got)
		}
	})
}

func TestGet(t *testing.T) {
	// Replace getFromCDP with a mock that returns predictable markdown.
	oldFn := getFromCDP
	getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
		return fmt.Sprintf("# Mock\n\nURL: %s\nCDP: %s", rawURL, cdpURL), nil
	}
	defer func() { getFromCDP = oldFn }()

	t.Run("success-local", func(t *testing.T) {
		got, err := Get(context.Background(), "https://example.com")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !strings.Contains(got, "URL: https://example.com") {
			t.Errorf("expected URL in markdown, got: %s", got)
		}
		if !strings.Contains(got, "CDP: ws://127.2.2.9:9227") {
			t.Errorf("expected local CDP URL, got: %s", got)
		}
	})

	t.Run("success-remote", func(t *testing.T) {
		oldRemote := config.Get("lightpanda-remote-url", "__unset__")
		oldToken := config.Get("lightpanda-remote-token", "__unset__")
		config.Set("lightpanda-remote-url", "wss://remote.example.com/ws")
		config.Set("lightpanda-remote-token", "tok")
		defer func() {
			restoreCfg("lightpanda-remote-url", oldRemote)
			restoreCfg("lightpanda-remote-token", oldToken)
		}()

		got, err := Get(context.Background(), "https://google.com")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if !strings.Contains(got, "CDP: wss://remote.example.com/ws?token=tok") {
			t.Errorf("expected remote CDP URL, got: %s", got)
		}
	})
}

func TestGetRemoteNotConfigured(t *testing.T) {
	oldRemote := config.Get("lightpanda-remote-url", "__unset__")
	config.Set("lightpanda-remote-url", "")
	defer func() { restoreCfg("lightpanda-remote-url", oldRemote) }()

	_, err := Get(context.Background(), "https://google.com")
	if err == nil {
		t.Fatal("expected error for unconfigured remote, got nil")
	}
	if !strings.Contains(err.Error(), "remote URL not configured") {
		t.Errorf("expected 'remote URL not configured' error, got: %v", err)
	}
}

func TestGetCDPError(t *testing.T) {
	oldFn := getFromCDP
	getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
		return "", errors.New("CDP connection refused")
	}
	defer func() { getFromCDP = oldFn }()

	t.Run("local-error", func(t *testing.T) {
		_, err := Get(context.Background(), "https://example.com")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "lightpanda 连接失败") {
			t.Errorf("expected wrapped local error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "CDP connection refused") {
			t.Errorf("expected original error in chain, got: %v", err)
		}
	})

	t.Run("remote-error", func(t *testing.T) {
		oldRemote := config.Get("lightpanda-remote-url", "__unset__")
		oldToken := config.Get("lightpanda-remote-token", "__unset__")
		config.Set("lightpanda-remote-url", "wss://remote.example.com/ws")
		config.Set("lightpanda-remote-token", "tok")
		defer func() {
			restoreCfg("lightpanda-remote-url", oldRemote)
			restoreCfg("lightpanda-remote-token", oldToken)
		}()

		_, err := Get(context.Background(), "https://google.com")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "lightpanda remote 连接失败") {
			t.Errorf("expected wrapped remote error, got: %v", err)
		}
	})
}

// restoreCfg restores a config value, used for test cleanup.
func restoreCfg(name, oldVal string) {
	if oldVal == "__unset__" {
		config.Set(name, "")
	} else {
		config.Set(name, oldVal)
	}
}
