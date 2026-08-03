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
//	refresh        markdown empty; html is a meta-refresh shell; the target
//	               URL (https://example.com/target) serves markdown
//	refresh-rel    like refresh but with a relative refresh target
//	empty-html     markdown empty; html has content (no refresh)
//	block-all      HTTP 0 even through the proxy
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
  status429) echo '{"url":"u","http_status":429,"content":"sorry"}' ;;
  empty) echo '{"url":"u","http_status":200,"content":""}' ;;
  interstitial) echo '{"url":"u","http_status":200,"content":"Trouble accessing Google Search, please click here"}' ;;
  raw) echo 'this is not json' ;;
  refresh)
    case " $* " in
      *" https://example.com/target "*) echo '{"url":"u","http_status":200,"content":"# Target Markdown\n"}' ;;
      *" --dump html "*) echo '{"url":"u","http_status":200,"content":"<head><meta http-equiv=\"refresh\" content=\"0;url=https://example.com/target\"></head>"}' ;;
      *) echo '{"url":"u","http_status":200,"content":""}' ;;
    esac ;;
  refresh-rel)
    case " $* " in
      *" --dump html "*) echo '{"url":"u","http_status":200,"content":"<meta http-equiv=\"refresh\" content=\"0;url=2024/post.html\">"}' ;;
      *" https://example.com/2024/post.html "*) echo '{"url":"u","http_status":200,"content":"# Relative Target\n"}' ;;
      *) echo '{"url":"u","http_status":200,"content":""}' ;;
    esac ;;
  refresh-loop)
    # Every page is an empty markdown shell that refreshes to the same URL,
    # so following the redirect must hit the cap and fail.
    case " $* " in
      *" --dump html "*) echo '{"url":"u","http_status":200,"content":"<meta http-equiv=\"refresh\" content=\"0;url=https://example.com/a\">"}' ;;
      *) echo '{"url":"u","http_status":200,"content":""}' ;;
    esac ;;
  empty-html)
    case " $* " in
      *" --dump html "*) echo '{"url":"u","http_status":200,"content":"<html><body><img src=\"x.png\"></body></html>"}' ;;
      *) echo '{"url":"u","http_status":200,"content":""}' ;;
    esac ;;
  block-all) echo '{"url":"u","http_status":0,"content":""}' ;;
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
	config.SetValue("lightpanda-additional-proxy-domains", nil)
	t.Cleanup(func() {
		config.SetValue("lightpanda-http-proxy", nil)
		config.SetValue("lightpanda-proxy", nil)
		config.SetValue("lightpanda-additional-proxy-domains", nil)
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
		want := []string{
			"fetch", "https://example.com", "--dump", "markdown", "--json",
			"--http-timeout", strconv.Itoa(probeTimeoutMS),
			"--terminate-ms", strconv.Itoa(terminateMS),
		}
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

	t.Run("force proxy skips the direct probe", func(t *testing.T) {
		clearProxyConfig(t)
		config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")
		if _, err := Fetch(context.Background(), "https://example.com",
			FetchOptions{ForceProxy: true}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		args := readArgs(t, argsFile)
		got := strings.Join(args, " ")
		// Single attempt through the proxy with the full timeout - example.com
		// is not in proxyDomains, so without ForceProxy this would probe first.
		if !strings.Contains(got, "--http-proxy socks5h://localhost:9999") {
			t.Errorf("args %q missing forced proxy", got)
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

	t.Run("force proxy without configured proxy still fetches", func(t *testing.T) {
		clearProxyConfig(t)
		// No proxy configured: ForceProxy has nothing to force, the direct
		// path must remain functional.
		if _, err := Fetch(context.Background(), "https://example.com",
			FetchOptions{ForceProxy: true}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := strings.Join(readArgs(t, argsFile), " "); strings.Contains(got, "--http-proxy") {
			t.Errorf("args %q must not contain a proxy flag", got)
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
		{name: "google 429 hints bing", mode: "status429", url: "https://www.google.com/search?q=x", wantErr: "Bing"},
		{name: "non-google 429 is generic", mode: "status429", wantErr: "HTTP 429"},
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

// TestFetchEmptyContentFallback covers the markdown-empty resolution: a
// meta-refresh shell is followed to its target, a page without convertible
// text falls back to html, and a page empty in every form keeps the error.
func TestFetchEmptyContentFallback(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)

	t.Run("meta refresh is followed to the target", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "refresh")
		out, err := Fetch(context.Background(), "https://example.com/start", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "Target Markdown") {
			t.Errorf("Fetch() = %q, want content from the refresh target", out)
		}
		// markdown(empty) + html(refresh shell) + markdown(target) = 3 probes.
		args := readArgs(t, argsFile)
		if len(args) != 27 {
			t.Errorf("expected 3 invocations (27 args), got %d: %v", len(args), args)
		}
		dumps := 0
		for _, a := range args {
			if a == "--dump" {
				dumps++
			}
		}
		if dumps != 3 { // one --dump per invocation
			t.Errorf("expected --dump on each of 3 invocations, got %d flags", dumps)
		}
	})

	t.Run("relative refresh target resolves against the page URL", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "refresh-rel")
		out, err := Fetch(context.Background(), "https://example.com/start", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "Relative Target") {
			t.Errorf("Fetch() = %q, want content from the relative refresh target", out)
		}
	})

	t.Run("refresh loop is capped", func(t *testing.T) {
		// The fake loop mode always serves an empty markdown shell that
		// refreshes to the same URL, so the chain must hit the cap and fail.
		t.Setenv("FAKE_MODE", "refresh-loop")
		_, err := Fetch(context.Background(), "https://example.com/loop", FetchOptions{})
		if err == nil || !strings.Contains(err.Error(), "no content") {
			t.Errorf("Fetch() error = %v, want no-content error after the cap", err)
		}
	})

	t.Run("no-text page falls back to html", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "empty-html")
		out, err := Fetch(context.Background(), "https://example.com/img", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "<img") {
			t.Errorf("Fetch() = %q, want html fallback content", out)
		}
	})

	t.Run("truly empty page still errors", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "empty")
		_, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
		if err == nil || !strings.Contains(err.Error(), "no content") {
			t.Errorf("Fetch() error = %v, want no-content error", err)
		}
	})
}

// TestFetchProxyDownHint verifies that a status-0 failure through a proxy
// names the proxy, so a dead ssh-socks tunnel is diagnosable instead of
// looking like a site outage.
func TestFetchProxyDownHint(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)
	t.Setenv("FAKE_MODE", "block-all")
	config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")

	_, err := Fetch(context.Background(), "https://example.com", FetchOptions{ForceProxy: true})
	if err == nil {
		t.Fatal("Fetch() error = nil, want error")
	}
	if !strings.Contains(err.Error(), "HTTP status 0") {
		t.Errorf("error %q missing HTTP status 0", err)
	}
	if !strings.Contains(err.Error(), "socks5h://localhost:9999") {
		t.Errorf("error %q missing the proxy URL", err)
	}
	if !strings.Contains(err.Error(), "proxy tunnel") {
		t.Errorf("error %q missing the tunnel hint", err)
	}
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

// TestNeedsProxyExtraDomains verifies user-configured domains are merged into
// the built-in list, in both array and comma-separated string forms.
func TestNeedsProxyExtraDomains(t *testing.T) {
	clearProxyConfig(t)
	config.SetValue("lightpanda-additional-proxy-domains", []any{"github.io", "Example.COM"})
	t.Cleanup(func() { config.SetValue("lightpanda-additional-proxy-domains", nil) })

	tests := []struct {
		url  string
		want bool
	}{
		{"https://lilianweng.github.io/posts/2026-06-24-scaling-laws/", true},
		{"https://foo.github.io", true},
		{"https://github.io", true},
		{"https://github.com/dscli/dscli", false}, // github.com, not github.io
		{"https://example.org", false},            // not in the configured list
	}
	for _, tt := range tests {
		if got := needsProxy(tt.url); got != tt.want {
			t.Errorf("needsProxy(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}

	// Comma-separated string form, with whitespace and an empty entry.
	config.SetValue("lightpanda-additional-proxy-domains", " gitlab.com ,,raw.githubusercontent.com ")
	for _, tt := range []struct {
		url  string
		want bool
	}{
		{"https://gitlab.com/foo", true},
		{"https://raw.githubusercontent.com/dscli/dscli/main/README.md", true},
		{"https://example.com", false},
	} {
		if got := needsProxy(tt.url); got != tt.want {
			t.Errorf("needsProxy(%q) = %v, want %v", tt.url, got, tt.want)
		}
	}

	// []string form, accepted defensively (the parser produces []any today).
	config.SetValue("lightpanda-additional-proxy-domains", []string{"GitHub.IO"})
	for _, tt := range []struct {
		url  string
		want bool
	}{
		{"https://lilianweng.github.io/posts/2026-06-24-scaling-laws/", true},
		{"https://example.com", false},
	} {
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

func TestParseOutputTarget(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		wantPath string
		wantLine int
		wantErr  string
	}{
		{name: "plain path", output: "notes.md", wantPath: "notes.md"},
		{name: "path with line", output: "notes.md:10", wantPath: "notes.md", wantLine: 10},
		{name: "line 1", output: "a.md:1", wantPath: "a.md", wantLine: 1},
		{name: "colon in dir", output: "dir:sub/notes.md", wantPath: "dir:sub/notes.md"},
		{name: "non-numeric suffix stays in path", output: "notes:v1.md", wantPath: "notes:v1.md"},
		{name: "path with line and colon", output: "a:b.md:5", wantPath: "a:b.md", wantLine: 5},
		{name: "zero line rejected", output: "notes.md:0", wantErr: "positive integer"},
		{name: "negative line rejected", output: "notes.md:-3", wantErr: "positive integer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path, line, err := parseOutputTarget(tt.output)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("parseOutputTarget(%q) error = %v, want containing %q", tt.output, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseOutputTarget(%q) error = %v", tt.output, err)
			}
			if path != tt.wantPath || line != tt.wantLine {
				t.Errorf("parseOutputTarget(%q) = (%q, %d), want (%q, %d)", tt.output, path, line, tt.wantPath, tt.wantLine)
			}
		})
	}
}

func TestWriteOutput(t *testing.T) {
	const content = "# Fetched\nbody\n"
	read := func(t *testing.T, path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		return string(data)
	}

	t.Run("plain path overwrites", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("old content\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path, content); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		if got := read(t, path); got != content {
			t.Errorf("file = %q, want %q", got, content)
		}
	})

	t.Run("missing file is created", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "new.md")
		if err := writeOutput(path, content); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		if got := read(t, path); got != content {
			t.Errorf("file = %q, want %q", got, content)
		}
	})

	t.Run("insert at line 1", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path+":1", "NEW\n"); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		want := "NEW\none\ntwo\n"
		if got := read(t, path); got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})

	t.Run("insert in the middle", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path+":2", "INS\n"); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		want := "one\nINS\ntwo\nthree\n"
		if got := read(t, path); got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})

	t.Run("insert without trailing newline in content", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("one\nthree\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path+":2", "two"); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		want := "one\ntwo\nthree\n"
		if got := read(t, path); got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})

	t.Run("insert beyond last line appends", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path+":99", "END\n"); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		want := "one\ntwo\nEND\n"
		if got := read(t, path); got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})

	t.Run("file without trailing newline", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, []byte("one\ntwo"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path+":2", "INS\n"); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		want := "one\nINS\ntwo"
		if got := read(t, path); got != want {
			t.Errorf("file = %q, want %q", got, want)
		}
	})

	t.Run("empty file insert at line 1", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := writeOutput(path+":1", "NEW\n"); err != nil {
			t.Fatalf("writeOutput() error = %v", err)
		}
		if got := read(t, path); got != "NEW\n" {
			t.Errorf("file = %q, want %q", got, "NEW\n")
		}
	})

	t.Run("invalid line rejected", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "a.md")
		err := writeOutput(path+":0", content)
		if err == nil || !strings.Contains(err.Error(), "positive integer") {
			t.Errorf("writeOutput() error = %v, want positive integer hint", err)
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Errorf("file should not be created on invalid target, stat err = %v", statErr)
		}
	})
}

