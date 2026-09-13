package lp

// Gated live probe that re-verifies the upload-name policy against the real
// site (see uploadname.go and docs/task-upload-names.md). It attaches a
// battery of candidate names to a fresh chat composer and dumps the page's
// attachment state, WITHOUT sending any message - the draft is discarded when
// the tab closes.
//
// Re-check verifiedUploadExts whenever the site may have changed, then update
// the set (and the date in its comment) from this output:
//
//	DSCLI_LIVE_UPLOAD_PROBE=1 go test -v -timeout 300s -run TestLiveUploadNameProbe ./internal/lp/
//
// Requires Chrome plus a logged-in dscli profile; it opens a visible browser
// window and uploads files to chat.deepseek.com, so never run it in CI or as
// part of a normal test pass. Without DSCLI_LIVE_UPLOAD_PROBE=1 it skips.
//
// It calls webchatAttachFiles, not webchatUpload, on purpose: the probe must
// send the RAW candidate names, while webchatUpload normalizes them first and
// the rejected-name cases would never reach the site.
//
// The battery must stay within the site's per-batch file cap
// (WebUploadMaxFiles); TestLiveUploadNameProbeBatterySize guards that.
// Candidates are the highest-value names, not every extension: the group of
// site-rejected and silently dropped names, the rename-verification group, and
// a representative sample of the accepted extensions.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// liveUploadProbeCandidates is the probe battery, kept as a package-level
// variable so the size guard can check it without a browser.
var liveUploadProbeCandidates = []struct{ name, content string }{
	// Controls: known-good names, and the name from the 09-13 review
	// incident that the site rejects server-side.
	{"README.md", "# control\n"},
	{"main.go", "package main\n"},
	{"changes.patch", "--- a\n+++ b\n"},
	{".gitignore", "*.txt\n"},
	// Config/text extensions.
	{"data.yml", "a: 1\n"},
	{"data.json", "{}\n"},
	{"data.toml", "a = 1\n"},
	// A bare extension as the whole name (hidden file): the extension
	// model says SafeUploadName passes it through, and this candidate is
	// what confirms it on the next probe round.
	{".md", "# hidden\n"},
	{"data.conf", "a=1\n"},
	{"data.cfg", "a=1\n"},
	{"data.log", "line\n"},
	{"data.csv", "a,b\n"},
	{"data.tsv", "a\tb\n"},
	{"data.lock", "lock\n"},
	{"data.env", "A=1\n"},
	// Go module and dotfiles (client-side silent drops expected).
	{"go.mod", "module x\n"},
	{"go.sum", "x v1\n"},
	{"go.work", "go 1.27\n"},
	{".gitattributes", "* text=auto\n"},
	// Scripts and languages.
	{"run.sh", "echo hi\n"},
	{"script.py", "print(1)\n"},
	{"lib.rs", "fn main() {}\n"},
	{"app.ts", "let a = 1;\n"},
	{"app.js", "let a = 1;\n"},
	{"style.css", "a {}\n"},
	{"page.html", "<p>x</p>\n"},
	{"data.xml", "<a/>\n"},
	{"icon.svg", "<svg/>\n"},
	{"query.sql", "select 1;\n"},
	{"msg.proto", "syntax = \"proto3\";\n"},
	{"notes.org", "* x\n"},
	{"init.el", "(setq x 1)\n"},
	{"build.mk", "all:\n"},
	{"app.rb", "puts 1\n"},
	{"main.c", "int main(){}\n"},
	{"tool.cpp", "int main(){}\n"},
	{"widget.h", "#pragma once\n"},
	{"App.java", "class App {}\n"},
	{"Widget.cs", "class W {}\n"},
	{"App.kt", "fun main() {}\n"},
	{"App.swift", "print(1)\n"},
	{"index.php", "<?php ?>\n"},
	// Extensionless names (client-side silent drops expected).
	{"Dockerfile", "FROM x\n"},
	{"Makefile", "all:\n"},
	{"LICENSE", "MIT\n"},
	// Rename verification: the ".txt" suffix must make these acceptable.
	{".gitignore.txt", "*.txt\n"},
	{"gitignore.txt", "*.txt\n"},
	{"Makefile.txt", "all:\n"},
	// Case sensitivity of the accepted extensions: the verified set holds
	// lower-case spellings only, so these are expected to be renamed. If the
	// site turns out to accept them as-is, the probe result is the evidence
	// needed to relax SafeUploadName's exact match.
	{"README.MD", "# case\n"},
	{"Icon.PNG", "\x89PNG\r\n"},
}

