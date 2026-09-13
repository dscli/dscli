package lp

// Gated fixture probe for the auto-continue recovery (see webchat.go's
// jsContinueGeneration / clickTrustedAt and docs/task-continue-generation.md).
//
// It runs headless Chromium against a LOCAL file:// fixture - no network, no
// real login profile - and pins the one fact the whole design rests on: the
// 「继续生成」 button validates isTrusted, so a JS-synthesized click is
// silently dropped while a CDP mouse event goes through.
//
//	DSCLI_LIVE_CLICK_PROBE=1 go test -v -run TestLiveContinueProbe ./internal/lp/
//
// Without DSCLI_LIVE_CLICK_PROBE=1 it skips (the default in CI and in a
// normal test pass).

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// continueProbeGuardJS installs the fixture-side mimic of the site's click
// guard: it counts trusted and synthetic clicks separately and records the
// trust of the most recent one, exactly like the bundle's onClick does (a
// synthetic event reaches the handler but carries isTrusted=false).
const continueProbeGuardJS = `
window.__trustedClicks = 0;
window.__syntheticClicks = 0;
window.__lastTrusted = null;
window.__guard = function(e) {
	if (e.isTrusted) { window.__trustedClicks++; window.__lastTrusted = true; }
	else { window.__syntheticClicks++; window.__lastTrusted = false; }
};
`

// continueProbeRow renders one assistant message row in the site's shape:
// [avatar, .ds-message bubble, action bar]. The button is a SIBLING of the
// bubble, which is the structural fact the detector's ownership walk relies
// on (a descendant button must not be the only thing that matches).
func continueProbeRow(bubble, button string) string {
	return fmt.Sprintf(`<div class="row">
	<div class="avatar">AI</div>
	<div class="ds-message"><div class="ds-assistant-message-main-content">%s</div></div>
	<div class="bar"><button onclick="__guard(event)">%s</button></div>
</div>`, bubble, button)
}

