package lp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"regexp"
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

	// maxRefreshFollows caps how many <meta http-equiv="refresh"> hops Fetch
	// will follow when a page's markdown dump comes back empty.  The cap
	// prevents redirect loops from spawning unbounded lightpanda processes.
	maxRefreshFollows = 2
)

// errEmptyContent marks a successful HTTP fetch whose dump came back empty.
// It is internal: Fetch resolves it (html fallback, meta-refresh following)
// before returning, so callers only ever see the plain "no content" error
// when the page genuinely has nothing to offer.
var errEmptyContent = errors.New("page returned no content")

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
	ForceProxy  bool   // skip the direct probe and go straight through the proxy
	Output      string // file path (or path:N) to write the result to as well; N inserts at line N
}

// Fetch runs `lightpanda fetch` for a single URL and returns its dump text.
//
// Each call spawns a fresh process and exits - unlike the MCP server there
// is no long-running process, so a hung page cannot poison later calls.
// The dump output is returned verbatim; callers should prefer markdown or
// semantic_tree over html for large pages.
//
// When opts.Output is set ("path" or "path:N"), the returned text is also
// written to that file - see writeOutput for the line-insertion semantics.
// The write happens only after a successful fetch, so a failed probe or a
// retry leaves no partial file behind.
//
// The proxy comes from opts.Proxy, or from the lightpanda-http-proxy value
// in config when unset.  Hosts known to need a proxy (see proxyDomains, plus
// user-configured lightpanda-additional-proxy-domains) go straight through
// it; other hosts are fetched directly first and retried via the proxy when
// the direct attempt fails or returns nothing.  ForceProxy skips the direct
// probe entirely.
//
// A markdown dump that comes back empty is not immediately an error: the
// page may be a <meta http-equiv="refresh"> shell (common on GitHub Pages
// sites), which converts to no markdown at all.  Fetch then re-fetches as
// html, and if the page is a meta-refresh redirect it follows the target
// (up to maxRefreshFollows hops) and returns the target's dump.  A page
// with no convertible text and no redirect falls back to returning its raw
// html instead of failing; only a page that is empty in every dump form
// yields the "no content" error.
func Fetch(ctx context.Context, rawURL string, opts FetchOptions) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "Fetch")
	defer span.Finish()

	text, err := fetchWithDepth(ctx, rawURL, opts, 0)
	if err != nil {
		return "", err
	}

	// A single success point: the result is written to opts.Output exactly
	// once, only after a successful fetch (never on the failed probe, never
	// from a followed meta-refresh hop).
	if opts.Output != "" {
		if werr := writeOutput(opts.Output, text); werr != nil {
			return "", fmt.Errorf("write output file: %w", werr)
		}
	}
	return text, nil
}

