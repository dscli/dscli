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
		// google.com — exact + subdomain suffix
		{"https://google.com/search?q=golang", true},
		{"https://www.google.com/", true},
		{"http://google.com", true},
		{"https://GOOGLE.COM", true},

		// gitlab.com
		{"https://gitlab.com", true},
		{"https://gitlab.com/explore", true},

		// github.com
		{"https://github.com", true},

		// googlesource.com — subdomain suffix
		{"https://go-review.googlesource.com/c/go/+/123", true},

		// duckduckgo.com — exact + subdomain suffix
		{"https://duckduckgo.com", true},
		{"https://lite.duckduckgo.com", true},

		// negative cases
		{"https://example.com", false},
		{"https://go.dev", false},
		{"https://www.google.cn", false},
		{"https://notgooglesource.com", false},

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
		withConfig(t, "lightpanda-remote-url", "")
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
func restoreFuncVars(avail func() bool, cmdExists func() bool, start func() error, usrSvc func() error, tried bool) {
	isLocalAvailable = avail
	lightpandaCmdExists = cmdExists
	startLightpanda = start
	setupUserService = usrSvc
	startTried = tried
}

// usrSvcFail is a setupUserService stub that always fails, used in tests
// that exercise the child-process fallback path.
func usrSvcFail() error { return fmt.Errorf("user service not available") }

func TestEnsureLocalLightpanda_CmdNotFound(t *testing.T) {
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, setupUserService, startTried)

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return false }
	setupUserService = usrSvcFail
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
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, setupUserService, startTried)

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	startLightpanda = func() error { return nil }
	setupUserService = usrSvcFail
	startTried = false

	if err := ensureLocalLightpanda(); err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
}

func TestEnsureLocalLightpanda_StartFailsOnce(t *testing.T) {
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, setupUserService, startTried)

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	startLightpanda = func() error { return fmt.Errorf("启动超时") }
	setupUserService = usrSvcFail
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
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, setupUserService, startTried)

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
	setupUserService = usrSvcFail
	startTried = false

	if err := ensureLocalLightpanda(); err != nil {
		t.Fatalf("expected no error (double-check passed), got: %v", err)
	}
}

// ---- User service tests ----

func TestEnsureLocalLightpanda_UserServiceSuccess(t *testing.T) {
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, setupUserService, startTried)

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	setupUserService = func() error { return nil } // user service succeeds
	startLightpanda = func() error {
		t.Error("startLightpanda should not be called when user service succeeds")
		return nil
	}
	startTried = false

	if err := ensureLocalLightpanda(); err != nil {
		t.Fatalf("expected success (user service path), got: %v", err)
	}
}

func TestEnsureLocalLightpanda_UserServiceFailsFallback(t *testing.T) {
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, setupUserService, startTried)

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	setupUserService = usrSvcFail // user service fails
	fallbackCalled := false
	startLightpanda = func() error {
		fallbackCalled = true
		return nil
	}
	startTried = false

	if err := ensureLocalLightpanda(); err != nil {
		t.Fatalf("expected success (fallback path), got: %v", err)
	}
	if !fallbackCalled {
		t.Error("expected fallback to startLightpanda after user service failure")
	}
}

func TestEnsureLocalLightpanda_BothFail(t *testing.T) {
	defer restoreFuncVars(isLocalAvailable, lightpandaCmdExists, startLightpanda, setupUserService, startTried)

	isLocalAvailable = func() bool { return false }
	lightpandaCmdExists = func() bool { return true }
	setupUserService = usrSvcFail
	startLightpanda = func() error { return fmt.Errorf("启动超时") }
	startTried = false

	// First call — both user service and child process fail.
	err := ensureLocalLightpanda()
	if err == nil {
		t.Fatal("expected error on first call")
	}

	// Second call — should not retry.
	err2 := ensureLocalLightpanda()
	if err2 == nil {
		t.Fatal("expected error on second call (already tried)")
	}
	if !strings.Contains(err2.Error(), "自动启动失败") {
		t.Errorf("expected '自动启动失败' error, got: %v", err2)
	}
}

func TestRemoteCDPEndpoint(t *testing.T) {
	t.Run("not configured", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "")
		_, err := remoteCDPEndpoint()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "未配置") {
			t.Errorf("expected '未配置' error, got: %v", err)
		}
	})

	t.Run("with token", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://remote.example.com/ws")
		withConfig(t, "lightpanda-remote-token", "secret123")
		got, err := remoteCDPEndpoint()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "wss://remote.example.com/ws?token=secret123"
		if got != want {
			t.Errorf("remoteCDPEndpoint() = %q, want %q", got, want)
		}
	})

	t.Run("without token", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://remote.example.com/ws")
		withConfig(t, "lightpanda-remote-token", "")
		got, err := remoteCDPEndpoint()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "wss://remote.example.com/ws"
		if got != want {
			t.Errorf("remoteCDPEndpoint() = %q, want %q", got, want)
		}
	})

	t.Run("with token and existing query", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://example.com/ws?key=val")
		withConfig(t, "lightpanda-remote-token", "tok")
		got, err := remoteCDPEndpoint()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "wss://example.com/ws?key=val&token=tok"
		if got != want {
			t.Errorf("remoteCDPEndpoint() = %q, want %q", got, want)
		}
	})
}

func TestGetRemote(t *testing.T) {
	var capturedCDPURL string
	var capturedRawURL string
	oldFn := getFromCDP
	getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
		capturedCDPURL = cdpURL
		capturedRawURL = rawURL
		return "# Remote content", nil
	}
	defer func() { getFromCDP = oldFn }()

	t.Run("uses remote endpoint for any URL", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://remote.example.com/ws")
		withConfig(t, "lightpanda-remote-token", "tok")

		// Even a local-only URL should go through remote.
		got, err := GetRemote(context.Background(), "https://go.dev")
		if err != nil {
			t.Fatalf("GetRemote failed: %v", err)
		}
		if capturedRawURL != "https://go.dev" {
			t.Errorf("rawURL = %q, want https://go.dev", capturedRawURL)
		}
		if capturedCDPURL != "wss://remote.example.com/ws?token=tok" {
			t.Errorf("cdpURL = %q, want wss://remote.example.com/ws?token=tok", capturedCDPURL)
		}
		if !strings.Contains(got, "Remote content") {
			t.Errorf("expected markdown content, got: %s", got)
		}
	})

	t.Run("error when remote not configured", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "")
		_, err := GetRemote(context.Background(), "https://go.dev")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "未配置") {
			t.Errorf("expected '未配置' error, got: %v", err)
		}
	})

	t.Run("CDP error wrapped", func(t *testing.T) {
		withConfig(t, "lightpanda-remote-url", "wss://remote.example.com/ws")
		getFromCDP = func(ctx context.Context, rawURL, cdpURL string) (string, error) {
			return "", errors.New("CDP timeout")
		}

		_, err := GetRemote(context.Background(), "https://example.com")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "lightpanda remote 连接失败") {
			t.Errorf("expected wrapped error, got: %v", err)
		}
		if !strings.Contains(err.Error(), "CDP timeout") {
			t.Errorf("expected original error in chain, got: %v", err)
		}
	})
}