package lp

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
)

// writeFakeBin creates a fake lightpanda binary on PATH that records its
// arguments one per line in argsFile and emits --json output whose shape
// depends on FAKE_MODE:
//
//	"" (default)   HTTP 200 with markdown content
//	block-direct   HTTP 0 without --http-proxy, HTTP 200 with it
//	status404      HTTP 404
//	empty          HTTP 200 with no content
//	interstitial   HTTP 200 with a Google anti-bot shell page
//	raw            non-JSON output
func writeFakeBin(t *testing.T, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "lightpanda")
	script := `#!/bin/sh
# Append (not overwrite) so multi-attempt flows are observable.
printf '%s\n' "$@" >> "$FAKE_ARGS_FILE"
case "$FAKE_MODE" in
  block-direct)
    # grep is not on PATH under t.Setenv, so match with a shell case.
    case " $* " in
      *" --http-proxy "*) echo '{"url":"u","http_status":200,"content":"# Fake Markdown\n"}' ;;
      *) echo '{"url":"u","http_status":0,"content":""}' ;;
    esac ;;
  status404) echo '{"url":"u","http_status":404,"content":"not found"}' ;;
  empty) echo '{"url":"u","http_status":200,"content":""}' ;;
  interstitial) echo '{"url":"u","http_status":200,"content":"Trouble accessing Google Search, please click here"}' ;;
  raw) echo 'this is not json' ;;
  *) echo '{"url":"u","http_status":200,"content":"# Fake Markdown\n"}' ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_ARGS_FILE", argsFile)
}

func readArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	// Consume the log so each subtest starts from an empty file.
	if err := os.Truncate(argsFile, 0); err != nil {
		t.Fatalf("truncate args file: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		return nil
	}
	return lines
}

// clearProxyConfig removes any proxy config keys.  Called at the start of
// each subtest (immediate) and again at its end, so values set by one
// subtest cannot leak into the next.
func clearProxyConfig(t *testing.T) {
	t.Helper()
	config.SetValue("lightpanda-http-proxy", nil)
	config.SetValue("lightpanda-proxy", nil)
	t.Cleanup(func() {
		config.SetValue("lightpanda-http-proxy", nil)
		config.SetValue("lightpanda-proxy", nil)
	})
}

func TestFetchArgs(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)

	t.Run("defaults", func(t *testing.T) {
		out, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("Fetch() = %q, want fake markdown", out)
		}
		want := []string{"fetch", "https://example.com", "--dump", "markdown", "--json",
			"--http-timeout", strconv.Itoa(probeTimeoutMS),
			"--terminate-ms", strconv.Itoa(terminateMS)}
		if got := readArgs(t, argsFile); strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("args = %v, want %v", got, want)
		}
	})

	t.Run("dump override", func(t *testing.T) {
		if _, err := Fetch(context.Background(), "https://example.com", FetchOptions{Dump: "html"}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := strings.Join(readArgs(t, argsFile), " "); !strings.Contains(got, "--dump html") {
			t.Errorf("args %q missing --dump html", got)
		}
	})

	t.Run("terminate override", func(t *testing.T) {
		if _, err := Fetch(context.Background(), "https://example.com", FetchOptions{TerminateMS: 5000}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := strings.Join(readArgs(t, argsFile), " "); !strings.Contains(got, "--terminate-ms 5000") {
			t.Errorf("args %q missing --terminate-ms 5000", got)
		}
	})

	t.Run("google goes straight through proxy", func(t *testing.T) {
		clearProxyConfig(t)
		config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")
		if _, err := Fetch(context.Background(), "https://www.google.com/search?q=go", FetchOptions{}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		args := readArgs(t, argsFile)
		got := strings.Join(args, " ")
		// Single attempt: proxy with the full timeout, no direct probe.
		if !strings.Contains(got, "--http-proxy socks5h://localhost:9999") {
			t.Errorf("args %q missing proxy", got)
		}
		if !strings.Contains(got, "--http-timeout "+strconv.Itoa(httpTimeoutMS)) {
			t.Errorf("args %q missing full http timeout", got)
		}
		if strings.Contains(got, "--http-timeout "+strconv.Itoa(probeTimeoutMS)) {
			t.Errorf("args %q must not contain the probe timeout", got)
		}
		if n := len(args); n != 11 {
			t.Errorf("expected a single invocation (11 args), got %d", n)
		}
	})

	t.Run("proxy from config alias", func(t *testing.T) {
		// lightpanda-proxy is the legacy key; it must still work when the
		// primary key is absent.
		config.SetValue("lightpanda-http-proxy", nil)
		config.Set("lightpanda-proxy", "socks5h://localhost:9998")
		if _, err := Fetch(context.Background(), "https://www.google.com/search?q=go", FetchOptions{}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := strings.Join(readArgs(t, argsFile), " "); !strings.Contains(got, "--http-proxy socks5h://localhost:9998") {
			t.Errorf("args %q missing legacy config proxy", got)
		}
	})

	t.Run("explicit proxy wins over config", func(t *testing.T) {
		clearProxyConfig(t)
		config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")
		if _, err := Fetch(context.Background(), "https://www.google.com/search?q=go",
			FetchOptions{Proxy: "socks5h://localhost:1111"}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := strings.Join(readArgs(t, argsFile), " "); !strings.Contains(got, "--http-proxy socks5h://localhost:1111") {
			t.Errorf("args %q: explicit proxy should override config", got)
		}
	})
}

func TestFetchProxyRetry(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)

	t.Run("direct failure retries via proxy", func(t *testing.T) {
		clearProxyConfig(t)
		t.Setenv("FAKE_MODE", "block-direct")
		config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")
		out, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("Fetch() = %q, want content from the proxy attempt", out)
		}
		attempts := readArgs(t, argsFile)
		if len(attempts) != 20 { // probe (9 args) + proxy retry (11 args)
			t.Fatalf("expected 2 attempts (20 args), got %d", len(attempts))
		}
		first := strings.Join(attempts[:9], " ")
		second := strings.Join(attempts[9:], " ")
		if !strings.Contains(first, "--http-timeout "+strconv.Itoa(probeTimeoutMS)) {
			t.Errorf("first attempt %q should use the probe timeout", first)
		}
		if !strings.Contains(second, "--http-proxy socks5h://localhost:9999") ||
			!strings.Contains(second, "--http-timeout "+strconv.Itoa(httpTimeoutMS)) {
			t.Errorf("second attempt %q should use proxy with full timeout", second)
		}
	})

	t.Run("direct block without proxy is an error", func(t *testing.T) {
		clearProxyConfig(t)
		t.Setenv("FAKE_MODE", "block-direct")
		_, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
		if err == nil || !strings.Contains(err.Error(), "HTTP status 0") {
			t.Errorf("Fetch() error = %v, want HTTP status 0", err)
		}
	})

	t.Run("google direct block gives targeted hint", func(t *testing.T) {
		clearProxyConfig(t)
		t.Setenv("FAKE_MODE", "block-direct")
		_, err := Fetch(context.Background(), "https://www.google.com/search?q=go", FetchOptions{})
		if err == nil || !strings.Contains(err.Error(), "Bing") {
			t.Errorf("Fetch() error = %v, want Bing hint", err)
		}
	})
}

func TestFetchResultValidation(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)

	tests := []struct {
		name    string
		mode    string
		url     string
		wantErr string
	}{
		{name: "http error", mode: "status404", wantErr: "HTTP 404"},
		{name: "empty content", mode: "empty", wantErr: "no content"},
		{name: "bot interstitial", mode: "interstitial", url: "https://www.google.com/search?q=x", wantErr: "anti-bot"},
		{name: "raw output", mode: "raw", wantErr: "parse --json"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FAKE_MODE", tt.mode)
			u := tt.url
			if u == "" {
				u = "https://example.com"
			}
			_, err := Fetch(context.Background(), u, FetchOptions{})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Fetch() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}

	t.Run("non-google interstitial passes through", func(t *testing.T) {
		// The interstitial detector only applies to Google hosts.
		t.Setenv("FAKE_MODE", "interstitial")
		out, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "Trouble accessing") {
			t.Errorf("Fetch() = %q, want raw content on non-Google hosts", out)
		}
	})
}

func TestNeedsProxy(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"https://www.google.com/search?q=go", true},
		{"https://google.com", true},
		{"https://mail.google.com/", true},
		{"https://www.youtube.com/watch?v=x", true},
		{"https://en.wikipedia.org/wiki/Go", true},
		{"https://x.com/foo", true},
		{"https://github.com/dscli/dscli", false},
		{"https://go.dev/doc", false},
		{"https://bing.com/search?q=x", false},
		{"not a url", false},
	}
	for _, tt := range tests {
		if got := needsProxy(tt.url); got != tt.want {
			t.Errorf("needsProxy(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}
}

func TestIsBotInterstitial(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		content string
		want    bool
	}{
		{name: "google sg_rel", url: "https://www.google.com/search?q=x", content: "If you're having trouble accessing Google Search, please click here", want: true},
		{name: "google sorry", url: "https://www.google.com/search?q=x", content: "Our systems have detected unusual traffic from your computer network", want: true},
		{name: "google normal page", url: "https://www.google.com/search?q=x", content: "<a href=\"/url?q=go.dev\">result</a>", want: false},
		{name: "non-google page with words", url: "https://example.com", content: "Our systems have detected unusual traffic in this article", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBotInterstitial(tt.url, tt.content); got != tt.want {
				t.Errorf("isBotInterstitial(%q, %q) = %v, want %v", tt.url, tt.content, got, tt.want)
			}
		})
	}
}

// TestRunFetchError verifies that a failing fetch surfaces stderr in the
// error message, so callers can diagnose 429/redirects.
func TestRunFetchError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "lightpanda")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'boom: 429 Too Many Requests' >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := runFetch(context.Background(), bin, "fetch", "https://example.com")
	if err == nil {
		t.Fatal("runFetch() error = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "boom: 429 Too Many Requests") {
		t.Errorf("error %q missing stderr content", msg)
	}
	if !strings.Contains(msg, "exit status") {
		t.Errorf("error %q missing exit status", msg)
	}
}

// TestFetchMissingBinary verifies the early error when lightpanda is not
// on PATH.
func TestFetchMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty PATH, no lightpanda
	_, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
	if err == nil {
		t.Fatal("Fetch() error = nil, want not-found error")
	}
	if !strings.Contains(err.Error(), "lightpanda not found") {
		t.Errorf("error = %v, want not-found message", err)
	}
}