// fetchWithDepth is the recursive core of Fetch.  depth tracks followed
// meta-refresh hops and caps the recursion via maxRefreshFollows.
func fetchWithDepth(ctx context.Context, rawURL string, opts FetchOptions, depth int) (string, error) {
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

	// Known blocked domains (or ForceProxy) go straight through the proxy;
	// probing them directly would just burn the probe timeout.
	var out string
	switch {
	case proxy != "" && (opts.ForceProxy || needsProxy(rawURL)):
		out, err = fetchOnce(ctx, path, rawURL, dump, proxy, httpTimeoutMS, term)
	default:
		out, err = fetchOnce(ctx, path, rawURL, dump, "", probeTimeoutMS, term)
		// The direct attempt failed or returned nothing - retry via the
		// proxy before giving up (covers blocked hosts not in proxyDomains).
		if err != nil && proxy != "" {
			out, err = fetchOnce(ctx, path, rawURL, dump, proxy, httpTimeoutMS, term)
		}
	}
	if err != nil {
		if errors.Is(err, errEmptyContent) {
			return resolveEmptyContent(ctx, path, rawURL, dump, proxy, httpTimeoutMS, term, depth)
		}
		return "", err
	}
	return out, nil
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
	args := []string{
		"fetch", rawURL, "--dump", dump, "--json",
		"--http-timeout", strconv.Itoa(timeoutMS),
		"--terminate-ms", strconv.Itoa(termMS),
	}
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
		if proxy != "" {
			// A dead proxy tunnel (e.g. the ssh-socks SOCKS5 forward) leaves
			// the local port listening while every connection through it
			// fails - the result is a silent status 0 with no navigate
			// error.  Name the proxy so the user knows where to look.
			return "", fmt.Errorf("lightpanda fetch: page did not load (HTTP status 0) via proxy %s: the proxy tunnel may be down or the site unreachable", redactProxy(proxy))
		}
		return "", fmt.Errorf("lightpanda fetch: page did not load (HTTP status 0); site may be blocked or unreachable")
	case res.HTTPStatus >= 400:
		if res.HTTPStatus == 429 && isGoogleHost(rawURL) {
			return "", fmt.Errorf("lightpanda fetch: HTTP 429: Google rate-limited the request (bot detection); use Bing for search")
		}
		return "", fmt.Errorf("lightpanda fetch: HTTP %d", res.HTTPStatus)
	case strings.TrimSpace(res.Content) == "":
		return "", errEmptyContent
	case isBotInterstitial(rawURL, res.Content):
		return "", fmt.Errorf("lightpanda fetch: site returned an anti-bot interstitial instead of results; for Google search, try Bing")
	}
	return res.Content, nil
}

// resolveEmptyContent handles a successful HTTP 200 whose markdown dump came
// back empty.  Three cases are possible:
//
//   - the page is a <meta http-equiv="refresh"> shell with no body text
//     (common on GitHub Pages sites) - re-fetch the target and return its
//     dump;
//   - the page has content that simply does not convert to markdown (image
//     page, JS shell) - return its html so the caller still gets something;
//   - the page is empty in every form - keep the original error.
//
// The html re-fetch reuses the same proxy decision as the original call.
func resolveEmptyContent(ctx context.Context, path, rawURL, dump, proxy string, timeoutMS, termMS, depth int) (string, error) {
	if dump != "markdown" {
		// Only markdown conversion can legitimately come back empty for a
		// loaded page; other dumps that are empty mean the page truly has
		// nothing.
		return "", errEmptyContent
	}
	htmlOut, herr := fetchOnce(ctx, path, rawURL, "html", proxy, timeoutMS, termMS)
	if herr != nil {
		return "", errEmptyContent
	}
	if strings.TrimSpace(htmlOut) == "" {
		return "", errEmptyContent
	}
	if target, ok := parseMetaRefresh(htmlOut); ok {
		if depth < maxRefreshFollows {
			if resolved, rerr := resolveURL(rawURL, target); rerr == nil {
				return fetchWithDepth(ctx, resolved, FetchOptions{Dump: dump, Proxy: proxy}, depth+1)
			}
		}
		// The redirect chain exceeded the cap (or the target does not
		// resolve): returning the shell html would be useless, so keep
		// the no-content error.
		return "", errEmptyContent
	}
	// No redirect: the page has body content but no convertible text.
	// Returning the html beats failing - the caller asked for a page and
	// gets it, just in a different format.
	return htmlOut, nil
}

// metaTagRe matches individual <meta ...> tags (HTML is case-insensitive).
var metaTagRe = regexp.MustCompile(`(?i)<meta\b[^>]*>`)

// metaRefreshURLRe matches the url= value inside a refresh meta tag's
// content attribute: content="0;url=https://example.com/next".
var metaRefreshURLRe = regexp.MustCompile(`(?i)\burl\s*=\s*["']?([^"'>\s]+)`)

// parseMetaRefresh extracts the redirect target from a
// <meta http-equiv="refresh" content="N;url=..."> tag, if present.
func parseMetaRefresh(html string) (string, bool) {
	for _, tag := range metaTagRe.FindAllString(html, -1) {
		if !strings.Contains(strings.ToLower(tag), "refresh") {
			continue
		}
		m := metaRefreshURLRe.FindStringSubmatch(tag)
		if m == nil {
			continue
		}
		return strings.Trim(m[1], `"'`), true
	}
	return "", false
}

