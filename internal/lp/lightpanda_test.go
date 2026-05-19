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
			config.Set(name, "")
		} else {
			config.Set(name, val)
		}
	}

	t.Run("local", func(t *testing.T) {
		tests := []struct {
			name      string
			rawURL    string
			wantURL   string
			wantLocal bool
		}{
			{"default local", "https://example.com", "ws://127.2.2.9:9227", true},
			{"go.dev", "https://go.dev", "ws://127.2.2.9:9227", true},
			{"github", "https://github.com", "ws://127.2.2.9:9227", true},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				gotURL, gotLocal := cdpEndpoint(tt.rawURL)
				if gotURL != tt.wantURL {
					t.Errorf("cdpEndpoint(%q) url = %q, want %q", tt.rawURL, gotURL, tt.wantURL)
				}
				if gotLocal != tt.wantLocal {
					t.Errorf("cdpEndpoint(%q) isLocal = %v, want %v", tt.rawURL, gotLocal, tt.wantLocal)
				}
			})
		}
	})

	t.Run("local-custom", func(t *testing.T) {
		oldLocal := saveCfg("lightpanda-local-url")
		defer restore("lightpanda-local-url", oldLocal)

		config.Set("lightpanda-local-url", "ws://localhost:9222")
		gotURL, gotLocal := cdpEndpoint("https://example.com")
		if gotURL != "ws://localhost:9222" {
			t.Errorf("expected custom local URL, got %q", gotURL)
		}
		if !gotLocal {
			t.Errorf("expected isLocal=true for custom local URL")
		}
	})

	t.Run("remote-not-configured-fallback-to-local", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		defer restore("lightpanda-remote-url", oldRemote)

		config.Set("lightpanda-remote-url", "")
		gotURL, gotLocal := cdpEndpoint("https://google.com")
		if gotURL != "ws://127.2.2.9:9227" {
			t.Errorf("expected fallback to local URL, got %q", gotURL)
		}
		if !gotLocal {
			t.Errorf("expected isLocal=true when remote not configured, got false")
		}
	})

	t.Run("remote-with-token", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		oldToken := saveCfg("lightpanda-remote-token")
		defer restore("lightpanda-remote-url", oldRemote)
		defer restore("lightpanda-remote-token", oldToken)

		config.Set("lightpanda-remote-url", "wss://euwest.cloud.lightpanda.io/ws")
		config.Set("lightpanda-remote-token", "secret123")

		gotURL, gotLocal := cdpEndpoint("https://google.com")
		wantURL := "wss://euwest.cloud.lightpanda.io/ws?token=secret123"
		if gotURL != wantURL {
			t.Errorf("cdpEndpoint(google) = %q, want %q", gotURL, wantURL)
		}
		if gotLocal {
			t.Errorf("expected isLocal=false for remote, got true")
		}
	})

	t.Run("remote-with-token-existing-query", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		oldToken := saveCfg("lightpanda-remote-token")
		defer restore("lightpanda-remote-url", oldRemote)
		defer restore("lightpanda-remote-token", oldToken)

		config.Set("lightpanda-remote-url", "wss://example.com/ws?key=val")
		config.Set("lightpanda-remote-token", "tok")

		gotURL, gotLocal := cdpEndpoint("https://google.com")
		wantURL := "wss://example.com/ws?key=val&token=tok"
		if gotURL != wantURL {
			t.Errorf("cdpEndpoint(google) = %q, want %q", gotURL, wantURL)
		}
		if gotLocal {
			t.Errorf("expected isLocal=false for remote, got true")
		}
	})

	t.Run("remote-no-token", func(t *testing.T) {
		oldRemote := saveCfg("lightpanda-remote-url")
		oldToken := saveCfg("lightpanda-remote-token")
		defer restore("lightpanda-remote-url", oldRemote)
		defer restore("lightpanda-remote-token", oldToken)

		config.Set("lightpanda-remote-url", "wss://example.com/ws")
		config.Set("lightpanda-remote-token", "")

		gotURL, gotLocal := cdpEndpoint("https://www.google.com")
		if gotURL != "wss://example.com/ws" {
			t.Errorf("expected URL without token, got %q", gotURL)
		}
		if gotLocal {
			t.Errorf("expected isLocal=false for remote, got true")
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

	// Mock isLocalAvailable to always return true (skip auto-start).
	oldAvail := isLocalAvailable
	isLocalAvailable = func() bool { return true }
	defer func() { isLocalAvailable = oldAvail }()

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

func TestGetRemoteFallbackToLocal(t *testing.T) {
	// When remote is not configured, even google.com uses local.
	oldRemote := config.Get("lightpanda-remote-url", "__unset__")
	config.Set("lightpanda-remote-url", "")
	defer func() { restoreCfg("lightpanda-remote-url", oldRemote) }()

	oldFn := getFromCDP
	getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
		return fmt.Sprintf("CDP: %s", cdpURL), nil
	}
	defer func() { getFromCDP = oldFn }()

	oldAvail := isLocalAvailable
	isLocalAvailable = func() bool { return true }
	defer func() { isLocalAvailable = oldAvail }()

	got, err := Get(context.Background(), "https://google.com")
	if err != nil {
		t.Fatalf("Get unexpectedly failed: %v", err)
	}
	if !strings.Contains(got, "CDP: ws://127.2.2.9:9227") {
		t.Errorf("expected fallback to local, got: %s", got)
	}
}

func TestGetCDPError(t *testing.T) {
	oldFn := getFromCDP
	getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
		return "", errors.New("CDP connection refused")
	}
	defer func() { getFromCDP = oldFn }()

	oldAvail := isLocalAvailable
	isLocalAvailable = func() bool { return true }
	defer func() { isLocalAvailable = oldAvail }()

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

// ---- Auto-start tests ----

func TestEnsureLocalLightpanda_CmdNotFound(t *testing.T) {
	origAvail := isLocalAvailable
	origCmd := lightpandaCmdExists
	origStart := startLightpanda
	origTried := startTried
	defer func() {
		isLocalAvailable = origAvail
		lightpandaCmdExists = origCmd
		startLightpanda = origStart
		startTried = origTried
	}()

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return false }
	startTried = false

	err := ensureLocalLightpanda()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "未安装") {
		t.Errorf("expected '未安装' error, got: %v", err)
	}
}