// TestLiveUploadNameProbeBatterySize keeps the battery inside the site's
// per-batch file cap: a probe that exceeds it would fail upload-side and its
// results would be misleading.
func TestLiveUploadNameProbeBatterySize(t *testing.T) {
	if len(liveUploadProbeCandidates) > WebUploadMaxFiles {
		t.Fatalf("probe battery has %d candidates, want <= %d (WebUploadMaxFiles)", len(liveUploadProbeCandidates), WebUploadMaxFiles)
	}
}

func TestLiveUploadNameProbe(t *testing.T) {
	if os.Getenv("DSCLI_LIVE_UPLOAD_PROBE") != "1" {
		t.Skip("live upload probe: set DSCLI_LIVE_UPLOAD_PROBE=1 (needs Chrome login)")
	}

	dir := t.TempDir()
	candidates := liveUploadProbeCandidates
	var paths []string
	for _, c := range candidates {
		p := filepath.Join(dir, c.name)
		if err := os.WriteFile(p, []byte(c.content), 0o644); err != nil {
			t.Fatalf("write %s: %v", c.name, err)
		}
		paths = append(paths, p)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 240*time.Second)
	defer cancel()

	allocCtx, allocCancel, err := NewChromium(ctx)
	if err != nil {
		t.Fatalf("NewChromium: %v", err)
	}
	defer allocCancel()
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	// Boot the chat composer (same waits as webchatSend).
	if err := chromedp.Run(
		tabCtx,
		chromedp.Navigate(deepseekChatURL),
		chromedp.WaitReady("body"),
		chromedp.ActionFunc(waitForChatTextarea),
		chromedp.Sleep(3*time.Second),
	); err != nil {
		t.Fatalf("boot: %v", err)
	}

	// Attach the RAW names: the production upload path would normalize the
	// names under test before they ever reach the site.
	upErr := chromedp.Run(tabCtx, chromedp.ActionFunc(func(ctx context.Context) error {
		return webchatAttachFiles(ctx, paths)
	}))
	t.Logf("webchatAttachFiles returned: %v", upErr)

	// Let cards settle (uploads/parse/server round trips).
	if err := chromedp.Run(tabCtx, chromedp.Sleep(40*time.Second)); err != nil {
		t.Fatalf("settle: %v", err)
	}

	names := make([]string, 0, len(candidates))
	for _, c := range candidates {
		names = append(names, c.name)
	}
	namesJSON, err := json.Marshal(names)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var out map[string]any
	// Presence detection avoids substring cross-talk between a name and its
	// ".txt" sibling (Makefile vs Makefile.txt, .gitignore vs .gitignore.txt):
	// an exact whole-line match wins, otherwise the name must sit on a
	// word/dot boundary. The matched line and how it matched are logged, so a
	// surprising result can be judged from the output alone.
	js := fmt.Sprintf(`(() => {
		const names = %s;
		const text = (document.body ? document.body.innerText : '') || '';
		const lines = text.split('\n').map(s => s.trim()).filter(Boolean);
		const esc = s => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
		const present = {};
		for (const n of names) {
			let idx = lines.findIndex(l => l === n);
			let how = 'exact-line';
			if (idx === -1) {
				const re = new RegExp('(^|[^\w.])' + esc(n) + '($|[^\w.])');
				idx = lines.findIndex(l => re.test(l));
				how = 'boundary';
			}
			present[n] = idx === -1 ? null : { how: how, line: lines[idx], next: lines[idx + 1] || '' };
		}
		const statusRe = /(上传|解析|删除|不支持|提取|异常|失败|重试)/;
		const statusLines = lines.filter(l => statusRe.test(l)).slice(0, 80);
		return { present: present, statusLines: statusLines };
	})()`, string(namesJSON))

	if err := chromedp.Run(tabCtx, chromedp.Evaluate(js, &out)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	pretty, _ := json.MarshalIndent(out, "", "  ")
	for _, line := range strings.Split(string(pretty), "\n") {
		t.Log(line)
	}

	// Raw visible text of the composer region for manual reading.
	var body string
	if err := chromedp.Run(tabCtx, chromedp.Evaluate(`(document.body ? document.body.innerText : '').split('\n').map(s=>s.trim()).filter(Boolean).slice(0,320).join('\n')`, &body)); err == nil {
		t.Logf("--- body head ---\n%s", body)
	}
}