func continueProbePage(rows string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>probe</title>
<style>
body{margin:0;font-family:sans-serif}
.row{display:flex;align-items:flex-start;gap:8px;padding:12px}
.bubble,.ds-message{background:#eef;padding:8px;border-radius:8px}
.bar{display:flex;gap:4px}
button{padding:6px 10px}
</style></head><body>
<div id="chat">` + rows + `</div>
<script>` + continueProbeGuardJS + `</script>
</body></html>`
}

// writeContinueProbeFile materializes one fixture page and returns its
// file:// URL.
func writeContinueProbeFile(t *testing.T, dir, name, html string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		t.Fatalf("write fixture %s: %v", name, err)
	}
	return "file://" + path
}

// continueProbeGuardState is the fixture guard's observable state.
type continueProbeGuardState struct {
	Trusted   int
	Synthetic int
	Last      bool
}

// readContinueProbeGuard reads the fixture guard counters inside a chromedp
// action, one scalar per call (a combined array would have to decode into
// []any of pointers, which the JSON decoder does not guarantee).
func readContinueProbeGuard(ctx context.Context) (continueProbeGuardState, error) {
	var st continueProbeGuardState
	if err := chromedp.Evaluate(`window.__trustedClicks`, &st.Trusted).Do(ctx); err != nil {
		return st, err
	}
	if err := chromedp.Evaluate(`window.__syntheticClicks`, &st.Synthetic).Do(ctx); err != nil {
		return st, err
	}
	if err := chromedp.Evaluate(`window.__lastTrusted === true`, &st.Last).Do(ctx); err != nil {
		return st, err
	}
	return st, nil
}

func TestLiveContinueProbe(t *testing.T) {
	if os.Getenv("DSCLI_LIVE_CLICK_PROBE") != "1" {
		t.Skip("live click probe: set DSCLI_LIVE_CLICK_PROBE=1 (runs headless Chromium on a local fixture)")
	}

	chromePath, err := findChrome()
	if err != nil {
		t.Skipf("no Chrome/Chromium available: %v", err)
	}

	dir := t.TempDir()
	target := writeContinueProbeFile(t, dir, "target.html", continueProbePage(
		continueProbeRow("半截回复…", "继续生成"),
	))
	decoy := writeContinueProbeFile(t, dir, "decoy.html", continueProbePage(
		continueProbeRow("完成的回复", "重新生成"),
	))
	empty := writeContinueProbeFile(t, dir, "empty.html", continueProbePage(""))

	// Headless, throwaway profile: the probe must never touch the real
	// logged-in dscli profile, and a file:// fixture needs no network.
	// The profile dir is a MkdirTemp rather than t.TempDir because
	// Chromium keeps writing into it after the test body returns, which
	// makes t.TempDir's strict cleanup fail on a non-empty directory.
	profileDir, err := os.MkdirTemp("", "dscli-continue-probe-")
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

	open := func(url string) {
		t.Helper()
		if err := chromedp.Run(
			tabCtx,
			chromedp.Navigate(url),
			chromedp.WaitReady("body"),
			chromedp.EmulateViewport(1024, 768),
			chromedp.Sleep(300*time.Millisecond),
		); err != nil {
			t.Fatalf("open %s: %v", url, err)
		}
	}

	// detect runs the production detector the same way webchatWait does:
	// inside a chromedp action, on the action's own context. The helpers use
	// Evaluate(...).Do(ctx), which only resolves on that context (a raw tab
	// context has no executor attached).
	detect := func() (label string, x, y float64, found bool) {
		t.Helper()
		if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
			label, x, y, found = continueGenerationButton(ctx)
			return nil
		})); err != nil {
			t.Fatalf("continueGenerationButton: %v", err)
		}
		return label, x, y, found
	}

	// --- 1. detector finds the real button and reports usable coordinates.
	open(target)
	label, x, y, found := detect()
	t.Logf("detector: found=%v label=%q x=%.1f y=%.1f", found, label, x, y)
	if !found {
		t.Fatalf("detector must find the 继续生成 button on the fixture")
	}
	if label != "继续生成" {
		t.Errorf("label = %q, want 继续生成", label)
	}
	var hit bool
	hitJS := fmt.Sprintf(`(() => {
		const el = document.elementFromPoint(%.1f, %.1f);
		return !!el && el.tagName === 'BUTTON';
	})()`, x, y)
	if err := chromedp.Run(tabCtx, chromedp.Evaluate(hitJS, &hit)); err != nil {
		t.Fatalf("elementFromPoint: %v", err)
	}
	if !hit {
		t.Errorf("reported coordinates (%.1f, %.1f) must hit the button", x, y)
	}

	// --- 2. the CDP click goes through the guard.
	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return clickTrustedAt(ctx, x, y)
	})); err != nil {
		t.Fatalf("clickTrustedAt: %v", err)
	}
	if err := chromedp.Run(tabCtx, chromedp.Sleep(200*time.Millisecond)); err != nil {
		t.Fatalf("settle: %v", err)
	}
	var st continueProbeGuardState
	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		st, err = readContinueProbeGuard(ctx)
		return err
	})); err != nil {
		t.Fatalf("read guard state: %v", err)
	}
	t.Logf("after clickTrustedAt: trusted=%d synthetic=%d lastTrusted=%v", st.Trusted, st.Synthetic, st.Last)
	if st.Trusted != 1 {
		t.Errorf("trusted clicks = %d, want 1 (the CDP click must reach the guard)", st.Trusted)
	}
	if !st.Last {
		t.Errorf("lastTrusted = %v, want true", st.Last)
	}

	// --- 3. the synthetic click is rejected: this is the whole reason
	// clickTrustedAt exists. The string below is test code, not detector
	// JS - the detector itself must stay click-free.
	if err := chromedp.Run(tabCtx, chromedp.Evaluate(
		`document.querySelector('button').click()`, nil,
	)); err != nil {
		t.Fatalf("synthetic click: %v", err)
	}
	if err := chromedp.Run(tabCtx, chromedp.Sleep(200*time.Millisecond)); err != nil {
		t.Fatalf("settle: %v", err)
	}
	if err := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var err error
		st, err = readContinueProbeGuard(ctx)
		return err
	})); err != nil {
		t.Fatalf("read guard state: %v", err)
	}
	t.Logf("after synthetic click: trusted=%d synthetic=%d lastTrusted=%v", st.Trusted, st.Synthetic, st.Last)
	if st.Trusted != 1 {
		t.Errorf("trusted clicks = %d after a synthetic click, want 1 (isTrusted guard must reject it)", st.Trusted)
	}
	if st.Synthetic != 1 {
		t.Errorf("synthetic clicks = %d, want 1 (the rejected attempt should still be recorded)", st.Synthetic)
	}
	if st.Last {
		t.Errorf("lastTrusted = %v after a synthetic click, want false", st.Last)
	}

	// --- 4. a row whose only button is 重新生成 must not match.
	open(decoy)
	label, _, _, found = detect()
	t.Logf("decoy (重新生成 only): found=%v label=%q", found, label)
	if found {
		t.Errorf("detector must not match the 重新生成 decoy (label=%q)", label)
	}

	// --- 5. an empty page must not match.
	open(empty)
	label, _, _, found = detect()
	t.Logf("empty page: found=%v label=%q", found, label)
	if found {
		t.Errorf("detector must not match an empty page (label=%q)", label)
	}
}
