package lp

// Gated fixture probe for the stopped-state 「重新生成」 recovery (see
// webchat.go's jsRegenerateStopped / regenerateStoppedButton and
// docs/task-stop-recovery-and-residue.md).
//
// It runs headless Chromium against a LOCAL file:// fixture - no network, no
// real login profile - and pins the detector's contract on the site's row
// shape: the stopped state has NO answer body, an 「已停止」 leaf in the think
// header, and the regenerate button as the SECOND icon of the action bar.
//
//	DSCLI_LIVE_CLICK_PROBE=1 go test -v -run TestLiveRegenerateProbe ./internal/lp/
//
// Without DSCLI_LIVE_CLICK_PROBE=1 it skips (the default in CI and in a
// normal test pass).

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// regenerateProbeJS installs the fixture-side click counter: the regenerate
// button has no isTrusted guard in the bundle, so the probe only needs to see
// that the CDP click reached the button at all.
const regenerateProbeJS = `
window.__hits = {};
window.__hit = function(id) { window.__hits[id] = (window.__hits[id] || 0) + 1; };
`

// regenerateProbeIcon renders one icon-only action-bar button: no text, an
// svg child - exactly what the detector's iconButtons helper looks for.
func regenerateProbeIcon(id string) string {
	return fmt.Sprintf(
		`<button id="%s" onclick="__hit('%s')"><svg width="16" height="16"><rect width="16" height="16"/></svg></button>`,
		id, id)
}

// regenerateProbeRow renders one assistant message row in the site's shape:
// [avatar, .ds-message bubble, footer action bar]. body non-empty adds a
// main-content (the reverse fixtures); bar=false omits the action bar.
func regenerateProbeRow(body string, bar bool) string {
	bubble := `<div class="ds-message"><div class="head"><span>已停止</span></div>`
	if body != "" {
		bubble += `<div class="ds-assistant-message-main-content">` + body + `</div>`
	}
	bubble += `</div>`
	barHTML := ""
	if bar {
		barHTML = `<div class="bar">` +
			regenerateProbeIcon("copy") +
			regenerateProbeIcon("regen") +
			regenerateProbeIcon("like") +
			regenerateProbeIcon("dislike") +
			regenerateProbeIcon("share") +
			`</div>`
	}
	return `<div class="row"><div class="avatar">AI</div>` + bubble + barHTML + `</div>`
}

