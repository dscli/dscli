package lp

// Gated fixture probe for the scoped DOM extraction (jsAssistantTextAfterFmt
// / assistantTextAfterMessage, see webchat.go and
// docs/task-webchat-recovery-extraction.md).
//
// It runs headless Chromium against a LOCAL file:// fixture - no network, no
// real login profile - and pins the behaviour the recovery fix rests on:
//
//  1. a page holding [an EARLIER assistant message with a block] + [the user
//     bubble carrying the sent message] + [the stopped 1/2 version with no
//     body] + [the 2/2 reply with the correct block] must extract ONLY the
//     2/2 content - never the earlier round, never the empty 1/2 version;
//  2. an auto-linked path inside the 2/2 reply must come back as literal
//     text (no `](` injection into the code);
//  3. a page without the anchor bubble must return "" so the caller falls
//     back to the legacy paths.
//
//	DSCLI_LIVE_EXTRACT_PROBE=1 go test -v -run TestLiveScopedExtractProbe ./internal/lp/
//
// Run it - together with TestLiveIDBSelectProbe - before merging changes to
// assistantTextAfterMessage / jsIDBGetAnswerFmt: CI cannot execute the
// gated probes.
//
// Without DSCLI_LIVE_EXTRACT_PROBE=1 it skips (the default in CI and in a
// normal test pass).

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// extractProbeSent is the message dscli sent. It is the anchor the extraction
// scans for among the .ds-message bubbles.
const extractProbeSent = "重新按正确范围裁切。"

// extractProbeOldBlock is the PREVIOUS round's reply, already executed. A
// correct extraction must never return it: its block is what made the shell
// judge report "the tags appear more than once".
const extractProbeOldBlock = "<p>旧回复</p><pre><code>echo old-round</code></pre>"

// extractProbeNewBlock is the 2/2 reply: it carries a prose auto-link (the
// site wraps URL-shaped text) and the one legitimate block, whose path is
// auto-linked too.
const extractProbeNewBlock = `<p>裁到页脚了。重新按正确范围裁切。</p>` +
	`<p>定位图：<a href="https://example.com/out/p6-check.png">example.com/out/p6-check.png</a> 已生成</p>` +
	`<pre><code>python3 <a href="https://example.com/out/page-6.png">example.com/out/page-6.png</a></code></pre>`

// extractProbeRow renders one message row in the site's shape: a .ds-message
// bubble carrying the text (or the assistant body), which is the element the
// anchor scan numbers in document order.
func extractProbeRow(role, body string) string {
	return `<div class="row"><div class="avatar">` + role + `</div>` +
		`<div class="ds-message">` + body + `</div></div>`
}

