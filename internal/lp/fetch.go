package lp

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"

	"github.com/dscli/dscli/internal/config"
	"github.com/nanjj/clog"
)

// FetchOptions configures a lightpanda fetch invocation.  Zero values
// mean "use lightpanda's default" (except Dump, which defaults to markdown).
type FetchOptions struct {
	Dump        string // markdown | html | semantic_tree | semantic_tree_text
	StripMode   string // comma-separated tag groups: js, css, ui, invisible, full
	WaitUntil   string // load | domcontentloaded | networkalmostidle | networkidle | done
	WaitMS      int    // wait time in milliseconds (lightpanda default 5000)
	TerminateMS int    // hard deadline in milliseconds; 0 = no deadline
	Proxy       string // --http-proxy URL (e.g. socks5h://localhost:8777); falls back to lightpanda-proxy config
}

// Fetch runs `lightpanda fetch` for a single URL and returns its stdout.
//
// Each call spawns a fresh process and exits - unlike the MCP server there
// is no long-running process, so a hung page cannot poison later calls.
// The dump output is returned verbatim; callers should prefer markdown or
// semantic_tree over html for large pages.
//
// The proxy comes from opts.Proxy, or from the lightpanda-proxy config
// value when unset.
func Fetch(ctx context.Context, rawURL string, opts FetchOptions) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "Fetch")
	defer span.Finish()

	path, err := exec.LookPath("lightpanda")
	if err != nil {
		return "", fmt.Errorf("lightpanda not found in PATH: %w", err)
	}

	args := []string{"fetch", rawURL}
	dump := opts.Dump
	if dump == "" {
		dump = "markdown"
	}
	args = append(args, "--dump", dump)
	if opts.StripMode != "" {
		args = append(args, "--strip-mode", opts.StripMode)
	}
	if opts.WaitUntil != "" {
		args = append(args, "--wait-until", opts.WaitUntil)
	}
	if opts.WaitMS > 0 {
		args = append(args, "--wait-ms", strconv.Itoa(opts.WaitMS))
	}
	if opts.TerminateMS > 0 {
		args = append(args, "--terminate-ms", strconv.Itoa(opts.TerminateMS))
	}
	proxy := opts.Proxy
	if proxy == "" {
		proxy = config.Get("lightpanda-proxy", "")
	}
	if proxy != "" {
		args = append(args, "--http-proxy", proxy)
	}

	out, err := runFetch(ctx, path, args...)
	if err != nil {
		return "", err
	}
	return out, nil
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