// resolveURL resolves ref (possibly relative, e.g. "2024/post.html")
// against the base URL of the page that contained it.
func resolveURL(base, ref string) (string, error) {
	b, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	r, err := url.Parse(ref)
	if err != nil {
		return "", err
	}
	return b.ResolveReference(r).String(), nil
}

// redactProxy renders a proxy URL for error messages without leaking any
// embedded credentials.
func redactProxy(proxy string) string {
	u, err := url.Parse(proxy)
	if err != nil {
		return proxy
	}
	return u.Redacted()
}

// writeOutput writes content to the file described by output.  Two forms are
// accepted:
//
//	"path"      overwrite the file (create it if missing)
//	"path:N"    insert content at line N (1-based): the content becomes file
//	            line N and the original line N and everything after it shift
//	            down.  A missing file ignores N; N beyond the last line appends.
//
// Lines are split on "\n"; CRLF files keep their \r characters verbatim.
func writeOutput(output, content string) error {
	path, line, err := parseOutputTarget(output)
	if err != nil {
		return err
	}
	if line == 0 {
		return os.WriteFile(path, []byte(content), 0o644)
	}

	data, rerr := os.ReadFile(path)
	if rerr != nil {
		if os.IsNotExist(rerr) {
			return os.WriteFile(path, []byte(content), 0o644)
		}
		return rerr
	}

	lines := strings.Split(string(data), "\n")
	idx := line - 1 // 0-based insertion point: content starts at file line `line`
	if idx > len(lines) {
		idx = len(lines)
	}
	head := strings.Join(lines[:idx], "\n")
	var b strings.Builder
	b.Grow(len(data) + len(content) + 16)
	b.WriteString(head)
	// join() never emits a trailing separator, so restore the newline that
	// separates lines[idx-1] from the inserted content (unless the head
	// already ends with one, e.g. appending after a trailing newline).
	if head != "" && !strings.HasSuffix(head, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(content)
	if content != "" && !strings.HasSuffix(content, "\n") {
		b.WriteByte('\n')
	}
	b.WriteString(strings.Join(lines[idx:], "\n"))
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// parseOutputTarget splits "path" or "path:N" into its path and a 1-based
// line number (0 = plain overwrite).  Only a trailing ":<positive integer>"
// is treated as a line number; anything else (e.g. "notes:v1.md") stays part
// of the path.  A numeric but non-positive suffix is an error, since ":0"
// is almost certainly a mistake.
func parseOutputTarget(output string) (path string, line int, err error) {
	if i := strings.LastIndex(output, ":"); i > 0 {
		suffix := output[i+1:]
		if n, aerr := strconv.Atoi(suffix); aerr == nil {
			if n <= 0 {
				return "", 0, fmt.Errorf("line number must be a positive integer, got %q", suffix)
			}
			return output[:i], n, nil
		}
	}
	return output, 0, nil
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

// needsProxy reports whether rawURL's host commonly requires a proxy.  The
// built-in list is merged with user-configured extra domains (config key
// lightpanda-additional-proxy-domains, array or comma-separated string).
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
	for _, d := range extraProxyDomains() {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// extraProxyDomains returns the user-configured additional proxy domains
// from config key lightpanda-additional-proxy-domains.  Both an array value
// and a comma-separated string are accepted; domains are trimmed and
// lowercased.  []string is accepted defensively even though the config
// parser currently produces []any.
func extraProxyDomains() []string {
	switch t := config.GetValue("lightpanda-additional-proxy-domains").(type) {
	case []any:
		items := make([]string, 0, len(t))
		for _, item := range t {
			if s, ok := item.(string); ok {
				items = append(items, s)
			}
		}
		return normDomains(items)
	case []string:
		return normDomains(t)
	case string:
		return normDomains(strings.Split(t, ","))
	}
	return nil
}

// normDomains trims, lowercases and drops empty entries.
func normDomains(items []string) []string {
	domains := make([]string, 0, len(items))
	for _, s := range items {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" {
			domains = append(domains, s)
		}
	}
	return domains
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
