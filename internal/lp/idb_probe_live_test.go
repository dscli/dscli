package lp

// Gated read-only live probe for the IndexedDB conversation record (design C
// of docs/task-webchat-recovery-extraction.md): after a recovery
// (auto-continue / regenerate) the IDB read failed 6/6 times, so this probe
// dumps the record's metadata to decide whether a more accurate candidate
// selection could read the 2/2 reply.
//
// It navigates to a real conversation with the LOGGED-IN dscli profile and
// dumps:
//
//   - the history-message record's version / timestamp / cacheResetAt;
//   - the newest 40 chat_messages as {index, role, status, fragments}; with
//     DSCLI_IDB_PROBE_MATCH set (comma-separated substrings) every message
//     is scanned instead and the output is a +-3-message window around each
//     fragment hit - the record holds the whole conversation, so this
//     locates one round inside it.
//
// READ-ONLY: it never sends a message and never executes a block. The
// browser window is VISIBLE (the site hydrates the record through its own
// client) and the tab is closed when the probe returns.
//
//	DSCLI_LIVE_IDB_PROBE='<conversation id or URL>' \
//	DSCLI_IDB_PROBE_PROFILE=<copy of ~/.dscli/chrome-profile> \
//	  go test -v -timeout 300s -run TestLiveIDBRecordProbe ./internal/lp/
//
// Without DSCLI_LIVE_IDB_PROBE set it skips (the default in CI and in a
// normal test pass).

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// idbProbeProfileEnv overrides the Chrome profile the probe boots with. The
// production profile is locked by the user's running dscli session, so the
// probe must run on a COPY (cookies live in the copy, so the site still
// recognizes the login).
const idbProbeProfileEnv = "DSCLI_IDB_PROBE_PROFILE"

// idbProbeAllocator boots a visible Chromium on the probe's own profile and
// returns the allocator context. Unlike NewChromium it takes the profile
// from the environment so a read-only probe never contends for the lock on
// the live profile.
func idbProbeAllocator(t *testing.T) (context.Context, func()) {
	t.Helper()
	chromePath, err := findChrome()
	if err != nil {
		t.Skipf("no Chrome/Chromium available: %v", err)
	}
	profile := strings.TrimSpace(os.Getenv(idbProbeProfileEnv))
	if profile == "" {
		t.Skipf("set %s=<path to a COPY of ~/.dscli/chrome-profile> to run (the live profile is locked)", idbProbeProfileEnv)
	}
	opts := append(chromiumExecAllocatorOptions(chromePath, profile),
		chromedp.Flag("disable-gpu", true))
	allocCtx, allocCancel := chromedp.NewExecAllocator(context.Background(), opts...)
	return allocCtx, allocCancel
}

// idbProbeDumpFmt dumps the conversation record's shape without any
// interpretation. Fragments are reported by type and rune length only - the
// probe never logs reply text.
//
// Without DSCLI_IDB_PROBE_MATCH the newest 40 messages are dumped. With it
// (matching substrings), every message is scanned instead, the output is a
// +-3-message window around each fragment hit (the record holds the whole
// conversation, so this locates one round inside it), and the hit indices
// are reported separately. %s is the JSON array of patterns.
const idbProbeDumpFmt = `(async () => {
	const pats = %s;
	const m = location.pathname.match(/\/a\/chat\/s\/([A-Za-z0-9_-]+)/);
	if (!m) return {ok: false, reason: 'no conversation id in url'};
	const db = await new Promise(function(res, rej) {
		const req = indexedDB.open('deepseek-chat');
		req.onsuccess = function() { res(req.result); };
		req.onerror = function() { rej(req.error); };
	});
	const rec = await new Promise(function(res, rej) {
		const tx = db.transaction('history-message', 'readonly');
		const r = tx.objectStore('history-message').get(m[1]);
		r.onsuccess = function() { res(r.result); };
		r.onerror = function() { rej(r.error); };
	});
	db.close();
	if (!rec) return {ok: false, reason: 'no idb record'};
	const msgs = (rec.data && rec.data.chat_messages) || [];
	const matches = [];
	const hits = [];
	for (let i = 0; i < msgs.length; i++) {
		const h = [];
		if (pats.length) {
			for (const p of pats) {
				for (const f of (msgs[i].fragments || [])) {
					if (f && typeof f.content === 'string' && f.content.indexOf(p) !== -1) { h.push(p); break; }
				}
			}
			if (h.length) matches.push(i);
		}
		hits.push(h);
	}
	const keep = [];
	if (pats.length) {
		const seen = {};
		for (const mi of matches) {
			for (let j = Math.max(0, mi - 3); j <= Math.min(msgs.length - 1, mi + 3); j++) seen[j] = true;
		}
		for (const k of Object.keys(seen)) keep.push(Number(k));
		keep.sort(function(a, b) { return a - b; });
	} else {
		for (let i = 0; i < msgs.length && i < 40; i++) keep.push(i);
	}
	const out = [];
	for (const i of keep) {
		const msg = msgs[i];
		const frags = [];
		for (const f of (msg.fragments || [])) {
			frags.push({
				type: (f && f.type) || '',
				len: (f && typeof f.content === 'string') ? f.content.length : -1,
			});
		}
		out.push({
			i: i,
			role: msg.role || '',
			status: msg.status || '',
			atu: (typeof msg.accumulated_token_usage === 'number') ? msg.accumulated_token_usage : null,
			hits: hits[i],
			frags: frags,
		});
	}
	return {
		ok: true,
		version: rec.version,
		timestamp: rec.timestamp,
		cacheResetAt: rec.cacheResetAt,
		total: msgs.length,
		matches: matches,
		msgs: out,
	};
})()`