func regenerateProbePage(rows string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>probe</title>
<style>
body{margin:0;font-family:sans-serif}
.row{display:flex;align-items:flex-start;gap:8px;padding:12px}
.ds-message{background:#eef;padding:8px;border-radius:8px}
.bar{display:flex;gap:4px}
button{width:28px;height:28px;padding:0}
</style></head><body>
<div id="chat">` + rows + `</div>
<script>` + regenerateProbeJS + `</script>
</body></html>`
}

// regenerateProbeEnv carries the shared headless browser and the fixture URLs.
type regenerateProbeEnv struct {
	tabCtx context.Context

	target   string
	body     string
	phrase   string
	noAction string
}

// open navigates the shared tab to a fixture and waits for layout.
func (e *regenerateProbeEnv) open(t *testing.T, url string) {
	t.Helper()
	if err := chromedp.Run(
		e.tabCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.EmulateViewport(1024, 768),
		chromedp.Sleep(300*time.Millisecond),
	); err != nil {
		t.Fatalf("open %s: %v", url, err)
	}
}

// detect runs the production detector the same way webchatWait does: inside a
// chromedp action, on the action's own context.
func (e *regenerateProbeEnv) detect(t *testing.T) regenerateDetect {
	t.Helper()
	var d regenerateDetect
	if err := chromedp.Run(e.tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		var derr error
		d, derr = regenerateStoppedButton(ctx)
		return derr
	})); err != nil {
		t.Fatalf("regenerateStoppedButton: %v", err)
	}
	return d
}

func (e *regenerateProbeEnv) settle(t *testing.T) {
	t.Helper()
	if err := chromedp.Run(e.tabCtx, chromedp.Sleep(200*time.Millisecond)); err != nil {
		t.Fatalf("settle: %v", err)
	}
}

// caseDetectorFindsButton: the detector locates the 2nd action-bar button and
// reports coordinates that actually hit it.
func (e *regenerateProbeEnv) caseDetectorFindsButton(t *testing.T) {
	e.open(t, e.target)
	d := e.detect(t)
	t.Logf("detector: present=%v clickable=%v buttons=%d x=%.1f y=%.1f", d.present, d.clickable, d.buttons, d.x, d.y)
	if !d.present {
		t.Fatalf("detector must find the stopped-state action bar")
	}
	if !d.clickable {
		t.Errorf("button must be reported clickable on an unoccluded fixture")
	}
	if d.buttons != 5 {
		t.Errorf("buttons = %d, want 5 (the site's copy/regen/like/dislike/share bar)", d.buttons)
	}
	var hitID string
	hitJS := fmt.Sprintf(`(() => {
		const el = document.elementFromPoint(%.1f, %.1f);
		if (!el) return '';
		const b = el.closest ? el.closest('button') : null;
		return b ? b.id : '';
	})()`, d.x, d.y)
	if err := chromedp.Run(e.tabCtx, chromedp.Evaluate(hitJS, &hitID)); err != nil {
		t.Fatalf("elementFromPoint: %v", err)
	}
	if hitID != "regen" {
		t.Errorf("reported coordinates (%.1f, %.1f) hit %q, want the 2nd button (regen)", d.x, d.y, hitID)
	}
}

// caseTrustedClick: the CDP click reaches the regenerate button exactly once.
func (e *regenerateProbeEnv) caseTrustedClick(t *testing.T) {
	e.open(t, e.target)
	d := e.detect(t)
	if !d.present || !d.clickable {
		t.Fatalf("fixture button not clickable: %+v", d)
	}
	if err := chromedp.Run(e.tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return clickTrustedAt(ctx, d.x, d.y)
	})); err != nil {
		t.Fatalf("clickTrustedAt: %v", err)
	}
	e.settle(t)
	var hits int
	if err := chromedp.Run(e.tabCtx, chromedp.Evaluate(`window.__hits.regen || 0`, &hits)); err != nil {
		t.Fatalf("read hit count: %v", err)
	}
	t.Logf("after clickTrustedAt: regen hits=%d", hits)
	if hits != 1 {
		t.Errorf("regen hits = %d, want 1 (the CDP click must reach the 2nd button)", hits)
	}
}

// caseBodyPresent: an answer body means the round was not stopped - the
// detector must decline.
func (e *regenerateProbeEnv) caseBodyPresent(t *testing.T) {
	e.open(t, e.body)
	d := e.detect(t)
	t.Logf("body present: present=%v", d.present)
	if d.present {
		t.Errorf("detector must not match a round with an answer body")
	}
}

// casePhraseInBody: the phrase 「已停止」 inside the answer body must not trip
// the detector (the scan skips the main-content subtree).
func (e *regenerateProbeEnv) casePhraseInBody(t *testing.T) {
	e.open(t, e.phrase)
	d := e.detect(t)
	t.Logf("phrase in body: present=%v", d.present)
	if d.present {
		t.Errorf("detector must not match a body that merely mentions 已停止")
	}
}

// caseNoActionBar: without the icon bar there is nothing to click.
func (e *regenerateProbeEnv) caseNoActionBar(t *testing.T) {
	e.open(t, e.noAction)
	d := e.detect(t)
	t.Logf("no action bar: present=%v", d.present)
	if d.present {
		t.Errorf("detector must not match without an action bar")
	}
}

func TestLiveRegenerateProbe(t *testing.T) {
	if os.Getenv("DSCLI_LIVE_CLICK_PROBE") != "1" {
		t.Skip("live click probe: set DSCLI_LIVE_CLICK_PROBE=1 (runs headless Chromium on a local fixture)")
	}

	chromePath, err := findChrome()
	if err != nil {
		t.Skipf("no Chrome/Chromium available: %v", err)
	}

	dir := t.TempDir()
	env := &regenerateProbeEnv{}
	env.target = writeContinueProbeFile(t, dir, "regen-target.html", regenerateProbePage(
		regenerateProbeRow("", true),
	))
	env.body = writeContinueProbeFile(t, dir, "regen-body.html", regenerateProbePage(
		regenerateProbeRow("这里有正文。", true),
	))
	env.phrase = writeContinueProbeFile(t, dir, "regen-phrase.html", regenerateProbePage(
		regenerateProbeRow("正文里提到了 已停止 这个词。", true),
	))
	env.noAction = writeContinueProbeFile(t, dir, "regen-noaction.html", regenerateProbePage(
		regenerateProbeRow("", false),
	))

	profileDir, err := os.MkdirTemp("", "dscli-regen-probe-")
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
	env.tabCtx = tabCtx

	t.Run("detector finds the regenerate button", func(t *testing.T) {
		env.caseDetectorFindsButton(t)
	})
	t.Run("trusted click reaches the button", func(t *testing.T) {
		env.caseTrustedClick(t)
	})
	t.Run("answer body present does not match", func(t *testing.T) {
		env.caseBodyPresent(t)
	})
	t.Run("phrase in body does not match", func(t *testing.T) {
		env.casePhraseInBody(t)
	})
	t.Run("no action bar does not match", func(t *testing.T) {
		env.caseNoActionBar(t)
	})
}
