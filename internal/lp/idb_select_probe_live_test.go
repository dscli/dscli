package lp

// Gated fixture probe for the IndexedDB answer reader (jsIDBGetAnswerFmt, see
// webchat.go and docs/task-webchat-recovery-extraction.md). It serves a local
// page (httptest, no network) at a matching /a/chat/s/<id> path with a
// stubbed indexedDB, seeds synthetic records and runs the PRODUCTION reader
// against them - the behavioural guard beside the string pins:
//
//  1. a regenerated round ([USER, stopped attempt (THINK only), regenerated
//     answer]) must return the regenerated answer, never the empty stopped
//     shell - the live-record shape of 2026-09-15;
//  2. a normal single-attempt round returns its answer;
//  3. a stopped round with no completed attempt reports its empty text (the
//     caller rejects it and keeps polling) instead of a stale round;
//  4. a record predating this round (the user message does not match) is
//     refused with the staleness reason;
//  5. a hypothetical flipped version order still yields the answer.
//
//	DSCLI_LIVE_EXTRACT_PROBE=1 go test -v -run TestLiveIDBSelectProbe ./internal/lp/
//
// Without DSCLI_LIVE_EXTRACT_PROBE=1 it skips (the default in CI and in a
// normal test pass).

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"
)

// idbSelectProbePage is the fixture page. location.pathname must match the
// reader's /a/chat/s/<id> regex, so the page is served over httptest (a
// file:// path can neither match nor be rewritten); everything else is a
// stub: indexedDB.open / transaction / objectStore.get resolve with
// window.__record, which each case seeds.
const idbSelectProbePage = `<!doctype html><html><head><meta charset="utf-8"><title>idb select probe</title>
<script>
window.__record = null;
window.__db = {
	close: function() {},
	transaction: function() {
		return {
			objectStore: function() {
				return {
					get: function() {
						var r = {};
						setTimeout(function() { r.result = window.__record; if (r.onsuccess) r.onsuccess(); }, 0);
						return r;
					}
				};
			}
		};
	}
};
var stub = {
	open: function() {
		var req = {};
		setTimeout(function() { req.result = window.__db; if (req.onsuccess) req.onsuccess(); }, 0);
		return req;
	}
};
try { Object.defineProperty(window, 'indexedDB', {value: stub, configurable: true}); }
catch (e) { window.indexedDB = stub; }
</script></head><body>probe</body></html>`

// idbSelectRecord wraps messages into the record shape the reader expects;
// the timestamp is fresh so the freshness guard passes.
func idbSelectRecord(msgs ...map[string]any) map[string]any {
	return map[string]any{
		"timestamp": time.Now().UnixMilli(),
		"data":      map[string]any{"chat_messages": msgs},
	}
}

// idbSelectUser builds a USER message carrying REQUEST fragment text.
func idbSelectUser(text string) map[string]any {
	return map[string]any{
		"role":                    "USER",
		"status":                  "FINISHED",
		"fragments":               []any{map[string]any{"type": "REQUEST", "content": text}},
		"accumulated_token_usage": 100,
	}
}

// idbSelectAssistant builds an ASSISTANT message with the given fragments.
func idbSelectAssistant(status string, frags ...map[string]any) map[string]any {
	anyFrags := make([]any, 0, len(frags))
	for _, f := range frags {
		anyFrags = append(anyFrags, f)
	}
	return map[string]any{
		"role":                    "ASSISTANT",
		"status":                  status,
		"fragments":               anyFrags,
		"accumulated_token_usage": 200,
	}
}

// idbSelectFrag builds one fragment of the given type and content.
func idbSelectFrag(typ, content string) map[string]any {
	return map[string]any{"type": typ, "content": content}
}

