package lp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/config"
)

// withConfig sets a config value for the duration of the test (or sub-test).
func withConfig(t *testing.T, key, value string) {
	t.Helper()
	old := config.Get(key, "__unset__")
	config.Set(key, value)
	t.Cleanup(func() {
		if old == "__unset__" {
			config.Set(key, "")
		} else {
			config.Set(key, old)
		}
	})
}

func TestIsRemoteURL(t *testing.T) {
	tests := []struct {
		rawURL string
		want   bool
	}{
		{"https://google.com/search?q=golang", true},
		{"https://www.google.com/", true},
		{"http://google.com", true},
		{"https://GOOGLE.COM", true},

		{"https://example.com", false},
		{"https://go.dev", false},
		{"https://github.com", false},
		{"https://www.google.cn", false},

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

func TestLocalListenAddr(t *testing.T) {
	// Only test project-specific logic: config reading + default fallback.
	// url.Parse correctness is stdlib's responsibility.

	t.Run("default when unconfigured", func(t *testing.T) {
		host, port := localListenAddr()
		if host != "127.2.2.9" || port != "9227" {
			t.Errorf("localListenAddr() = (%q, %q), want (127.2.2.9, 9227)", host, port)
		}
	})

	t.Run("reads configured URL", func(t *testing.T) {
		withConfig(t, "lightpanda-local-url", "ws://0.0.0.0:9228")
		host, port := localListenAddr()
		if host != "0.0.0.0" || port != "9228" {
			t.Errorf("localListenAddr() = (%q, %q), want (0.0.0.0, 9228)", host, port)
		}
	})

	t.Run("invalid URL falls back to defaults", func(t *testing.T) {
		withConfig(t, "lightpanda-local-url", "not-a-::valid-url")
		host, port := localListenAddr()
		if host != "127.2.2.9" || port != "9227" {
			t.Errorf("localListenAddr() = (%q, %q), want (127.2.2.9, 9227)", host, port)
		}
	})
}

func TestCdpEndpoint(t *testing.T) {
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
		withConfig(t, "lightpanda-local-url", "ws://localhost:9222")
		gotURL, gotLocal := cdpEndpoint("https://example.com")
		if gotURL != "ws://localhost:9222" {
			t.Errorf("expected custom local URL, got %q", gotURL)
		}
		if !gotLocal {
			t.Errorf("expected isLocal=true for custom local URL")
		}
	})

	t.Run("remote-not-configured-fallback-to-local", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "")
		gotURL, gotLocal := cdpEndpoint("https://google.com")
		if gotURL != "ws://127.2.2.9:9227" {
			t.Errorf("expected fallback to local URL, got %q", gotURL)
		}
		if !gotLocal {
			t.Errorf("expected isLocal=true when remote not configured, got false")
		}
	})

	t.Run("remote-with-token", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://euwest.cloud.lightpanda.io/ws")
		withConfig(t, "lightpanda-remote-token", "secret123")
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
		withConfig(t, "lightpanda-remote-url", "wss://example.com/ws?key=val")
		withConfig(t, "lightpanda-remote-token", "tok")
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
		withConfig(t, "lightpanda-remote-url", "wss://example.com/ws")
		withConfig(t, "lightpanda-remote-token", "")
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
	// getFromCDP is replaced with a spy that captures the endpoint
	// and returns predictable content. This verifies Get passes the
	// correct cdpURL through the pipeline.
	var capturedCDPURL string
	oldFn := getFromCDP
	getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
		capturedCDPURL = cdpURL
		return "# Test\n\ncontent", nil
	}
	defer func() { getFromCDP = oldFn }()

	// Skip auto-start.
	oldAvail := isLocalAvailable
	isLocalAvailable = func() bool { return true }
	defer func() { isLocalAvailable = oldAvail }()

	t.Run("local endpoint", func(t *testing.T) {
		got, err := Get(context.Background(), "https://example.com")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if capturedCDPURL != "ws://127.2.2.9:9227" {
			t.Errorf("cdpURL = %q, want ws://127.2.2.9:9227", capturedCDPURL)
		}
		if !strings.Contains(got, "content") {
			t.Errorf("expected markdown content, got: %s", got)
		}
	})

	t.Run("remote endpoint with token", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://remote.example.com/ws")
		withConfig(t, "lightpanda-remote-token", "tok")

		got, err := Get(context.Background(), "https://google.com")
		if err != nil {
			t.Fatalf("Get failed: %v", err)
		}
		if capturedCDPURL != "wss://remote.example.com/ws?token=tok" {
			t.Errorf("cdpURL = %q, want wss://remote.example.com/ws?token=tok", capturedCDPURL)
		}
		if !strings.Contains(got, "content") {
			t.Errorf("expected markdown content, got: %s", got)
		}
	})
}

func TestGetRemoteFallbackToLocal(t *testing.T) {
	withConfig(t, "lightpanda-remote-url", "")

	var capturedCDPURL string
	oldFn := getFromCDP
	getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
		capturedCDPURL = cdpURL
		return "ok", nil
	}
	defer func() { getFromCDP = oldFn }()

	oldAvail := isLocalAvailable
	isLocalAvailable = func() bool { return true }
	defer func() { isLocalAvailable = oldAvail }()

	_, err := Get(context.Background(), "https://google.com")
	if err != nil {
		t.Fatalf("Get unexpectedly failed: %v", err)
	}
	if capturedCDPURL != "ws://127.2.2.9:9227" {
		t.Errorf("expected fallback to local, got cdpURL=%q", capturedCDPURL)
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

	t.Run("local error wraps with startup hint", func(t *testing.T) {
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
		// Error hint should include the actual host:port from config.
		if !strings.Contains(err.Error(), "127.2.2.9") {
			t.Errorf("error hint missing host, got: %v", err)
		}
	})

	t.Run("remote error wraps differently", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://remote.example.com/ws")
		withConfig(t, "lightpanda-remote-token", "tok")

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

// restoreFuncVars saves and restores all function variables used by
// ensureLocalLightpanda. Use with defer.
func restoreFuncVars(avail func() bool, cmdExists func() bool, start func() error, tried bool) {
	isLocalAvailable = avail
	lightpandaCmdExists = cmdExists
	startLightpanda = start
	startTried = tried
}

func TestEnsureLocalLightpanda_CmdNotFound(t *testing.T) {
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, startTried)

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
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, startTried)

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	startLightpanda = func() error { return nil }
	startTried = false

	if err := ensureLocalLightpanda(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestEnsureLocalLightpanda_StartFailsOnce(t *testing.T) {
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, startTried)

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
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, startTried)

	callCount := 0
	isLocalAvailable = func() bool {
		callCount++
		return callCount > 1 // First call false, second (inside lock) true
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