func TestEnsureLocalLightpanda_StartSuccess(t *testing.T) {
	origAvail := isLocalAvailable
	origCmd := lightpandaCmdExists
	origStart := startLightpanda
	origTried := startTried
	defer func() {
		isLocalAvailable = origAvail
		lightpandaCmdExists = origCmd
		startLightpanda = origStart
		startTried = origTried
	}()

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	startLightpanda = func() error { return nil }
	startTried = false

	if err := ensureLocalLightpanda(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestEnsureLocalLightpanda_StartFailsOnce(t *testing.T) {
	origAvail := isLocalAvailable
	origCmd := lightpandaCmdExists
	origStart := startLightpanda
	origTried := startTried
	defer func() {
		isLocalAvailable = origAvail
		lightpandaCmdExists = origCmd
		startLightpanda = origStart
		startTried = origTried
	}()

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	startLightpanda = func() error { return fmt.Errorf("启动超时") }
	startTried = false

	// First call — attempts start, fails.
	err := ensureLocalLightpanda()
	if err == nil {
		t.Fatal("expected error on first call")
	}

	// Second call — should not retry (startTried is true).
	err2 := ensureLocalLightpanda()
	if err2 == nil {
		t.Fatal("expected error on second call (already tried)")
	}
	if !strings.Contains(err2.Error(), "自动启动失败") {
		t.Errorf("expected '自动启动失败' error, got: %v", err2)
	}
}

func TestEnsureLocalLightpanda_DoubleCheck(t *testing.T) {
	// After acquiring lock, double-check isLocalAvailable.
	// Simulate: first check returns false, but inside lock returns true.
	origAvail := isLocalAvailable
	origCmd := lightpandaCmdExists
	origStart := startLightpanda
	origTried := startTried
	defer func() {
		isLocalAvailable = origAvail
		lightpandaCmdExists = origCmd
		startLightpanda = origStart
		startTried = origTried
	}()

	callCount := 0
	isLocalAvailable = func() bool {
		callCount++
		return callCount > 1 // First call false, second call (inside lock) true
	}
	lightpandaCmdExists = func() bool { return true }
	startLightpanda = func() error {
		t.Error("startLightpanda should not be called when double-check passes")
		return nil
	}
	startTried = false

	if err := ensureLocalLightpanda(); err != nil {
		t.Fatalf("expected no error (double-check passed), got: %v", err)
	}
}

// restoreCfg restores a config value, used for test cleanup.
func restoreCfg(name, oldVal string) {
	if oldVal == "__unset__" {
		config.Set(name, "")
	} else {
		config.Set(name, oldVal)
	}
}