func TestLiveIDBSelectProbe(t *testing.T) {
	if os.Getenv("DSCLI_LIVE_EXTRACT_PROBE") != "1" {
		t.Skip("live extract probes: set DSCLI_LIVE_EXTRACT_PROBE=1 (headless Chromium on local fixtures)")
	}
	chromePath, err := findChrome()
	if err != nil {
		t.Skipf("no Chrome/Chromium available: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprint(w, idbSelectProbePage)
	}))
	defer srv.Close()

	// Headless, throwaway profile: never touch the real logged-in dscli
	// profile. The profile dir is a MkdirTemp rather than t.TempDir because
	// Chromium keeps writing into it after the test body returns.
	profileDir, err := os.MkdirTemp("", "dscli-idb-select-probe-")
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

	if err := chromedp.Run(
		tabCtx,
		chromedp.Navigate(srv.URL+"/a/chat/s/probe-conversation"),
		chromedp.WaitReady("body"),
	); err != nil {
		t.Fatalf("open probe page: %v", err)
	}

	run := func(t *testing.T, record map[string]any, wantMsg string) map[string]any {
		t.Helper()
		recJSON, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		var out map[string]any
		if err := chromedp.Run(
			tabCtx,
			chromedp.Evaluate("window.__record = "+string(recJSON), nil),
			chromedp.ActionFunc(func(ctx context.Context) error {
				return evaluateAwait(ctx, fmt.Sprintf(jsIDBGetAnswerFmt, quoteJS(wantMsg), time.Now().UnixMilli()), &out)
			}),
		); err != nil {
			t.Fatalf("run reader: %v", err)
		}
		return out
	}

	const sent = "output of script91.sh (attached as script91.txt):"

	t.Run("regenerated round returns the answer, not the stopped attempt", func(t *testing.T) {
		out := run(t, idbSelectRecord(
			idbSelectUser(sent),
			idbSelectAssistant("FINISHED", idbSelectFrag("THINK", "Let me re-read the ruler ...")),
			idbSelectAssistant(
				"FINISHED",
				idbSelectFrag("THINK", "So fig-05/06 should be ..."),
				idbSelectFrag("RESPONSE", "裁到页脚了。重新按正确范围裁切。\n<shell>\n<script>\nprint(1)\n</script>\n</shell>"),
			),
		), sent)
		if ok, _ := out["found"].(bool); !ok {
			t.Fatalf("found = false (%v), the record must be read", out["reason"])
		}
		text, _ := out["text"].(string)
		if !strings.Contains(text, "裁到页脚了") {
			t.Errorf("text must be the regenerated answer, got %q", text)
		}
		if strings.Contains(text, "Let me re-read the ruler") {
			t.Errorf("text picked the stopped first attempt: %q", text)
		}
		if status, _ := out["status"].(string); status != "FINISHED" {
			t.Errorf("status = %q, want FINISHED", status)
		}
	})

	t.Run("normal single-attempt round", func(t *testing.T) {
		out := run(t, idbSelectRecord(
			idbSelectUser(sent),
			idbSelectAssistant("FINISHED", idbSelectFrag("RESPONSE", "plain answer")),
		), sent)
		if text, _ := out["text"].(string); text != "plain answer" {
			t.Errorf("text = %q, want %q", text, "plain answer")
		}
	})

	t.Run("stopped round without an answer keeps the polling semantics", func(t *testing.T) {
		out := run(t, idbSelectRecord(
			idbSelectUser(sent),
			idbSelectAssistant("FINISHED", idbSelectFrag("THINK", "thinking only")),
		), sent)
		if ok, _ := out["found"].(bool); !ok {
			t.Fatalf("found = false, want the empty stopped attempt reported as-is")
		}
		if text, _ := out["text"].(string); text != "" {
			t.Errorf("text = %q, want an empty text (caller rejects and polls)", text)
		}
	})

	t.Run("record predating the round is refused", func(t *testing.T) {
		out := run(t, idbSelectRecord(
			idbSelectUser("some other message"),
			idbSelectAssistant("FINISHED", idbSelectFrag("RESPONSE", "stale")),
		), sent)
		if ok, _ := out["found"].(bool); ok {
			t.Fatalf("found = true, want the predates guard to refuse")
		}
		if reason, _ := out["reason"].(string); !strings.Contains(reason, "predates") {
			t.Errorf("reason = %q, want the predates refusal", reason)
		}
	})

	t.Run("flipped version order still yields the answer", func(t *testing.T) {
		out := run(t, idbSelectRecord(
			idbSelectUser(sent),
			idbSelectAssistant(
				"FINISHED",
				idbSelectFrag("THINK", "v2 first (hypothetical order)"),
				idbSelectFrag("RESPONSE", "answer v2"),
			),
			idbSelectAssistant("FINISHED", idbSelectFrag("THINK", "stopped shell second")),
		), sent)
		if text, _ := out["text"].(string); text != "answer v2" {
			t.Errorf("text = %q, want %q", text, "answer v2")
		}
	})
}