func TestLiveIDBRecordProbe(t *testing.T) {
	raw := strings.TrimSpace(os.Getenv("DSCLI_LIVE_IDB_PROBE"))
	if raw == "" {
		t.Skip("set DSCLI_LIVE_IDB_PROBE=<conversation id or URL> to run")
	}
	url := raw
	if ConversationIDFromURL(url) == "" {
		url = constructConversationURL(raw)
	}

	// DSCLI_IDB_PROBE_MATCH locates a round inside the whole record: the
	// dump then scans every message and prints a window around each hit.
	pats := []string{}
	if rawPats := strings.TrimSpace(os.Getenv("DSCLI_IDB_PROBE_MATCH")); rawPats != "" {
		for _, p := range strings.Split(rawPats, ",") {
			if p = strings.TrimSpace(p); p != "" {
				pats = append(pats, p)
			}
		}
	}
	patsJSON, err := json.Marshal(pats)
	if err != nil {
		t.Fatalf("match patterns: %v", err)
	}

	allocCtx, allocCancel := idbProbeAllocator(t)
	defer allocCancel()

	// Generous deadline: the site needs time to hydrate the record after
	// the navigation.
	bootCtx, bootCancel := context.WithTimeout(allocCtx, 240*time.Second)
	defer bootCancel()
	tabCtx, tabCancel := chromedp.NewContext(bootCtx)
	defer tabCancel()

	var dump map[string]any
	if err := chromedp.Run(
		tabCtx,
		chromedp.Navigate(url),
		chromedp.WaitReady("body"),
		chromedp.Sleep(8*time.Second),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return evaluateAwait(ctx, fmt.Sprintf(idbProbeDumpFmt, patsJSON), &dump)
		}),
	); err != nil {
		t.Fatalf("probe run: %v", err)
	}

	ok, _ := dump["ok"].(bool)
	if !ok {
		t.Fatalf("dump failed: %v", dump["reason"])
	}
	t.Logf("url=%s", url)
	t.Logf("record: version=%v timestamp=%v cacheResetAt=%v totalMessages=%v",
		dump["version"], dump["timestamp"], dump["cacheResetAt"], dump["total"])
	if matches, _ := dump["matches"].([]any); len(matches) > 0 {
		t.Logf("match indices (newest-first): %v", matches)
	}

	msgs, _ := dump["msgs"].([]any)
	mode := "newest-40"
	if len(pats) > 0 {
		mode = "match-window(+/-3)"
	}
	t.Logf("dump: %d messages (newest-first, %s):", len(msgs), mode)
	for _, raw := range msgs {
		msg, _ := raw.(map[string]any)
		frags, _ := msg["frags"].([]any)
		var desc []string
		for _, rf := range frags {
			f, _ := rf.(map[string]any)
			desc = append(desc, fmt.Sprintf("%v(len=%v)", f["type"], f["len"]))
		}
		line := map[string]any{
			"i": msg["i"], "role": msg["role"], "status": msg["status"],
			"atu": msg["atu"], "frags": desc,
		}
		if hits, _ := msg["hits"].([]any); len(hits) > 0 {
			line["hits"] = hits
		}
		b, _ := json.Marshal(line)
		t.Logf("  %s", b)
	}
}