// TestFetchOutput verifies that a successful fetch writes the result to the
// output file, the proxy-retry path writes exactly once, and a failed fetch
// leaves no file behind.
func TestFetchOutput(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeBin(t, argsFile)
	clearProxyConfig(t)

	t.Run("writes file and still returns content", func(t *testing.T) {
		outFile := filepath.Join(t.TempDir(), "out.md")
		out, err := Fetch(context.Background(), "https://example.com", FetchOptions{Output: outFile})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("Fetch() = %q, want content returned", out)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		if string(data) != out {
			t.Errorf("file = %q, want %q", string(data), out)
		}
	})

	t.Run("proxy retry writes once", func(t *testing.T) {
		clearProxyConfig(t)
		t.Setenv("FAKE_MODE", "block-direct")
		config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")
		outFile := filepath.Join(t.TempDir(), "out.md")
		if _, err := Fetch(context.Background(), "https://example.com", FetchOptions{Output: outFile}); err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		if !strings.Contains(string(data), "Fake Markdown") {
			t.Errorf("file = %q, want markdown from the proxy attempt", string(data))
		}
	})

	t.Run("failed fetch leaves no file", func(t *testing.T) {
		clearProxyConfig(t)
		t.Setenv("FAKE_MODE", "status404")
		outFile := filepath.Join(t.TempDir(), "out.md")
		if _, err := Fetch(context.Background(), "https://example.com", FetchOptions{Output: outFile}); err == nil {
			t.Fatal("Fetch() error = nil, want error")
		}
		if _, statErr := os.Stat(outFile); !os.IsNotExist(statErr) {
			t.Errorf("output file should not exist after failed fetch, stat err = %v", statErr)
		}
	})

	t.Run("insert at line keeps surrounding lines", func(t *testing.T) {
		outFile := filepath.Join(t.TempDir(), "out.md")
		if err := os.WriteFile(outFile, []byte("# Title\nold\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		out, err := Fetch(context.Background(), "https://example.com", FetchOptions{Output: outFile + ":2"})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		// The fake output ends with "\n", so it is inserted verbatim:
		// "# Title\n" + out + "old\n" with no extra blank line.
		want := "# Title\n" + out + "old\n"
		if string(data) != want {
			t.Errorf("file = %q, want %q", string(data), want)
		}
	})
}