// extractProbePage wraps the rows into a minimal document.
func extractProbePage(rows string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>probe</title></head>` +
		`<body><div id="chat">` + rows + `</div></body></html>`
}

// extractProbeTarget is the full recovery-shaped page: an earlier assistant
// round, the user bubble with the sent message, the stopped 1/2 version with
// no body, then the 2/2 reply.
func extractProbeTarget() string {
	return extractProbePage(
		extractProbeRow("AI", `<div class="ds-assistant-message-main-content">`+extractProbeOldBlock+`</div>`) +
			extractProbeRow("USER", extractProbeSent) +
			extractProbeRow("AI", `<div class="ds-assistant-message-main-content"></div>`) +
			extractProbeRow("AI", `<div class="ds-assistant-message-main-content">`+extractProbeNewBlock+`</div>`),
	)
}

// extractProbeEnv carries the headless browser plus a helper that runs the
// production extraction inside a chromedp action (Evaluate(...).Do(ctx) only
// resolves on an action context).
type extractProbeEnv struct {
	tabCtx context.Context
}

// extract runs the production wrapper on the current page.
func (e *extractProbeEnv) extract(t *testing.T, sent string) string {
	t.Helper()
	var got string
	if err := chromedp.Run(e.tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		got = assistantTextAfterMessage(ctx, sent)
		return nil
	})); err != nil {
		t.Fatalf("assistantTextAfterMessage: %v", err)
	}
	return got
}

func (e *extractProbeEnv) open(t *testing.T, url string) {
	t.Helper()
	if err := chromedp.Run(
		e.tabCtx,
		chromedp.EmulateViewport(1024, 768),
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(200*time.Millisecond),
	); err != nil {
		t.Fatalf("open %s: %v", url, err)
	}
}

func TestLiveScopedExtractProbe(t *testing.T) {
	if os.Getenv("DSCLI_LIVE_EXTRACT_PROBE") != "1" {
		t.Skip("live extract probe: set DSCLI_LIVE_EXTRACT_PROBE=1 (runs headless Chromium on a local fixture)")
	}

	chromePath, err := findChrome()
	if err != nil {
		t.Skipf("no Chrome/Chromium available: %v", err)
	}

	dir := t.TempDir()
	target := writeContinueProbeFile(t, dir, "target.html", extractProbeTarget())
	noAnchor := writeContinueProbeFile(t, dir, "noanchor.html", extractProbePage(
		extractProbeRow("AI", `<div class="ds-assistant-message-main-content">`+extractProbeOldBlock+`</div>`),
	))

	// Headless, throwaway profile: never touch the real logged-in dscli
	// profile, and a file:// fixture needs no network. The profile dir is a
	// MkdirTemp rather than t.TempDir because Chromium keeps writing into it
	// after the test body returns.
	profileDir, err := os.MkdirTemp("", "dscli-extract-probe-")
	if err != nil {
		t.Fatalf("profile dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(profileDir) }()

	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(profileDir),
		chromedp.Headless,
		chromedp.NoSandbox,
		chromedp.Flag("disable-gpu", true),
	}
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer allocCancel()

	ctx, cancel := context.WithTimeout(allocCtx, 120*time.Second)
	defer cancel()
	tabCtx, tabCancel := chromedp.NewContext(ctx)
	defer tabCancel()
	env := &extractProbeEnv{tabCtx: tabCtx}

	t.Run("only the current round survives", func(t *testing.T) {
		env.open(t, target)
		got := env.extract(t, extractProbeSent)
		t.Logf("extracted (%d runes):\n%s", countRunes(got), got)
		if strings.Contains(got, "echo old-round") || strings.Contains(got, "旧回复") {
			t.Errorf("the previous round leaked into the reply:\n%s", got)
		}
		if !strings.Contains(got, "python3 example.com/out/page-6.png") {
			t.Errorf("the 2/2 reply body is missing:\n%s", got)
		}
		if n := strings.Count(got, "```"); n != 2 {
			t.Errorf("code fence count = %d, want 2 (exactly one block):\n%s", n, got)
		}
	})

	t.Run("an auto-linked path stays literal", func(t *testing.T) {
		env.open(t, target)
		got := env.extract(t, extractProbeSent)
		if strings.Contains(got, "](") {
			t.Errorf("auto link was rendered as markdown link syntax:\n%s", got)
		}
		if strings.Contains(got, "https://") {
			t.Errorf("auto link injected the scheme into the text:\n%s", got)
		}
	})

	t.Run("no anchor returns empty", func(t *testing.T) {
		env.open(t, noAnchor)
		if got := env.extract(t, extractProbeSent); got != "" {
			t.Errorf("extraction without the anchor bubble = %q, want \"\" (must fall back)", got)
		}
		if got := env.extract(t, ""); got != "" {
			t.Errorf("extraction with an empty sent message = %q, want \"\"", got)
		}
	})

	t.Run("an echoing reply does not steal the anchor", func(t *testing.T) {
		// The reply restates the sent message at its start; the anchor scan
		// must skip the answered assistant bubble and keep the user bubble,
		// otherwise the extraction would start after the reply itself.
		echoing := writeContinueProbeFile(t, dir, "echo.html", extractProbePage(
			extractProbeRow("USER", extractProbeSent)+
				extractProbeRow("AI", `<div class="ds-assistant-message-main-content">`+
					extractProbeSent+`<p>收到，开始处理。</p></div>`),
		))
		env.open(t, echoing)
		got := env.extract(t, extractProbeSent)
		if !strings.Contains(got, "收到，开始处理。") {
			t.Errorf("the echoing reply must still be returned:\n%s", got)
		}
	})
}
