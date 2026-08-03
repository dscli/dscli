package lp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"strconv"
	"strings"

	"github.com/dscli/dscli/internal/config"
	"github.com/nanjj/clog"
)

const (
	// httpTimeoutMS is the --http-timeout for the final attempt.  lightpanda's
	// default is 10000ms (10s), which is too tight for slow sites over a proxy;
	// we fix it to 5 minutes.  The tool call timeout (toolcall/web) is the
	// ultimate backstop.
	httpTimeoutMS = 300000

	// probeTimeoutMS caps the direct attempt.  A blocked host (e.g. Google
	// from behind the GFW) usually hangs until the transfer timeout, so the
	// probe fails fast and the proxy retry takes over instead of stalling.
	probeTimeoutMS = 20000

	// terminateMS is the default --terminate-ms: page JavaScript is killed
	// after this long, so endless-script pages cannot hold the fetch until
	// httpTimeoutMS.  Normal pages finish their scripts in seconds and are
	// unaffected.
	terminateMS = 60000
)

// proxyDomains are hosts that commonly require a proxy in restricted
// networks.  Fetches to them skip the direct probe and go straight through
// the configured proxy (config key lightpanda-http-proxy).
var proxyDomains = []string{
	"google.com", "googleapis.com", "gstatic.com", "googleusercontent.com",
	"youtube.com", "youtu.be",
	"wikipedia.org", "wikimedia.org",
	"twitter.com", "x.com",
	"facebook.com", "instagram.com",
	"telegram.org", "t.me",
}

// FetchOptions configures a lightpanda fetch invocation.  Zero values mean
// "use lightpanda's default" (except Dump, which defaults to markdown).
type FetchOptions struct {
	Dump        string // markdown | html | semantic_tree | semantic_tree_text
	TerminateMS int    // JS deadline in ms; 0 uses terminateMS
	Proxy       string // --http-proxy URL; empty falls back to config lightpanda-http-proxy
}

// Fetch runs `lightpanda fetch` for a single URL and returns its dump text.
//
// Each call spawns a fresh process and exits - unlike the MCP server there
// is no long-running process, so a hung page cannot poison later calls.
// The dump output is returned verbatim; callers should prefer markdown or
// semantic_tree over html for large pages.
//
// The proxy comes from opts.Proxy, or from the lightpanda-http-proxy value
// in config when unset.  Hosts known to need a proxy (see proxyDomains) go
// straight through it; other hosts are fetched directly first and retried
// via the proxy when the direct attempt fails or returns nothing.
func Fetch(ctx context.Context, rawURL string, opts FetchOptions) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "Fetch")
	defer span.Finish()

	path, err := exec.LookPath("lightpanda")
	if err != nil {
		return "", fmt.Errorf("lightpanda not found in PATH: %w", err)
	}

	dump := opts.Dump
	if dump == "" {
		dump = "markdown"
	}
	term := opts.TerminateMS
	if term == 0 {
		term = terminateMS
	}
	proxy := opts.Proxy
	if proxy == "" {
		proxy = config.Get("lightpanda-http-proxy", "", "lightpanda-proxy")
	}

	// Known blocked domains go straight through the proxy; probing them
	// directly would just burn the probe timeout.
	if proxy != "" && needsProxy(rawURL) {
		return fetchOnce(ctx, path, rawURL, dump, proxy, httpTimeoutMS, term)
	}

	out, err := fetchOnce(ctx, path, rawURL, dump, "", probeTimeoutMS, term)
	if err == nil {
		return out, nil
	}
	// The direct attempt failed or returned nothing - retry via the proxy
	// before giving up (covers blocked hosts not in proxyDomains).
	if proxy != "" {
		return fetchOnce(ctx, path, rawURL, dump, proxy, httpTimeoutMS, term)
	}
	return "", err
}

// fetchResult mirrors the JSON object lightpanda emits with --json.
type fetchResult struct {
	URL        string `json:"url"`
	HTTPStatus int    `json:"http_status"`
	Content    string `json:"content"`
}

// fetchOnce runs a single lightpanda fetch and returns the dumped content.
// Results are validated through the --json status fields: a page that did
// not load (status 0), an HTTP error, an anti-bot interstitial, or an empty
// dump all become errors instead of silent empty output.
func fetchOnce(ctx context.Context, path, rawURL, dump, proxy string, timeoutMS, termMS int) (string, error) {
	args := []string{"fetch", rawURL, "--dump", dump, "--json",
		"--http-timeout", strconv.Itoa(timeoutMS),
		"--terminate-ms", strconv.Itoa(termMS)}
	if proxy != "" {
		args = append(args, "--http-proxy", proxy)
	}

	out, err := runFetch(ctx, path, args...)
	if err != nil {
		return "", err
	}
	var res fetchResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		preview := out
		if len(preview) > 100 {
			preview = preview[:100]
		}
		return "", fmt.Errorf("lightpanda fetch: parse --json output: %w (output: %q)", err, preview)
	}
	switch {
	case res.HTTPStatus == 0:
		if isGoogleHost(rawURL) {
			return "", fmt.Errorf("lightpanda fetch: page did not load (HTTP status 0): Google rejects the Lightpanda browser fingerprint; use Bing for search")
		}
		return "", fmt.Errorf("lightpanda fetch: page did not load (HTTP status 0); site may be blocked or unreachable")
	case res.HTTPStatus >= 400:
		if res.HTTPStatus == 429 && isGoogleHost(rawURL) {
			return "", fmt.Errorf("lightpanda fetch: HTTP 429: Google rate-limited the request (bot detection); use Bing for search")
		}
		return "", fmt.Errorf("lightpanda fetch: HTTP %d", res.HTTPStatus)
	case strings.TrimSpace(res.Content) == "":
		return "", fmt.Errorf("lightpanda fetch: page returned no content")
	case isBotInterstitial(rawURL, res.Content):
		return "", fmt.Errorf("lightpanda fetch: site returned an anti-bot interstitial instead of results; for Google search, try Bing")
	}
	return res.Content, nil
}

// isGoogleHost reports whether rawURL points at a Google property (used to
// give targeted diagnostics for Google's fingerprint-based blocking).
func isGoogleHost(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "google.com" || strings.HasSuffix(host, ".google.com")
}

// isBotInterstitial detects search-engine anti-bot pages (Google's HTTP 200
// "trouble accessing" shell, the /sorry CAPTCHA page) so the caller gets an
// actionable error instead of a result-less dump.  Only Google hosts are
// checked to avoid false positives on ordinary pages.
func isBotInterstitial(rawURL, content string) bool {
	if !isGoogleHost(rawURL) {
		return false
	}
	s := strings.ToLower(content)
	return strings.Contains(s, "trouble accessing google") ||
		strings.Contains(s, "unusual traffic") ||
		strings.Contains(s, "our systems have detected")
}

// needsProxy reports whether rawURL's host commonly requires a proxy.
func needsProxy(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, d := range proxyDomains {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// runFetch executes the lightpanda binary and returns its stdout.
// Injectable for tests.  On failure the error carries stderr - the fetch
// CLI prints the reason there - so callers can diagnose 429/redirects.
var runFetch = func(ctx context.Context, path string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("lightpanda fetch: %w: %s", err, msg)
	}
	return stdout.String(), nil
}
