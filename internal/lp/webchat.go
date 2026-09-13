package lp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/lockfile"
	"github.com/nanjj/clog"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/input"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// ErrLoginRequired is returned when the browser is not logged in to DeepSeek.
// Callers should trigger a visible login flow and retry.
var ErrLoginRequired = errors.New("login required — open visible browser to complete login")

// ErrServerBusy is returned when DeepSeek reports a temporary overload,
// either explicitly ("服务器忙，请稍后再试") or implicitly (a stable page
// with no answer content for a long time). This maps to the official API
// error codes 429 (rate limit), 500 (server error) and 503 (overloaded),
// whose documented remedy is "retry your request after a brief wait".
// Callers should retry with a backoff; the error text may wrap the
// server-provided message via %w.
var ErrServerBusy = errors.New("deepseek server busy — retry after a brief wait")

// ErrSendRejected is returned when the message was never acknowledged as
// sent: the textarea keeps the input, no generation signal appears and no
// new content shows up within the confirmation window. This typically means
// the server rejected the submit while overloaded. Retryable like
// ErrServerBusy.
var ErrSendRejected = errors.New("message send not acknowledged — server may be busy")

// ErrTruncated is returned when an extracted response shows clear structural
// signs of premature termination (an unclosed code fence or unterminated
// JSON), or when the server interrupted the generation mid-flight (the
// 「继续生成」 button appeared on the message) and the automatic continue did
// not resume it within budget: the generation was cut off mid-output while
// the server was overloaded. It is distinct from ErrServerBusy (no answer at
// all) and from the poll-budget timeout (the server kept the stream open):
// the answer EXISTS but is incomplete, so retrying the whole request is the
// only useful recovery.
var ErrTruncated = errors.New("response truncated — output cut off mid-generation")

const (
	deepseekChatURL = "https://chat.deepseek.com"

	// Polling configuration for response detection.
	webChatPollInterval     = 2 * time.Second // interval between polls
	webChatStablePolls      = 3               // text unchanged for this many polls = tentative done
	webChatExtendedPolls    = 10              // additional stable polls before force-extraction (escape hatch)
	webChatMaxPolls         = 300             // max polls before timeout (600s total)
	webChatConfirmPolls     = 5               // send-ack window: polls to observe send confirmation (10s)
	webChatEmptyStablePolls = 30              // stable-with-no-content polls (60s) before treating as server busy
	webChatMaxResends       = 3               // automatic resend attempts before giving up
	webChatResendCooldown   = 8 * time.Second // minimum gap between auto resends (UI settle time)

	// Auto-continue recovery for a server-interrupted generation (the site
	// renders a 「继续生成」 button on the stopped message). The click budget
	// is separate from webChatMaxResends: a busy server can interrupt a
	// resume as well, and each interruption is a distinct UI event.
	webChatMaxContinues         = 3                // max auto clicks of 「继续生成」 within one wait
	webChatContinueCooldown     = 8 * time.Second  // minimum gap between two clicks (UI flip time)
	webChatContinueResumeWindow = 45 * time.Second // budget for the resume to prove itself after a click

	// webChatMaxContinueClickFailures caps CONSECUTIVE CDP dispatch failures
	// for the continue click. A dispatch failure does not consume the click
	// budget (see continueRecovery.click), so without this cap a page whose
	// input pipeline is broken would just poll out; failing fast into
	// ErrTruncated gives the caller the retryable continue-from-here path
	// instead of a generic timeout.
	webChatMaxContinueClickFailures = 3

	// webChatTextareaWait is how long webchatSend polls for the chat
	// composer before concluding the page is not a chat page. A new
	// conversation used to sleep a blind 3s after navigation; a cold
	// Chrome boot plus React hydration can exceed that, and the set-value
	// step then failed with a misleading "login required" error. The
	// bounded poll keeps recovery honest: the sign-in page fails FAST
	// into the login flow (see waitForChatTextarea), while a logged-in
	// slow hydration gets a generous window. The sign-in marker is
	// debounced (two consecutive samples) so a transient login-form
	// flash during hydration cannot misroute a logged-in session.
	webChatTextareaWait = 60 * time.Second

	// IndexedDB extraction (the authoritative answer source, see
	// webchatExtractReloadedIDB): the record write trails the DOM render,
	// and a slow page hydration must not silently degrade the answer to the
	// DOM-extracted text - retry the reload a few times and give each read
	// a generous window before falling back. Worst case this path takes
	// ~3 x (15s reload + 15s read) + 2s between attempts ~= 92s; that is
	// deliberate (extraction completeness beats latency here) but any
	// change to these constants should be mindful of the total.
	webChatIDBPollWindow   = 15 * time.Second // read window per reload attempt
	webChatIDBReadAttempts = 3                // reload+read attempts before DOM fallback

	// JS snippet to set a textarea's value via the native setter (triggers
	// message string.
	jsSetTextareaFmt = `(() => {
	const ta = document.querySelector('textarea');
	if (!ta || ta.offsetParent === null) {
		return {error: 'no visible textarea — login required'};
	}
	const setter = Object.getOwnPropertyDescriptor(
		HTMLTextAreaElement.prototype, 'value'
	).set;
	setter.call(ta, %s);
	ta.dispatchEvent(new Event('input', {bubbles: true}));
	return {success: true};
})()`

	// jsChatReadyState classifies the page while the chat composer is
	// missing: "ok" when a textarea exists with layout (offsetParent is
	// null for position:fixed elements even when visible, so zero-size
	// rect is the display:none truth), "login" on the sign-in page (its
	// .ds-sign-in-form-wrapper is stable, un-hashed markup), "waiting"
	// otherwise (hydration still in progress). Used by
	// waitForChatTextarea: a plain "textarea missing" cannot be
	// distinguished from login by the absence alone, so the sign-in
	// marker gives the fast path while a slow chat page keeps polling.
	jsChatReadyState = `(() => {
		const ta = document.querySelector('textarea');
		if (ta) {
			if (ta.offsetParent !== null) return 'ok';
			const r = ta.getBoundingClientRect();
			if (r.width > 0 && r.height > 0) return 'ok';
		}
		if (document.querySelector('.ds-sign-in-form-wrapper, .ds-sign-in-form')) return 'login';
		return 'waiting';
	})()`

	// jsFindFileInput reports whether the chat page has a file input in the
	// DOM. Modern chat UIs pre-render a hidden <input type="file"> and open
	// it from the paperclip button, so uploads usually need no click at all.
	jsFindFileInput = `(() => {
		return {found: !!document.querySelector('input[type="file"]')};
	})()`

	// jsClickUploadBtnFmt clicks the upload (paperclip) button by
	// aria-label/title/text heuristics. Used when the file input is not in
	// the DOM and must be revealed by the button.
	jsClickUploadBtnFmt = `(() => {
		const keys = ['上传', '附件', 'upload', 'attachment', 'paperclip'];
		const els = document.querySelectorAll('button, [role="button"], [aria-label], [title]');
		for (const el of els) {
			if (el.offsetParent === null) continue;
			const t = (el.textContent || '').trim();
			if (t.length > 20) continue;
			const aria = ((el.getAttribute('aria-label') || '') + ' ' + (el.getAttribute('title') || '')).toLowerCase();
			const txt = t.toLowerCase();
			for (const k of keys) {
				if (aria.indexOf(k) !== -1 || txt.indexOf(k) !== -1) {
					el.click();
					return {success: true, matched: k};
				}
			}
		}
		return {success: false, error: 'upload button not found'};
	})()`

	// jsUploadReadyCountFmt counts upload-ready confirmations for the given
	// file names (%s is a JSON array of file base names, injected by
	// webchatSetUploadFiles). The page renders uploaded text/PDF attachments
	// as titled cards and images as blob/data thumbnails, so a name found in
	// the visible page text OR a thumbnail counts as one ready file; counts
	// are capped at the number of names (a mixed batch may match both ways).
	// The client-side wait loop polls this until every file is confirmed or
	// the budget expires (then the send proceeds with a warning). The
	// confirmation is optimistic by construction: pre-existing thumbnails in
	// a continued conversation, or a short file name matching unrelated page
	// text, can confirm early - the send retry and the fall-through warning
	// make this best-effort, not a guarantee.
	jsUploadReadyCountFmt = `(() => {
		const names = %s;
		const text = (document.body ? document.body.innerText : '') || '';
		let named = 0;
		for (const n of names) {
			if (n && text.indexOf(n) !== -1) named++;
		}
		const images = document.querySelectorAll('img[src^="blob:"], img[src^="data:image/"]').length;
		return {count: Math.min(named + images, names.length), named: named, images: images};
	})()`

	// jsGetAssistantText extracts all assistant response HTML from
	// .ds-assistant-message-main-content elements — the stable, un-hashed
	// answer container the site marks on each assistant message. It
	// returns the innerHTML parts (one per element) so the Go side can
	// convert the rendered DOM back to markdown — innerText loses code
	// fences, inline-code backticks and list markers, which breaks the
	// fidelity callers rely on.
	//
	// WHY not a bare .ds-markdown query: the deep-think reasoning block
	// ALSO renders as .ds-markdown (the collapsed "已思考" text), so a
	// bare query would mix the reasoning into the answer. The main-
	// content class is only on the visible answer; when the class is
	// gone (site redesign), the fallback query excludes blocks that
	// contain the "已思考（用时 N 秒）" status span.
	//
	// NOTE: the result may include pre-existing conversation history
	// (continued conversations); webchatWait strips it via mdBaseline.
	jsGetAssistantText = `(() => {
	const main = document.querySelectorAll('.ds-assistant-message-main-content');
	const all = main.length ? main : document.querySelectorAll('.ds-markdown');
	// Filter out elements that live in the sidebar/navigation panel.
	const els = Array.from(all).filter(function(el) {
		var p = el.parentElement;
		while (p) {
			var c = (p.className || '');
			var r = p.getAttribute && p.getAttribute('role') || '';
			if (/\b(sidebar|navigation)\b/i.test(c) || r === 'navigation') {
				return false;
			}
			p = p.parentElement;
		}
		if (!main.length) {
			// Fallback mode: drop the deep-think block. Its reasoning
			// text carries the status span "已思考（用时 N 秒）" — a
			// rendered answer never contains such a span.
			var spans = el.querySelectorAll('span');
			for (var i = 0; i < spans.length; i++) {
				var t = (spans[i].textContent || '').trim();
				if (/^已思考[（(]?/.test(t) && /[（(]?用时\s*\d+|\d+\s*秒/.test(t)) {
					return false;
				}
			}
		}
		return true;
	});
	if (els.length === 0) return [];
	// Concatenate ALL matched elements, not just the last one: streaming
	// responses may be split across multiple blocks.
	return els.map(function(el) { return el.innerHTML; });
})()`
	// jsLastAnswerAfterFmt returns the innerText of THIS round's answer
	// block: the last .ds-assistant-message-main-content that appears in
	// a message bubble AFTER the bubble containing the sent message %
	// (the user's text). Scoping on the message bubble (a stable class)
	// excludes both the deep-think reasoning (a sibling of the answer
	// block) and previous rounds' history (they sit BEFORE the sent
	// message). %s is the sent message.
	jsLastAnswerAfterFmt = `(() => {
	const want = %s;
	const bubbles = Array.from(document.querySelectorAll('.ds-message'));
	const ord = new Map();
	bubbles.forEach(function(b, i) { ord.set(b, i); });
	let anchor = -1;
	for (let i = bubbles.length - 1; i >= 0; i--) {
		const t = (bubbles[i].innerText || '');
		const firstLine = t.split('\n')[0].trim();
		if (firstLine === want || t.trim().indexOf(want) === 0) { anchor = i; break; }
	}
	if (anchor < 0) return '';
	const mains = Array.from(document.querySelectorAll('.ds-message .ds-assistant-message-main-content'));
	for (let i = mains.length - 1; i >= 0; i--) {
		const b = mains[i].closest('.ds-message');
		if (b && (ord.get(b) || 0) > anchor) return mains[i].innerText;
	}
	return '';
})()`
	// jsIDBGetAnswerFmt reads the assistant's answer from the site's own
	// IndexedDB conversation cache (database "deepseek-chat", object store
	// "history-message", keyed by conversation ID). This is the structured
	// data the web app renders, so it carries the ORIGINAL markdown of the
	// reply — no HTML→markdown reconstruction, no UI-chrome recovery.
	//
	// WHY IndexedDB and not the DOM: the site's message markup uses
	// deployment-hashed CSS class names (e.g. "_5255ff8" or "fbb737a4")
	// that change without notice — the historical .ds-markdown selector now
	// matches nothing — while the IDB schema (chat_messages →
	// fragments[].type=RESPONSE → content=markdown) is app-internal and
	// stable across UI redesigns.
	//
	// The assistant's fragments also include a THINK fragment (the deep-
	// think reasoning, hidden behind the "已思考" collapse); only RESPONSE
	// fragments form the visible answer. status=FINISHED means the reply
	// completed; before that the record is absent or still streaming, so
	// the caller keeps polling.
	//
	// %s is the sent message and %d the send time (Unix ms). A cached
	// record can be STALE for a continued conversation (it still shows the
	// previous round as FINISHED), so extraction is refused unless the
	// record's last USER message matches the sent message AND the record
	// was written around the send (within 2 minutes before it): the site
	// writes the conversation record once at creation, which can win the
	// race against the captured send time by milliseconds, so the
	// freshness window must tolerate that creation write while still
	// rejecting old records.
	//
	// chat_messages is NEWEST-FIRST (verified against a live record on
	// 2026-08-26: the current round's USER/ASSISTANT messages sit at index
	// 0/1 while the first round ever sits at the tail), and the record's
	// timestamp is refreshed on every page load, so the freshness window
	// never rejects a reloaded page. Because of the newest-first order the
	// "last" USER/ASSISTANT messages are the FIRST matches scanning from
	// index 0 - scanning from the tail would pick the FIRST round of a
	// continued conversation and fail the user-message guard.
	jsIDBGetAnswerFmt = `(async () => {
	try {
		const wantMsg = %s;
		const notBefore = %d;
		const m = location.pathname.match(/\/a\/chat\/s\/([A-Za-z0-9_-]+)/);
		if (!m) return {found: false, reason: 'no conversation id in url'};
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
		if (!rec) return {found: false, reason: 'no idb record'};
		if (typeof rec.timestamp !== 'number' || rec.timestamp < notBefore - 120000) {
			return {found: false, reason: 'idb record stale (before send)'};
		}
		const msgs = (rec.data && rec.data.chat_messages) || [];
		// Newest-first: this round's messages are at the front.
		let lastUser = null;
		let last = null;
		for (let i = 0; i < msgs.length; i++) {
			if (msgs[i].role === 'USER' && !lastUser) lastUser = msgs[i];
			if (msgs[i].role === 'ASSISTANT' && !last) last = msgs[i];
		}
		// The record must already contain THIS round's user message —
		// otherwise it is the pre-send snapshot of a continued
		// conversation and its FINISHED assistant message is stale.
		const userParts = [];
		if (lastUser) {
			for (const f of (lastUser.fragments || [])) {
				if (f && f.type === 'REQUEST' && typeof f.content === 'string') userParts.push(f.content);
			}
		}
		if (userParts.join('\n\n').trim() !== wantMsg.trim()) {
			return {found: false, reason: 'idb record predates this round'};
		}
		if (!last) return {found: false, reason: 'no assistant message yet'};
		const frags = last.fragments || [];
		const parts = [];
		const thinkParts = [];
		for (const f of frags) {
			if (f && typeof f.content === 'string' && f.content.trim() === '') continue;
			if (f && f.type === 'RESPONSE') parts.push(f.content);
			if (f && f.type === 'THINK') thinkParts.push(f.content);
		}
		// 本轮回复的 token 数 = 同轮 USER 与 ASSISTANT 消息的累计用量之差
		// （站点自己的运行计数器，与页面头部显示的数字一致；任一侧缺失
		// 或差值为负时按 0 处理，绝不伪造计数）。
		let tokens = 0;
		const uAtu = lastUser && lastUser.accumulated_token_usage;
		const aAtu = last && last.accumulated_token_usage;
		if (typeof uAtu === 'number' && typeof aAtu === 'number' && aAtu >= uAtu) {
			tokens = aAtu - uAtu;
		}
		return {
			found: true,
			status: last.status || '',
			text: parts.join('\n\n'),
			reason: thinkParts.join('\n\n'),
			respCount: parts.length,
			thinkCount: thinkParts.length,
			tokens: tokens,
		};
	} catch (e) {
		return {found: false, reason: String((e && e.message) || e)};
	}
})()`
	// jsIDBGetLastAssistantFmt reads the LAST assistant message of the
	// CURRENT conversation from the site's IndexedDB conversation cache —
	// the continuation point of a resumed conversation (HandleWebChatResume).
	//
	// Unlike jsIDBGetAnswerFmt it does NOT need a sent-message anchor or a
	// freshness window: there is no "this round" — the caller wants whatever
	// the expert last said, whether it is a final answer (multi-turn resume)
	// or a pending tool-call block (interrupted round, chat_messages is
	// newest-first so the last assistant message is the FIRST role=ASSISTANT
	// entry scanning from index 0).
	jsIDBGetLastAssistantFmt = `(async () => {
	try {
		const m = location.pathname.match(/\/a\/chat\/s\/([A-Za-z0-9_-]+)/);
		if (!m) return {found: false, reason: 'no conversation id in url'};
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
		if (!rec) return {found: false, reason: 'no idb record'};
		const msgs = (rec.data && rec.data.chat_messages) || [];
		let last = null;
		for (let i = 0; i < msgs.length; i++) {
			if (msgs[i].role === 'ASSISTANT') { last = msgs[i]; break; }
		}
		if (!last) return {found: false, reason: 'no assistant message'};
		const frags = last.fragments || [];
		const parts = [];
		for (const f of frags) {
			if (f && typeof f.content === 'string' && f.content.trim() === '') continue;
			if (f && f.type === 'RESPONSE') parts.push(f.content);
		}
		return {found: true, status: last.status || '', text: parts.join('\n\n')};
	} catch (e) {
		return {found: false, reason: String((e && e.message) || e)};
	}
})()`
	// jsIsGenerationActive checks whether the AI is still generating a response.
	// Returns true if a stop/cancel button is visible or the textarea is disabled
	// (both signals that generation is in progress). Used to distinguish between
	// genuine completion and a streaming pause.
	jsIsGenerationActive = `(() => {
		// Signal 1: a visible stop/cancel button during generation.
		var btns = document.querySelectorAll('button, [role="button"]');
		for (var i = 0; i < btns.length; i++) {
			var b = btns[i];
			if (b.offsetParent === null) continue;
			var txt = (b.textContent || '').trim().toLowerCase();
			var aria = (b.getAttribute('aria-label') || '').toLowerCase();
			if (txt.indexOf('stop') !== -1 || txt.indexOf('停止') !== -1 ||
				txt.indexOf('cancel') !== -1 || txt.indexOf('取消') !== -1 ||
				aria === 'stop' || aria === '停止') {
				return true;
			}
		}
		// Signal 2: textarea disabled during generation.
		var ta = document.querySelector('textarea');
		if (ta && ta.disabled) return true;
		return false;
	})()`

	// jsTextareaCleared reports whether the chat textarea has been emptied.
	// After a successful send the React-controlled textarea clears
	// immediately, so a non-empty textarea inside the confirmation window
	// means the submit was rejected (typically server overload).
	jsTextareaCleared = `(() => {
		const ta = document.querySelector('textarea');
		return !!ta && ta.value.trim() === '';
	})()`

	// jsEnterDispatchBase is the shared Enter keydown → keypress → keyup
	// dispatch sequence used by both jsSendEnter and jsSendEnterOnly.
	// KeyboardEvent dispatch is used instead of chromedp.KeyEvent because
	// the latter may not trigger React's event handling in a remote
	// allocator (chromium service) context. The full sequence (keydown →
	// keypress → keyup) matches what a real keyboard produces, improving
	// compatibility with frameworks that listen for specific events.
	// The IIFE is closed by each concatenated suffix, which also carries
	// its own return/fallback tail.
	jsEnterDispatchBase = `(() => {
		const ta = document.querySelector('textarea');
		if (!ta) return {error: 'no textarea'};
		if (ta.offsetParent === null) return {error: 'textarea not visible'};
		// Ensure the textarea has focus before dispatching keyboard events.
		// This is critical when the page loads with focus on the left sidebar
		// conversation list instead of the textarea.
		ta.click();
		ta.focus({preventScroll: true});
		if (document.activeElement !== ta) {
			ta.select();
		}
		var opts = {
			key: 'Enter', code: 'Enter', keyCode: 13, which: 13,
			bubbles: true, cancelable: true,
		};
		ta.dispatchEvent(new KeyboardEvent('keydown', opts));
		ta.dispatchEvent(new KeyboardEvent('keypress', opts));
		ta.dispatchEvent(new KeyboardEvent('keyup', opts));
	`
	// jsSendEnter additionally clicks the send button as a fallback to
	// ensure the message is submitted even if the KeyboardEvent dispatch
	// doesn't trigger React's submit handler (e.g. when focus is on the
	// left sidebar conversation list and the textarea isn't the active
	// element).
	jsSendEnter = jsEnterDispatchBase + `
		// Async fallback: React 18 batches state updates, so a synchronous
		// check right after dispatchEvent still sees the old textarea value
		// and would click the send button on top of a successfully
		// submitted Enter — double-sending the message (two identical user
		// messages observed in a real session). A macrotask window lets
		// React flush the controlled component first; only then is a stale
		// value a genuine "Enter was ignored" signal.
		setTimeout(function() {
			if (ta.value.trim() !== '') {
				var sendBtn = document.querySelector('[role="button"].ds-button--primary');
				if (sendBtn && sendBtn.offsetParent !== null) {
					sendBtn.click();
				}
			}
		}, 300);
		return {success: true};
	})()`

	// jsResendFailedFmt detects the failed-send retry button and clicks it.
	// When the server rejects a message (overload, network error), the site
	// keeps the message bubble in an error state with a "重发/重试" button
	// next to it. Two UI generations are handled:
	//
	//   - Text/aria path (older builds): visible text must match exactly
	//     (variants enumerated); aria-label/title match loosely (sites add
	//     extra words there).
	//   - Icon path (current build, verified 2026-08-27 against the live
	//     bundle: main.a006649905.js): the retry renders as a FILLED YELLOW
	//     CIRCLE with a redo-arrow SVG — no text, no aria-label, no title
	//     (the "重试" tooltip is a CSS overlay, not a DOM attribute). The
	//     button is <button class="ds-button ds-button--filled
	//     ds-button--circle ds-button--warning"> with the resend SVG. The
	//     ds-button classes are vendor-stable (not hashed), and the
	//     warning-filled circle is the only ds-button--warning button in
	//     the chat UI, so the class + SVG child check is a reliable match.
	//     Disabled is skipped: the button is disabled while a generation is
	//     running.
	//
	// Never the "重新回答/重新生成" action buttons that sit on COMPLETED
	// answers, so clicking there cannot corrupt a successful round into a
	// duplicate.
	jsResendFailedFmt = `(() => {
		const reExact = /^(重发|重试|重新发送|再次发送|重发消息|重新发送消息|发送失败[^]{0,10}(重发|重试)|resend|retry)$/;
		const reLoose = /(重发|重试|重新发送|再次发送|发送失败|resend|retry)/;
		const reExclude = /(重新(回答|生成|思考|加载)|regenerate|re-?answer|reload|refresh)/;
		const cands = document.querySelectorAll('button, [role="button"]');
		for (let i = 0; i < cands.length; i++) {
			const b = cands[i];
			// Disabled retries (a generation is running) must never be
			// clicked, on either path.
			if (b.disabled) continue;
			// Position:fixed elements (and popover/portal containers)
			// report offsetParent === null while being fully visible, so
			// offsetParent alone is not a visibility truth; the zero-size
			// rect check catches display:none / not-yet-laid-out.
			if (b.offsetParent === null) {
				const r = b.getBoundingClientRect();
				if (r.width === 0 || r.height === 0) continue;
			}
			const t = (b.textContent || '').trim().toLowerCase();
			const aria = (b.getAttribute('aria-label') || '').trim().toLowerCase();
			const title = (b.getAttribute('title') || '').trim().toLowerCase();
			if (reExclude.test(aria) || reExclude.test(title) || reExclude.test(t)) continue;
			if (t || aria || title) {
				const hit = reExact.test(t) || reLoose.test(aria) || reLoose.test(title);
				if (hit) {
					b.click();
					return {found: true, matched: t || aria || title || 'button'};
				}
			}
			// Icon-only retry (current UI): filled warning circle + SVG.
			if (b.classList.contains('ds-button--warning') && b.querySelector('svg')) {
				b.click();
				return {found: true, matched: 'icon retry (ds-button--warning)'};
			}
		}
		return {found: false};
	})()`

	// jsContinueGeneration detects the site's 「继续生成」 (Continue) button
	// on a server-interrupted assistant message and reports its viewport
	// centre WITHOUT clicking it.
	//
	// Site facts (bundle forensics 2026-09-13, main.d69e3d8c16.js, page
	// commit-id 5d128f98):
	//
	//   - Button text: 继续生成 (English bundle: "Continue").
	//   - The assistant message row is [avatar, .ds-message bubble, bottom
	//     action bar]. The button lives in the action bar and is therefore a
	//     SIBLING of the bubble, not a descendant of it. The .ds-message
	//     class is un-hashed, so "walk up until an ancestor directly owns a
	//     .ds-message child" is the ownership test - a button that is not
	//     inside a message row (settings panel, sidebar) never matches.
	//   - The site itself only renders the button on the LATEST interrupted
	//     message (status INCOMPLETE, no child messages), so no "which
	//     round" inference is needed on this side.
	//
	// This snippet deliberately performs NO click, and must never gain one:
	// the continue button is the ONLY element in the entire bundle whose
	// onClick validates isTrusted, so every synthetic click (el.click(),
	// dispatchEvent(new MouseEvent(...))) is silently ignored. The Go side
	// dispatches a real CDP input event instead - see clickTrustedAt.
	jsContinueGeneration = `(() => {
		const wants = ['继续生成', 'continue'];
		const cands = document.querySelectorAll('button, [role="button"]');
		let pick = null;
		for (let i = 0; i < cands.length; i++) {
			const b = cands[i];
			if (b.disabled) continue;
			if ((b.getAttribute('aria-disabled') || '') === 'true') continue;
			// position:fixed popovers report offsetParent === null while
			// being visible, so the zero-size rect is the display:none
			// truth (same visibility convention as jsResendFailedFmt).
			if (b.offsetParent === null) {
				const r0 = b.getBoundingClientRect();
				if (r0.width === 0 || r0.height === 0) continue;
			}
			const t = (b.textContent || '').trim().toLowerCase();
			const aria = (b.getAttribute('aria-label') || '').trim().toLowerCase();
			const title = (b.getAttribute('title') || '').trim().toLowerCase();
			// EXACT match: 重新生成 / 重新回答 (regenerate) must never hit.
			let label = '';
			if (wants.indexOf(t) !== -1) label = t;
			else if (wants.indexOf(aria) !== -1) label = aria;
			else if (wants.indexOf(title) !== -1) label = title;
			if (!label) continue;
			// Ownership: nearest ancestor that directly owns a .ds-message
			// bubble is the button's own message row.
			let owned = false;
			let p = b.parentElement;
			while (p) {
				const kids = p.children;
				for (let k = 0; k < kids.length; k++) {
					const c = kids[k];
					if (c.classList && c.classList.contains('ds-message')) { owned = true; break; }
				}
				if (owned) break;
				p = p.parentElement;
			}
			if (!owned) continue;
			// Keep the LAST match: document order ends at the newest row.
			pick = {b: b, label: label};
		}
		if (!pick) return {found: false};
		const b = pick.b;
		if (b.scrollIntoView) b.scrollIntoView({block: 'center'});
		const r = b.getBoundingClientRect();
		const x = r.left + r.width / 2;
		const y = r.top + r.height / 2;
		// Occlusion check: the caller clicks by coordinate, so a covered
		// button would send the click somewhere else entirely. A present but
		// blocked button is reported as such, NOT as absent: folding it into
		// found=false would let the caller read the poll as healthy and
		// extract the interrupted fragment.
		const hit = document.elementFromPoint(x, y);
		const clickable = !!hit && (hit === b || b.contains(hit));
		return {found: true, clickable: clickable, label: pick.label, x: x, y: y};
	})()`

	// jsSendEnterOnly dispatches the Enter sequence without the send-button
	// fallback. Used by the resend loop when the message is still sitting in
	// the textarea (submit rejected and the site restored the text): the
	// async fallback inside jsSendEnter could race the resend state reset.
	jsSendEnterOnly = jsEnterDispatchBase + `
		return {success: true};
	})()`
)

// WebChatOptions configures a WebChat call.
type WebChatOptions struct {
	// Attachments are file paths (images, text, PDF) uploaded to the chat
	// before the message is sent (up to 50 files, 100MB total - see
	// WebUploadMaxFiles / WebUploadMaxTotal).
	Attachments []string

	// Keep continues a saved conversation instead of starting a new one.
	// Empty means a new conversation. Special values:
	//   "last" — the most recently saved conversation;
	//   "<id>" — a specific conversation ID, as returned in a previous
	//     call's WebChatResult.URL (use ConversationIDFromURL to extract);
	//   "<full URL>" — a chat.deepseek.com conversation URL (e.g. copied
	//     from a browser); the conversation is registered for later use.
	// Use keep="list" with ListConversations to enumerate saved ones.
	Keep string

	// Role names a role-specific prompt template (expert/review/dev, see
	// prompt.RenderPromptForRole) that HandleWebChat prepends to the
	// message. Non-empty Role also enables the DSML tool loop. Only
	// HandleWebChat consumes it - WebChatWithOptions ignores it.
	// HandleWebChat injects it only when Keep == "" (a new conversation)
	// and SkipPromptInjection is false; a resumed conversation does not
	// re-inject the prompt (it was already in the first round's history).
	Role string

	// System is raw persona text that HandleWebChat prepends to the message.
	// It takes precedence over Role. Only HandleWebChat consumes it -
	// WebChatWithOptions ignores it. HandleWebChat injects it only when
	// Keep == "" (a new conversation) and SkipPromptInjection is false; a
	// resumed conversation does not re-inject it.
	System string

	// SkipPromptInjection suppresses the first-round Role/System prompt
	// injection while keeping Role's other effects (DSML gating and markup
	// stripping, role labels): the caller supplies the role instructions by
	// other means, e.g. as an uploaded attachment (see code_review). Only
	// HandleWebChat consumes it - WebChatWithOptions rejects it.
	SkipPromptInjection bool

	// ShellTool switches the consultation to the <shell> block channel:
	// the role prompt carries the shell tool doc in place of the DSML doc,
	// and HandleWebChat / HandleWebChatResume route replies through the
	// shell loop (judge, execute locally, feed the merged output back as
	// an attached scriptN.txt). Only the handle layer consumes it -
	// WebChatWithOptions rejects it. See docs/task-shell-block.md.
	ShellTool bool
}

// WebChatResult is the outcome of a WebChat call: the assistant's visible
// answer in original markdown (Content), its deep-think reasoning when the
// site exposes it (Reasoning, "" if unavailable), plus the final conversation
// URL, which contains the conversation ID usable with WebChatOptions.Keep to
// continue the same conversation later. URL is "" if it could not be
// determined.
type WebChatResult struct {
	Content string
	// Reasoning is the deep-think reasoning (THINK fragments) when the
	// site exposes it; "" when unavailable. Consumed by the tool loop's
	// per-round print (outfmt.PrintContent) and by callers that want the
	// full reply.
	Reasoning string
	URL       string

	// OutputTokens is this reply's total output token count (thinking +
	// content combined), derived from the site's IndexedDB
	// accumulated_token_usage difference between the round's ASSISTANT and
	// USER messages (the site's own running counter). The count is no
	// longer printed in the reply header (the header now shows the role);
	// the field is kept for diagnostics and future reuse. 0 when the counts
	// are unavailable (DOM fallback, or a mock transport).
	OutputTokens int

	// Printed reports that the reply (Content and Reasoning) was already
	// printed by the DSML tool loop via outfmt.PrintContent. Callers that
	// display the final answer must skip re-printing Content when set;
	// false for a one-shot reply with no tool loop.
	Printed bool
}

// WebChat sends a message to chat.deepseek.com via a local Chrome/Chromium
// browser and returns the assistant's text response.
//
// The browser is launched fresh for each call and closed after the response is
// received. Cookies persist via the shared Chrome profile directory, so prior
// login state is available across calls.
//
// If ctx carries context.KeepKey set to "last" or a conversation ID, WebChat
// attempts to continue that conversation rather than starting a new one. New
// conversations use the site's current default model. Use WebChatWithOptions
// for file uploads and the full Keep value set.
func WebChat(ctx context.Context, message string) (string, error) {
	res, err := WebChatWithOptions(ctx, message, WebChatOptions{
		Keep: context.ContextValue(ctx, context.KeepKey, ""),
	})
	if err != nil {
		return "", err
	}
	return res.Content, nil
}

// WebChatWithOptions is WebChat with explicit attachment and continuation
// options. Attachment paths are resolved to absolute, and the result
// validated before a browser is launched, so bad input (missing/oversized
// attachments, unknown keep target) fails fast without starting Chrome.
//
// The browser is one-shot: launched for this send and closed afterwards.
// When called through HandleWebChat the context carries a shared
// webChatSession and the send reuses that browser instead.
//
// See WebChatOptions for the Keep default.
func WebChatWithOptions(ctx context.Context, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "WebChatWithOptions")
	defer span.Finish()

	resolved, err := resolveWebAttachments(opts.Attachments)
	if err != nil {
		return WebChatResult{}, err
	}
	opts.Attachments = resolved
	if err := validateWebChatOptions(opts); err != nil {
		return WebChatResult{}, err
	}
	convURL, err := resolveConversation(opts.Keep)
	if err != nil {
		return WebChatResult{}, err
	}
	return webChatWithURL(ctx, convURL, message, opts)
}

// validateWebChatOptions checks attachment limits before launching a browser,
// so bad input fails fast without starting Chrome.
func validateWebChatOptions(opts WebChatOptions) error {
	// Role/System/SkipPromptInjection/ShellTool are handle-level concerns
	// (prompt rendering + tool loops). Rejecting them here (instead of
	// silently ignoring) makes the layering explicit: a caller that passes
	// them to the transport is using the wrong entry point.
	if opts.Role != "" || opts.System != "" || opts.SkipPromptInjection || opts.ShellTool {
		return fmt.Errorf("Role/System/SkipPromptInjection/ShellTool are only honored by HandleWebChat")
	}
	return validateWebAttachments(opts.Attachments)
}

// --- shared browser session -------------------------------------------------

// webChatSession is the browser instance shared by every send of one
// HandleWebChat call: the initial message, backoff retries and DSML
// tool-loop follow-ups all reuse the same tab, and the browser is closed
// once when the consultation ends. Launching a fresh Chrome per round was
// the dominant cost of a multi-tool consultation (cold start + profile load;
// a design case study ran 9 consecutive tool calls).
//
// The browser is booted LAZILY on the first send, so argument/validation
// errors still fail fast without starting Chrome (see WebChatWithOptions).
// The tab context is pinned to the boot context: every send runs chromedp
// actions on it, so the boot deadline applies to the whole session (all
// sends of a consultation share it - identical to the pre-session behavior,
// where each one-shot browser also derived from the same parent). Tracing
// consequence: the first round's spans nest correctly under the send
// callers, while later rounds' work attaches to the boot branch.
//
// Retryable failures (ErrServerBusy / ErrSendRejected / ErrTruncated) leave
// the tab navigable: the next Send re-navigates to the conversation URL and
// recovers. Non-retryable chromedp errors abort the consultation, so no
// mid-session browser rebuild is needed.
//
// Close is safe concurrently with Send: the mutex protects the boot state.
// Calling Close makes the session reusable - a later Send boots a fresh
// browser.
type webChatSession struct {
	mu     sync.Mutex
	tabCtx context.Context // chromedp tab context; nil until booted
	stop   func()          // closes the tab and the browser; nil until booted
}

// newWebChatSession returns a session handle. The browser itself is not
// started until the first Send.
func newWebChatSession() *webChatSession {
	return &webChatSession{}
}

// ensureTab boots the shared browser on first use and returns the tab
// context. The context is pinned to the boot context: every action (send or
// read) runs chromedp actions on it, so the boot deadline applies to the
// whole session. Safe to call concurrently: the mutex serializes the boot.
func (s *webChatSession) ensureTab(ctx context.Context) (context.Context, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tabCtx == nil {
		// Boot with THIS call's context so the first round's spans and
		// deadline propagate through to the browser.
		allocCtx, cancel, err := NewChromium(ctx)
		if err != nil {
			return nil, err
		}
		tabCtx, tabClose := chromedp.NewContext(allocCtx)
		s.tabCtx = tabCtx
		s.stop = func() {
			tabClose()
			cancel()
		}
	}
	return s.tabCtx, nil
}

// Send performs one complete exchange on the session's browser, booting the
// browser on first use: navigate to the conversation, send the message, wait
// for and extract the response, then register the conversation for later
// continuation. The exchange semantics are webchatSend's.
func (s *webChatSession) Send(ctx context.Context, conversationURL, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "webChatSessionSend")
	defer span.Finish()
	tabCtx, err := s.ensureTab(ctx)
	if err != nil {
		return WebChatResult{}, err
	}

	// webchatSend wraps every error with "webchat:", so no extra prefix here
	// (a second one would produce "webchat: webchat: ...").
	response, reasoning, finalURL, tokens, err := webchatSend(tabCtx, conversationURL, message, opts, 0)
	if err != nil {
		return WebChatResult{}, err
	}
	if finalURL != "" {
		_ = registerConversation(finalURL)
	}
	return WebChatResult{Content: response, Reasoning: reasoning, URL: finalURL, OutputTokens: tokens}, nil
}

// Close shuts down the shared browser (tab and process). Safe before the
// first send - nothing was booted yet - and idempotent; it is the last thing
// HandleWebChat does.
func (s *webChatSession) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stop != nil {
		s.stop()
		s.stop = nil
		s.tabCtx = nil
	}
}

// webChatSessionKey is the internal context key that carries the shared
// session from HandleWebChat down to the transport.
type webChatSessionKey struct{}

// withWebChatSession attaches a session to ctx so subsequent transport calls
// reuse its browser.
func withWebChatSession(ctx context.Context, s *webChatSession) context.Context {
	return context.WithValue(ctx, webChatSessionKey{}, s)
}

// webChatSessionFrom returns the shared session attached to ctx, or nil when
// the transport is used directly (one-shot browser per send).
func webChatSessionFrom(ctx context.Context) *webChatSession {
	s, _ := ctx.Value(webChatSessionKey{}).(*webChatSession)
	return s
}

// webChatWithURL is the common implementation shared by new conversations
// (empty url) and continuation (saved url).
//
// When ctx carries a webChatSession (set by HandleWebChat), the send reuses
// that browser; in a direct transport call the browser is launched for
// exactly this send and closed afterwards.
func webChatWithURL(ctx context.Context, conversationURL, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "webChatWithURL")
	defer span.Finish()
	sess := webChatSessionFrom(ctx)
	if sess == nil {
		sess = newWebChatSession()
		defer sess.Close()
	}
	return sess.Send(ctx, conversationURL, message, opts)
}

// webchatSend sends a message and returns the response, the deep-think
// reasoning (if extractable), and the final page URL (which contains the
// conversation ID for continuation). If login is needed, it triggers a
// manual login flow in the same Chrome session and retries once.
//
// opts.Attachments are files (images, text, PDF) uploaded before sending.
func webchatSend(tabCtx context.Context, conversationURL, message string, opts WebChatOptions, retry int) (string, string, string, int, error) {
	span, ctx := clog.StartSpanFromContext(tabCtx, "webchatSend")
	defer span.Finish()
	navURL := conversationURL
	if navURL == "" {
		navURL = deepseekChatURL
	}
	isNewConv := (conversationURL == "")

	if !isNewConv {
		fmt.Fprintf(os.Stderr, "📋 继续会话: %s\n", conversationURL)
	}

	var baseline, response, reasoning, finalURL, mdBaseline string
	var mdBaselineParts []string
	var tokens int // this round's token count from IndexedDB (0 when unavailable)

	// Base navigation and page hydration. The clipboard permission is
	// granted first so the site's copy button (and the readText fallback)
	// work from the automated session; failure is non-fatal — extraction
	// falls back to DOM conversion.
	actions := []chromedp.Action{
		chromedp.ActionFunc(func(ctx context.Context) error {
			if err := webchatGrantClipboard(ctx); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️ 剪贴板权限设置失败: %v\n", err)
			}
			return nil
		}),
		chromedp.Navigate(navURL),
		chromedp.WaitReady("body"),
	}

	// Wait for the chat composer before touching it. The old
	// .ds-markdown selector no longer exists (the site dropped it in the
	// hashed-class redesign), so the textarea is the last always-present
	// chat control. New conversations previously slept a blind 3s, which
	// failed on a slow cold boot; continuing conversations used
	// WaitReady, which blocked forever on the sign-in page and masked the
	// login signal (there is no deadline on the tab context). The bounded
	// poll handles both: login fails fast into the recovery flow, and a
	// slow hydration keeps waiting up to webChatTextareaWait. The settle
	// delay afterwards lets React finish rendering the composer controls.
	actions = append(
		actions,
		chromedp.ActionFunc(func(ctx context.Context) error {
			return waitForChatTextarea(ctx)
		}),
		chromedp.Sleep(3*time.Second),
	)

	// Upload attachments (images, text, PDF). Fatal on failure: files the
	// caller asked to attach must not be silently dropped.
	if len(opts.Attachments) > 0 {
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			return webchatUpload(ctx, opts.Attachments)
		}))
	}

	actions = append(
		actions,
		// Record baseline text before sending.
		chromedp.Evaluate("document.body ? document.body.innerText : ''", &baseline),

		// Record the .ds-markdown baseline (all assistant HTML visible
		// before sending). webchatWait strips this prefix from the
		// extracted text so continued conversations don't include
		// pre-existing history. The conversion is its own action: action
		// contexts are only valid inside chromedp.Run.
		chromedp.Evaluate(jsGetAssistantText, &mdBaselineParts),
		chromedp.ActionFunc(func(ctx context.Context) error {
			mdBaseline = joinMarkdown(mdBaselineParts)
			return nil
		}),

		// Set the textarea value (JS needed for React-controlled inputs).
		chromedp.ActionFunc(func(ctx context.Context) error {
			return webchatSetValue(ctx, message)
		}),

		// Brief delay then dispatch Enter via JS to send.
		// JS KeyboardEvent dispatch is used instead of chromedp.KeyEvent
		// because the latter may not trigger React's event handling in a
		// remote allocator context (chromium service).
		chromedp.Sleep(500*time.Millisecond),
		chromedp.Evaluate(jsSendEnter, nil),

		// Wait for and extract the assistant response.
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			response, err = webchatWait(ctx, baseline, mdBaseline, message)
			return err
		}),

		// Capture the final URL (contains conversation ID).
		chromedp.Location(&finalURL),

		// Reload and re-read the answer from the site's IndexedDB cache:
		// the site writes the conversation record once per page load, so
		// a live poll during the send can never see the new round. After
		// the reload the record is complete (FINISHED) and carries the
		// ORIGINAL markdown plus the deep-think THINK fragments. IDB is
		// the authoritative extraction; DOM text is the degraded fallback,
		// so a slow hydration retries the reload instead of silently
		// downgrading the answer. Only after all attempts fail does the
		// DOM-extracted response stay, with an explicit warning.
		chromedp.ActionFunc(func(ctx context.Context) error {
			for attempt := 1; attempt <= webChatIDBReadAttempts; attempt++ {
				if content, think, tk, ok := webchatExtractReloadedIDB(ctx, finalURL, message); ok {
					response = content
					reasoning = think
					tokens = tk
					return nil
				}
				if attempt < webChatIDBReadAttempts {
					select {
					case <-ctx.Done():
						return nil
					case <-time.After(time.Second):
					}
				}
			}
			fmt.Fprintf(os.Stderr, "⚠️ IndexedDB 提取失败（重试 %d 次），使用 DOM 提取文本，可能不完整\n", webChatIDBReadAttempts)
			return nil
		}),
	)

	err := chromedp.Run(ctx, actions...)
	if err != nil {
		// If login is needed and we haven't retried yet, perform login
		// in the same Chrome session and retry once.
		if errors.Is(err, ErrLoginRequired) && retry == 0 {
			fmt.Fprintln(os.Stderr, "🔐 未登录，在浏览器窗口中完成登录...")
			if loginErr := deepseekLogin(ctx, "", nil, true); loginErr != nil {
				return "", "", "", 0, fmt.Errorf("webchat login: %w", loginErr)
			}
			return webchatSend(ctx, conversationURL, message, opts, retry+1)
		}
		return "", "", "", 0, fmt.Errorf("webchat: %w", err)
	}

	// Session info (keep:<id>) is surfaced by the caller (CLI / ask_expert),
	// not here, so the library layer stays silent about presentation.
	return response, reasoning, finalURL, tokens, nil
}

// waitForChatTextarea polls until the chat composer textarea is visible,
// then returns nil. It replaces the blind Sleep(3s) after navigation for
// new conversations: a cold Chrome boot plus React hydration can take
// longer than 3s, and the later set-value step then failed with a
// misleading "login required" error even though the user WAS logged in.
//
// chromedp.WaitReady cannot be used for this: without a deadline it blocks
// until the tab context is cancelled, and on the sign-in page (no textarea
// ever appears) that would mask the ErrLoginRequired signal that drives
// the login recovery. The bounded poll classifies the page instead:
// sign-in fails fast into the login flow, a slow chat page waits up to
// webChatTextareaWait.
//
// The login signal is debounced: a SPA can briefly render the sign-in
// shell before auth state resolves, so a single "login" sample — which
// would misroute a logged-in session into the login flow — is not enough;
// the marker must persist for two consecutive polls. A 60s timeout that
// still lands on ErrLoginRequired is a last resort for a truly broken
// page: deepseekLogin detects the session is already authenticated and
// returns immediately, and the subsequent retry re-navigates warm.
func waitForChatTextarea(ctx context.Context) error {
	deadline := time.Now().Add(webChatTextareaWait)
	loginStreak := 0
	for {
		var state string
		if err := chromedp.Evaluate(jsChatReadyState, &state).Do(ctx); err == nil {
			switch state {
			case "ok":
				return nil
			case "login":
				loginStreak++
				if loginStreak >= 2 {
					return fmt.Errorf("no visible textarea: %w", ErrLoginRequired)
				}
			default: // "waiting": hydration in progress, reset the streak
				loginStreak = 0
			}
		}
		// Evaluation errors (transient CDP hiccups) are tolerated: keep
		// polling, timeout is the only hard exit.
		if time.Now().After(deadline) {
			return fmt.Errorf("no visible textarea: %w", ErrLoginRequired)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// webchatSetValue sets the chat textarea value via JS (triggers React onChange).
func webchatSetValue(ctx context.Context, message string) error {
	span, ctx := clog.StartSpanFromContext(ctx, "webchatSetValue")
	defer span.Finish()

	quoted := quoteJS(message)
	var result map[string]any
	js := fmt.Sprintf(jsSetTextareaFmt, quoted)

	if err := chromedp.Evaluate(js, &result).Do(ctx); err != nil {
		return fmt.Errorf("set value: %w", err)
	}
	if errMsg, ok := result["error"].(string); ok {
		return fmt.Errorf("%s: %w", errMsg, ErrLoginRequired)
	}
	return nil
}

// WebUploadMaxFiles is the maximum number of files chat.deepseek.com accepts
// in one upload batch. Exported so callers that assemble attachments
// themselves (code_review) can pre-drop inputs under the same budget instead
// of failing in validateWebAttachments.
const WebUploadMaxFiles = 50

// WebUploadMaxTotal is the maximum total byte size chat.deepseek.com accepts
// in one upload batch (100MB).
const WebUploadMaxTotal = 100 << 20

// webUploadReadyWaitAttempts bounds the best-effort wait for the page to
// confirm uploaded attachments (one probe per second) before the send goes
// ahead with a warning - an upload the page never acknowledges must not
// silently drop the caller's attachments.
const webUploadReadyWaitAttempts = 10

// validateWebAttachments checks the web chat upload limits: at most 50 files
// and 100MB total, measured on the files as given (a later rename never
// changes the budget). File types are not restricted here; the page accepts
// images, text and PDF files, and names it would reject are normalized by
// prepareUploadAttachments.
func validateWebAttachments(files []string) error {
	if len(files) == 0 {
		return nil
	}
	if len(files) > WebUploadMaxFiles {
		return fmt.Errorf("too many attachments: %d (max %d)", len(files), WebUploadMaxFiles)
	}
	var total int64
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return fmt.Errorf("attachment %s: %w", f, err)
		}
		total += info.Size()
	}
	if total > WebUploadMaxTotal {
		return fmt.Errorf("attachments too large: %d bytes (max %d)", total, WebUploadMaxTotal)
	}
	return nil
}

// resolveWebAttachments converts attachment paths to absolute paths.
// Uploads are executed inside the Chrome process via CDP (DOM.setFileInputFiles),
// which resolves paths against Chrome's working directory — not dscli's — so a
// relative path that exists for the user would fail to upload. Absolutizing at
// the entry point makes every path unambiguous for both validation and upload.
func resolveWebAttachments(files []string) ([]string, error) {
	if len(files) == 0 {
		return files, nil // keep nil/empty as-is
	}
	resolved := make([]string, len(files))
	for i, f := range files {
		abs, err := filepath.Abs(f)
		if err != nil {
			return nil, fmt.Errorf("attachment %s: %w", f, err)
		}
		resolved[i] = abs
	}
	return resolved, nil
}

// IsImageFile reports whether path has an image extension accepted by the
// DeepSeek web upload (which recognizes text embedded in images).
func IsImageFile(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp":
		return true
	}
	return false
}

// webchatUpload attaches files to the chat: it validates the batch against the
// site limits, normalizes the names the site would reject (see
// prepareUploadAttachments), and hands the result to webchatAttachFiles.
func webchatUpload(ctx context.Context, files []string) error {
	if err := validateWebAttachments(files); err != nil {
		return err
	}
	// The site decides acceptance by extension: normalize the names (and
	// de-duplicate them) before anything is handed to Chrome. Limits were
	// validated on the original files above, so the count and byte budget
	// are unchanged by a rename.
	prepared, err := prepareUploadAttachments(files)
	if err != nil {
		return err
	}
	if prepared.cleanup != nil {
		defer prepared.cleanup()
	}
	for _, note := range prepared.notes {
		fmt.Fprintln(os.Stderr, note)
	}
	return webchatAttachFiles(ctx, prepared.files)
}

// webchatAttachFiles attaches an already-normalized batch to the chat via the
// hidden file input. The direct path sets files on the input node with CDP
// DOM.setFileInputFiles, which fires React's change handler without opening a
// native dialog. If the input is missing, the upload button is clicked with
// file-chooser interception enabled and the opened chooser is completed
// programmatically.
//
// Split from webchatUpload so the gated live upload-name probe can send RAW
// names: normalization would rewrite exactly the names the probe measures.
func webchatAttachFiles(ctx context.Context, files []string) error {
	var probe map[string]any
	if err := chromedp.Evaluate(jsFindFileInput, &probe).Do(ctx); err != nil {
		return fmt.Errorf("locate file input: %w", err)
	}
	if found, _ := probe["found"].(bool); found {
		return webchatSetUploadFiles(ctx, files)
	}

	// The input is created on demand. Intercept the chooser so no native
	// dialog appears in the visible browser, click the upload button, and
	// complete the chooser from the opened event.
	chooserCh := make(chan *page.EventFileChooserOpened, 1)
	chromedp.ListenTarget(ctx, func(ev any) {
		if e, ok := ev.(*page.EventFileChooserOpened); ok {
			select {
			case chooserCh <- e:
			default:
			}
		}
	})
	if err := page.SetInterceptFileChooserDialog(true).Do(ctx); err != nil {
		return fmt.Errorf("enable file chooser interception: %w", err)
	}
	var clickResult map[string]any
	if err := chromedp.Evaluate(jsClickUploadBtnFmt, &clickResult).Do(ctx); err != nil {
		return fmt.Errorf("click upload button: %w", err)
	}
	if ok, _ := clickResult["success"].(bool); !ok {
		msg, _ := clickResult["error"].(string)
		return fmt.Errorf("click upload button: %s", msg)
	}
	matched, _ := clickResult["matched"].(string)
	fmt.Fprintf(os.Stderr, "📎 点击上传按钮 (%s)\n", matched)
	select {
	case ev := <-chooserCh:
		return dom.SetFileInputFiles(files).WithBackendNodeID(ev.BackendNodeID).Do(ctx)
	case <-time.After(3 * time.Second):
		// No chooser event: the click may have rendered the input without
		// opening a dialog. Retry the direct path.
		var probe map[string]any
		if err := chromedp.Evaluate(jsFindFileInput, &probe).Do(ctx); err != nil {
			return fmt.Errorf("locate file input after click: %w", err)
		}
		if found, _ := probe["found"].(bool); found {
			return webchatSetUploadFiles(ctx, files)
		}
		return fmt.Errorf("upload button did not reveal a file input")
	}
}

// webchatSetUploadFiles sets the files on the file input node via CDP, then
// waits (best-effort) for the page to confirm the attachments (titled card
// text or thumbnail) before returning, so the send that follows does not race
// a still-uploading batch. Confirmation is best-effort: after
// webUploadReadyWaitAttempts the send proceeds with a warning - an upload the
// page never acknowledges must not silently drop the caller's attachments.
func webchatSetUploadFiles(ctx context.Context, files []string) error {
	quoted := make([]string, len(files))
	for i, f := range files {
		quoted[i] = quoteJS(filepath.Base(f))
	}
	js := fmt.Sprintf(jsUploadReadyCountFmt, "["+strings.Join(quoted, ", ")+"]")

	if err := chromedp.SetUploadFiles("input[type='file']", files, chromedp.ByQuery).Do(ctx); err != nil {
		return fmt.Errorf("set upload files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "📎 已添加 %d 个附件\n", len(files))
	for range webUploadReadyWaitAttempts {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		var countResult map[string]any
		if err := chromedp.Evaluate(js, &countResult).Do(ctx); err != nil {
			continue
		}
		if n, _ := countResult["count"].(float64); int(n) >= len(files) {
			fmt.Fprintf(os.Stderr, "📎 %d 个附件已就绪\n", int(n))
			return nil
		}
	}
	fmt.Fprintln(os.Stderr, "⚠️ 附件就绪未确认，继续发送")
	return nil
}

// webchatWait polls until the assistant response stabilizes, then extracts
// it from the rendered DOM: the .ds-assistant-message-main-content elements
// (primary, converted back to markdown), then the bubble-scoped answer
// block, then body-text after the sent message (last resort).
//
// The original markdown + deep-think reasoning come from the site's
// IndexedDB cache AFTER a reload (see webchatExtractReloadedIDB, called by
// webchatSend once this wait returns): IndexedDB is written once per page
// load, so polling it during a live send can only see the pre-send snapshot.
//
// mdBaseline is the concatenated .ds-markdown text captured before sending;
// it is stripped from DOM-extracted text so continued conversations return
// only the new response. sentMessage is the user's message used to anchor
// the body-text fallback (see extractAfterMessage).
//
// To avoid premature extraction during streaming pauses (>6s), it uses a
// generation-active check: when stability is first detected, it checks
// whether DeepSeek is still generating (via DOM signals like the stop button).
// Only extracts when generation appears complete or the extended poll window
// expires (escape hatch after webChatExtendedPolls additional polls).
//
// Failed sends are retried automatically before the wait gives up:
//
//   - Retry button: when the server rejects a message, the site renders a
//     "重发/重试" button next to the failed bubble; webchatWait clicks it
//     and restarts the round (up to webChatMaxResends attempts).
//   - Stale textarea: if the text is still sitting in the textarea when the
//     send-ack window (webChatConfirmPolls) expires and no retry button is
//     visible, the Enter dispatch itself was ignored — re-dispatch it once
//     per round instead of failing immediately.
//
// Server overload is handled explicitly:
//
//   - Send-ack window: within webChatConfirmPolls the send must be
//     acknowledged (textarea cleared, generation active, or new content).
//     Otherwise the submit was rejected → ErrSendRejected (fail fast
//     instead of polling for the full budget).
//   - Busy text: an extracted response that is short and matches known
//     overload phrases ("服务器忙，请稍后再试", "try again later", ...) is
//     returned as ErrServerBusy, never as an answer.
//   - Empty stability: a page that is stable with no answer content for
//     webChatEmptyStablePolls polls means the request stalled server-side
//     → ErrServerBusy (fail fast instead of the full poll-budget timeout).
//   - Truncated output: a stable, completed generation whose text shows
//     structural signs of being cut off (unclosed code fence, unterminated
//     JSON) is returned as ErrTruncated, never as a silently incomplete
//     answer. Retryable like ErrServerBusy.
//   - Continue button: a server-side stop leaves the message INCOMPLETE with
//     a 「继续生成」 button in its action bar. The wait clicks it
//     automatically (up to webChatMaxContinues times) and keeps polling
//     until the resume is confirmed, so the half-written answer is never
//     extracted as the round's final reply. The click MUST be a real CDP
//     mouse event: that button validates isTrusted and a synthetic click is
//     silently ignored (see clickTrustedAt). A resume that does not happen
//     within the budget is ErrTruncated.
//
// The poll budget comes from webChatPollBudget: when the context carries a
// deadline (the tool framework derives it from the ask_expert timeout
// argument), we poll until that deadline instead of the hardcoded
// webChatMaxPolls — so a caller-passed timeout (e.g. 1200s) genuinely extends
// the wait for long generations (full 26-question papers can exceed 600s).
func webchatWait(ctx context.Context, baseline, mdBaseline, sentMessage string) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "webchatWait")
	defer span.Finish()
	var lastText string
	stableCount := 0
	emptyStableCount := 0
	sendAck := false
	ackPolls := 0
	polls := 0
	maxPolls := webChatPollBudget(ctx)
	// resendCount is a SHARED budget across both recovery mechanisms
	// (retry-button clicks and stale-textarea Enter re-dispatches): a
	// mixed sequence exhausts webChatMaxResends like a homogeneous one,
	// so a round can never auto-resend more than the limit in total.
	resendCount := 0
	lastResendAt := time.Time{}

	// Continue-generation recovery state (jsContinueGeneration). A server
	// stop is a distinct event from a rejected send, so it carries its own
	// budget/cooldown pair rather than sharing resendCount.
	var cont continueRecovery

	for i := 0; i < maxPolls; i++ {
		polls++
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(webChatPollInterval):
		}

		var current string
		if err := chromedp.Evaluate(
			"document.body ? document.body.innerText : ''", &current,
		).Do(ctx); err != nil {
			continue // tolerate transient errors
		}

		// Failed-send retry: a rejected message renders a "重发" button
		// next to the failed bubble; clicking it restarts the round state.
		// See resendStep for the full rationale.
		if handled, err := resendStep(ctx, resendSeams{}, resendInput{
			resendCount:      &resendCount,
			lastResendAt:     &lastResendAt,
			ackPolls:         &ackPolls,
			stableCount:      &stableCount,
			emptyStableCount: &emptyStableCount,
			lastText:         &lastText,
			cont:             &cont,
		}); err != nil {
			return "", err
		} else if handled {
			sendAck = false
			continue
		}

		// Send-ack window: the message must show evidence of submission
		// within the confirmation window. While unconfirmed we skip the
		// normal stability logic so a rejected submit fails fast instead of
		// being misread as an empty stable page (60s) or timing out.
		if !sendAck {
			action, err := sendAckStep(ctx, sendAckSeams{}, current, baseline, &ackPolls, &resendCount, &lastResendAt)
			// On error the action is webChatAbort and meaningless: abort the
			// round without consulting it.
			if err != nil {
				return "", err
			}
			eff, err := ackLoopEffectFor(action, current)
			if err != nil {
				return "", err
			}
			if eff.acked {
				// Submission acknowledged; fall through to the recovery and
				// stability stages below with sendAck latched true.
				sendAck = true
			} else if eff.nextPoll {
				if eff.refreshLastText {
					lastText = eff.text
				}
				continue
			}
		}

		// Continue-generation recovery. A busy server can stop a generation
		// mid-flight: the message turns INCOMPLETE and the site renders a
		// 「继续生成」 button in the message's action bar. The page is then
		// QUIET and the half-written text can look perfectly stable, so
		// without this block the truncated fragment is extracted as the
		// round's final answer and the model's remaining work is lost.
		//
		// Invariant: while the button is visible, or a dispatched click has
		// not yet been confirmed as resumed, extraction is forbidden - the
		// "stable" text at that moment is the pre-interruption fragment.

		// The recovery runs the design's four ordered steps (resume gate →
		// detect → click → hold) as one call; see continueRecovery.step.
		// answer lazily reads the round's assistant content: it is the
		// preferred resume-evidence baseline (the whole-page body churns on
		// the click's own UI updates), and it is only read when a click is
		// recorded or a resume is pending.
		act, err := cont.step(ctx, current, func() string {
			return cleanBodyResponse(lastAnswerText(ctx, sentMessage))
		})
		if err != nil {
			return "", err
		}
		if act == continueHold {
			// Reset the stability counters too: an interrupted generation
			// must not let pre-interruption stability carry into an
			// extraction right after the hold ends.
			stableCount, emptyStableCount = 0, 0
			lastText = current
			continue
		}
		if act == continueClicked {
			// The resume restarts the answer, so stale stability counters
			// must not let the pre-interruption text win an extraction.
			stableCount, emptyStableCount = 0, 0
			lastText = ""
			continue
		}

		done, resp, err := stabilityStep(ctx, stabilityInput{
			current:          current,
			lastText:         &lastText,
			stableCount:      &stableCount,
			emptyStableCount: &emptyStableCount,
			mdBaseline:       mdBaseline,
			sentMessage:      sentMessage,
		})
		if err != nil {
			return "", err
		}
		if done {
			return resp, nil
		}
	}

	return "", fmt.Errorf("response timeout after %d polls (%.0fs)", maxPolls, float64(maxPolls)*webChatPollInterval.Seconds())
}

// continueRecovery tracks the auto-continue recovery across polls of a
// single webchatWait call: how many clicks were spent, when the last one was
// dispatched, and whether the resume it asked for has been confirmed yet.
//
// It is deliberately per-call rather than shared state: a new wait (or a
// resend, which restarts the round) gets a fresh budget.
//
// The now/active/detect/clickAt fields are injectable seams: nil means the
// production implementation. Tests substitute deterministic stand-ins to
// exercise the state machine without a browser.
type continueRecovery struct {
	clicks   int       // clicks dispatched so far (budget: webChatMaxContinues)
	lastAt   time.Time // dispatch time of the last click (cooldown anchor)
	pending  bool      // a click was dispatched, the resume is unconfirmed
	failures int       // CONSECUTIVE dispatch failures (see webChatMaxContinueClickFailures)
	warned   bool      // a detect error was already logged once

	// base is the resume-evidence baseline captured at click time.
	// baseFromAnswer records whether it came from the round's assistant
	// content (preferred) rather than the whole-page body text. bodyBase is
	// always recorded, so a later unreadable answer (evaluation failure, lost
	// anchor) can fall back to the body comparison instead of stalling the
	// resume window.
	base           string
	baseFromAnswer bool
	bodyBase       string
	deadline       time.Time // deadline for the resume to prove itself

	now     func() time.Time
	active  func(context.Context) bool
	detect  func(context.Context) (continueDetect, error)
	clickAt func(context.Context, float64, float64) error
}

// continueDetect is the detector's three-way outcome. Absent and blocked are
// distinct on purpose: a blocked button must suppress extraction exactly like
// a clickable one, otherwise the interrupted fragment gets extracted.
type continueDetect struct {
	label     string
	x, y      float64
	present   bool // the button exists in the message row
	clickable bool // present AND unoccluded, so a coordinate click lands
}

// continueAction is the outcome of one recovery step, telling webchatWait how
// to treat the poll's body text.
type continueAction int

const (
	// continueNone: no interrupted generation in sight; the normal
	// stability/extraction logic may run.
	continueNone continueAction = iota
	// continueHold: an interruption is present or a requested resume is
	// still unconfirmed; keep polling and do NOT extract (the body text is
	// the pre-interruption fragment).
	continueHold
	// continueClicked: a click was just dispatched; keep polling and also
	// discard the stability counters, since the resume restarts the answer.
	continueClicked
)

func (r *continueRecovery) nowFn() time.Time {
	if r.now != nil {
		return r.now()
	}
	return time.Now()
}

func (r *continueRecovery) activeFn(ctx context.Context) bool {
	if r.active != nil {
		return r.active(ctx)
	}
	return isGenerationActive(ctx)
}

func (r *continueRecovery) detectFn(ctx context.Context) (continueDetect, error) {
	if r.detect != nil {
		return r.detect(ctx)
	}
	return continueGenerationButton(ctx)
}

func (r *continueRecovery) clickFn(ctx context.Context, x, y float64) error {
	if r.clickAt != nil {
		return r.clickAt(ctx, x, y)
	}
	return clickTrustedAt(ctx, x, y)
}

// step runs one poll's worth of auto-continue recovery, in the order the
// design mandates:
//
//  1. Resume gate: a pending click is cleared once the resume has proven
//     itself (an active generation, or a change in the same text source the
//     baseline came from). A pending resume that outlives
//     webChatContinueResumeWindow is ErrTruncated: the answer exists but the
//     site will not finish it.
//  2. Detect: runs EVERY poll, independent of the click cooldown, so a
//     present button suppresses extraction even right after a click while
//     the UI has not flipped yet. A detection ERROR is never read as "no
//     interruption" - it holds.
//  3. Click: only when the button is present AND clickable AND the cooldown
//     has elapsed.
//  4. Hold: a present button, an unconfirmed resume, or a failed detection
//     always wins over extraction.
//
// body is the whole-page text; answer lazily yields the round's assistant
// content (may be empty), and is only consulted when a click is recorded or
// a resume is pending.
func (r *continueRecovery) step(ctx context.Context, body string, answer func() string) (continueAction, error) {
	if err := r.gate(ctx, body, answer); err != nil {
		return continueNone, err
	}
	d, derr := r.detectFn(ctx)
	if derr != nil {
		if !r.warned {
			r.warned = true
			fmt.Fprintf(os.Stderr, "⚠️ 检测「继续生成」按钮失败（将继续轮询）: %v\n", derr)
		}
		// Deliberately hold-only, with NO failure cap that fails the round:
		// detection runs on every poll, including healthy ones, so escalating
		// a transient CDP hiccup on a healthy round into ErrTruncated would
		// make the caller retry - and that retry posts another message into a
		// conversation whose generation may still be running. Holding (never
		// extracting, bounded by the poll budget) is the safe trade.
		return continueHold, nil
	}
	if d.present && d.clickable && r.ready() {
		clicked, err := r.click(ctx, d, body, answer)
		if err != nil {
			return continueNone, err
		}
		if clicked {
			return continueClicked, nil
		}
	}
	if d.present || r.pending {
		return continueHold, nil
	}
	return continueNone, nil
}

// gate implements step 1: it clears the pending flag once the resume has
// proven itself, and fails the wait once the resume window has expired.
func (r *continueRecovery) gate(ctx context.Context, body string, answer func() string) error {
	if !r.pending {
		return nil
	}
	if r.resumed(ctx, body, answer) {
		r.pending = false
		return nil
	}
	if r.nowFn().After(r.deadline) {
		return fmt.Errorf("%w: 已点击「继续生成」%d 次，生成仍未恢复（服务器中断）", ErrTruncated, r.clicks)
	}
	return nil
}

// resumed reports whether the clicked resume has visibly taken hold: an
// active generation, or a change in the SAME text source the baseline came
// from. Comparing the whole-page body while an assistant-content baseline
// exists would misfire on the click's own UI churn (toast line, button
// state), clearing pending before the resume actually started.
func (r *continueRecovery) resumed(ctx context.Context, body string, answer func() string) bool {
	if r.activeFn(ctx) {
		return true
	}
	if r.baseFromAnswer {
		// Answer unreadable right now (evaluation failure, anchor lost):
		// fall back to the body baseline rather than stalling pending until
		// the deadline. The bodyBase != "" guard makes the fallback
		// independent of constructor ordering (a struct literal that forgot
		// to set bodyBase must not compare against the empty string and
		// report a spurious resume).
		//
		// Trade-off: this fallback can clear pending on an unrelated body
		// change (the click's own UI churn) slightly early. That is accepted:
		// the alternative is stalling to the resume-window timeout and
		// failing a round that actually resumed, and the very next poll's
		// detection re-asserts the hold if the button is still there.
		if a := answer(); a != "" {
			return a != r.base
		}
		return r.bodyBase != "" && body != r.bodyBase
	}
	return body != r.base
}

// ready reports whether a freshly detected button may be clicked now, i.e.
// the cooldown since the previous click has elapsed. Detection itself runs
// every poll regardless; this only throttles the click rate.
func (r *continueRecovery) ready() bool {
	return r.nowFn().Sub(r.lastAt) >= webChatContinueCooldown
}

// click implements step 3: it dispatches the real CDP mouse click and arms
// the resume window. clicked=false with a nil error means the dispatch failed
// and the caller should keep polling - the failure does not consume the click
// budget (a transient CDP hiccup must not burn one of the three attempts),
// but the cooldown still advances so a broken page is not hammered. An
// exhausted click budget, or webChatMaxContinueClickFailures consecutive
// dispatch failures, is ErrTruncated.
func (r *continueRecovery) click(ctx context.Context, d continueDetect, body string, answer func() string) (clicked bool, err error) {
	if r.clicks >= webChatMaxContinues {
		return false, fmt.Errorf("%w: 自动点击「继续生成」%d 次仍未完成（服务器中断）", ErrTruncated, r.clicks)
	}
	if cerr := r.clickFn(ctx, d.x, d.y); cerr != nil {
		r.failures++
		r.lastAt = r.nowFn()
		fmt.Fprintf(os.Stderr, "⚠️ 点击「继续生成」失败（连续第 %d 次）: %v\n", r.failures, cerr)
		if r.failures >= webChatMaxContinueClickFailures {
			// Wrap BOTH the retryable sentinel and the last CDP error so
			// errors.Is can reach either one.
			return false, fmt.Errorf("%w: 点击「继续生成」连续失败 %d 次: %w", ErrTruncated, r.failures, cerr)
		}
		return false, nil
	}
	r.failures = 0
	r.warned = false
	r.clicks++
	r.lastAt = r.nowFn()
	r.pending = true
	// Prefer the round's assistant content as the baseline; fall back to the
	// whole-page body only when it cannot be read.
	r.bodyBase = body
	if a := answer(); a != "" {
		r.base, r.baseFromAnswer = a, true
	} else {
		r.base, r.baseFromAnswer = body, false
	}
	r.deadline = r.nowFn().Add(webChatContinueResumeWindow)
	fmt.Fprintf(os.Stderr, "🔄 检测到生成中断（%s），已点击「继续生成」继续（%d/%d）...\n", d.label, r.clicks, webChatMaxContinues)
	return true, nil
}

// webChatAction is a narrow control signal from a webchatWait sub-step back
// to the poll loop.
type webChatAction int

const (
	// webChatProceed: this stage is satisfied; the caller moves on (for the
	// ack stage that means sendAck is now true).
	webChatProceed webChatAction = iota
	// webChatNextPoll: start the next poll WITHOUT touching lastText. Used by
	// the stale-textarea re-dispatch, whose original inline code did a bare
	// `continue` and deliberately left lastText alone.
	webChatNextPoll
	// webChatAckPending: the send is still unconfirmed and nothing was
	// re-dispatched; start the next poll AND refresh lastText with the
	// current body text (the original sub-threshold path did exactly that).
	webChatAckPending
	// webChatAbort: the stage failed. It is always paired with a non-nil
	// error, which is the authoritative signal - the caller must check err
	// first and ignore the action, so webChatAbort carries no extra meaning
	// beyond "this call failed".
	webChatAbort
)

// sendAckSeams holds the injectable dependencies of sendAckStep. A nil field
// means the production implementation; tests substitute deterministic
// stand-ins to exercise the window without a browser.
type sendAckSeams struct {
	active  func(context.Context) bool
	cleared func(context.Context) bool
	resend  func(context.Context) error
	now     func() time.Time
}

func (s sendAckSeams) isActive(ctx context.Context) bool {
	if s.active != nil {
		return s.active(ctx)
	}
	return isGenerationActive(ctx)
}

func (s sendAckSeams) isCleared(ctx context.Context) bool {
	if s.cleared != nil {
		return s.cleared(ctx)
	}
	return textareaCleared(ctx)
}

func (s sendAckSeams) redispatch(ctx context.Context) error {
	if s.resend != nil {
		return s.resend(ctx)
	}
	return resendStaleTextarea(ctx)
}

func (s sendAckSeams) nowFn() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// sendAckStep runs one poll of the send-ack confirmation window, the stage
// that decides whether the submit was accepted. The returned action is the
// single source of truth for the caller's next move; there is deliberately no
// separate acked flag to disagree with it.
//
// Contract:
//   - action=webChatProceed, err=nil: evidence of submission was seen (new
//     body content, an active generation, or a cleared textarea).
//   - action=webChatAckPending, err=nil: still unconfirmed and nothing was
//     re-dispatched. The caller must start the next poll AND set lastText to
//     the current body text.
//   - action=webChatNextPoll, err=nil: a stale-textarea re-dispatch just
//     happened. The caller must start the next poll WITHOUT touching lastText.
//   - err != nil: the round must fail. The action is then always
//     webChatAbort and carries no further meaning; the caller checks err
//     first and aborts the round.
//
// It mutates the shared round state through pointers: ackPolls resets on a
// re-dispatch, resendCount advances the shared resend budget, and
// lastResendAt arms the resend cooldown.
func sendAckStep(
	ctx context.Context,
	seams sendAckSeams,
	current, baseline string,
	ackPolls, resendCount *int,
	lastResendAt *time.Time,
) (action webChatAction, err error) {
	if current != baseline || seams.isActive(ctx) || seams.isCleared(ctx) {
		return webChatProceed, nil
	}
	*ackPolls += 1
	// No retry button and the text still sits in the textarea: the Enter
	// dispatch was ignored entirely. Re-dispatch once and keep the
	// confirmation window open - failing immediately would skip the
	// automatic recovery the caller expects.
	if *ackPolls >= webChatConfirmPolls {
		if rerr := seams.redispatch(ctx); rerr != nil {
			// Wrap the underlying dispatch error alongside the sentinel, so
			// errors.Is reaches both (matching the click-failure path).
			return webChatAbort, fmt.Errorf("%w: 消息重发失败: %w", ErrSendRejected, rerr)
		}
		*resendCount += 1
		if *resendCount > webChatMaxResends {
			return webChatAbort, fmt.Errorf("%w: 自动重发 %d 次仍失败", ErrSendRejected, *resendCount-1)
		}
		fmt.Fprintf(os.Stderr, "🔄 消息未被接受，重新发送（%d/%d）...\n", *resendCount, webChatMaxResends)
		*ackPolls = 0
		*lastResendAt = seams.nowFn()
		return webChatNextPoll, nil
	}
	return webChatAckPending, nil
}

// ackLoopEffect is the poll-loop state change an ack action calls for: either
// latch the send as acknowledged, or move to the next poll with an optional
// lastText refresh.
type ackLoopEffect struct {
	acked bool
	// nextPoll starts the next poll; refreshLastText decides whether lastText
	// is first set to text (the original inline code refreshed it on the
	// sub-threshold path and deliberately did not on the re-dispatch path).
	nextPoll        bool
	refreshLastText bool
	text            string
}

// ackLoopEffectFor maps a send-ack action onto the poll loop's state change.
// It is a pure function so the mapping (including the exhaustion guard) is
// testable without a browser. webChatAbort means the companion error carries
// the failure, so reaching here with it is a caller bug.
func ackLoopEffectFor(action webChatAction, current string) (ackLoopEffect, error) {
	switch action {
	case webChatProceed:
		return ackLoopEffect{acked: true}, nil
	case webChatNextPoll:
		return ackLoopEffect{nextPoll: true}, nil
	case webChatAckPending:
		return ackLoopEffect{nextPoll: true, refreshLastText: true, text: current}, nil
	case webChatAbort:
		// Defence in depth: webChatAbort is only meaningful alongside a
		// non-nil error, and the production caller checks err first. This
		// branch exists for a future caller that ignores err, so the mistake
		// surfaces as an error here instead of a silent fall-through into the
		// recovery/stability stages.
		return ackLoopEffect{}, fmt.Errorf("send-ack step returned webChatAbort without an error")
	default:
		// Exhaustiveness guard: a future action added to the enum must be
		// handled here rather than silently falling through into the
		// recovery/stability stages (which would extract a fragment).
		return ackLoopEffect{}, fmt.Errorf("send-ack step returned unknown action %d", int(action))
	}
}

// resendSeams holds the injectable dependencies of resendStep. A nil field
// means the production implementation.
type resendSeams struct {
	detect func(context.Context) (string, bool)
	now    func() time.Time
}

func (s resendSeams) failed(ctx context.Context) (string, bool) {
	if s.detect != nil {
		return s.detect(ctx)
	}
	return resendFailedMessage(ctx)
}

func (s resendSeams) nowFn() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

// resendInput is the round state resendStep reads and mutates. Everything is
// a pointer so the helper has no hidden coupling to webchatWait's locals.
type resendInput struct {
	resendCount      *int
	lastResendAt     *time.Time
	ackPolls         *int
	stableCount      *int
	emptyStableCount *int
	lastText         *string
	cont             *continueRecovery
}

// resendStep runs one poll of the failed-send retry stage: a rejected message
// renders a "重发" button next to the failed bubble. A rejected send ALSO
// changes the body text (the bubble appears), so the send-ack stage alone
// cannot distinguish it from success - the retry button is the reliable
// signal. Clicking it restarts the round state, since the retry is a fresh
// send; the cooldown keeps a slow UI from being clicked twice in a row.
//
// It returns handled=true when a resend happened and the caller must start the
// next poll, and a non-nil error when the resend budget is exhausted.
func resendStep(ctx context.Context, seams resendSeams, in resendInput) (handled bool, err error) {
	if seams.nowFn().Sub(*in.lastResendAt) < webChatResendCooldown {
		return false, nil
	}
	matched, clicked := seams.failed(ctx)
	if !clicked {
		return false, nil
	}
	*in.resendCount += 1
	if *in.resendCount > webChatMaxResends {
		return false, fmt.Errorf("%w: 自动重发 %d 次仍失败（%s）", ErrServerBusy, *in.resendCount-1, matched)
	}
	*in.lastResendAt = seams.nowFn()
	fmt.Fprintf(os.Stderr, "🔄 检测到发送失败（%s），自动重发（%d/%d）...\n", matched, *in.resendCount, webChatMaxResends)
	*in.ackPolls = 0
	*in.stableCount = 0
	*in.emptyStableCount = 0
	*in.lastText = ""
	// A resend starts a fresh send, so any in-flight continue recovery
	// belongs to the abandoned round: dropping it prevents a stale
	// pending/deadline from failing the new one.
	*in.cont = continueRecovery{}
	return true, nil
}

// stabilityInput carries the per-round state the stability/extraction stage
// reads and mutates. Everything is a pointer or a value copy so the helper
// has no hidden coupling to webchatWait's locals.
type stabilityInput struct {
	current          string
	lastText         *string
	stableCount      *int
	emptyStableCount *int
	mdBaseline       string
	sentMessage      string
}

// stabilityStep runs one poll of the stability/extraction stage, the block
// that decides whether the round is finished. It returns done=true with the
// extracted answer (or the terminal error) when the round ends, and
// done=false when the caller should keep polling.
//
// The body is a mechanical extraction of webchatWait's original inline block:
// every `continue` became `return false, "", nil`, every `return` became
// `return true, ...`, and the trailing lastText update keeps its original
// position (after the inner block, so only the sub-threshold path reaches it).
func stabilityStep(ctx context.Context, in stabilityInput) (done bool, resp string, err error) {
	if in.current == *in.lastText && *in.lastText != "" {
		*in.stableCount += 1

		if *in.stableCount >= webChatStablePolls {
			// Gated extraction: only return when generation appears complete
			// or the escape hatch fires.
			canExtract := !isGenerationActive(ctx) ||
				*in.stableCount >= webChatStablePolls+webChatExtendedPolls

			// Fallback: extract from .ds-markdown elements. This naturally
			// excludes UI chrome (search info, toggle labels, footer text).
			// The rendered DOM is converted back to markdown (code fences,
			// inline-code backticks, list markers) - innerText alone would
			// lose all of that structure. NOTE: the current DeepSeek UI
			// dropped .ds-markdown entirely (hashed classes), so this path
			// is a compatibility net for other layouts.
			if resp := getAssistantText(ctx); resp != "" {
				resp = stripBaselinePrefix(resp, in.mdBaseline)
				// Belt-and-braces: strip body-text fallback artifacts
				// (code-block toolbar labels) in case the markdown converter
				// was bypassed. No-op for clean output.
				resp = stripUIChromePrefix(resp)
				// A short overload notice must never be returned as an
				// answer - it would poison the caller's decision-making.
				if isBusyErrorText(resp) {
					return true, "", fmt.Errorf("%w: %s", ErrServerBusy, resp)
				}
				if canExtract {
					// A response cut off mid-generation (unclosed code
					// fence, unterminated JSON) is a distinct, retryable
					// failure: the answer exists but is unusable, and
					// polling further will not complete it.
					if isTruncated(resp) {
						return true, "", fmt.Errorf("%w (%d chars)", ErrTruncated, utf8.RuneCountInString(resp))
					}
					if isCompleteResponse(resp) {
						return true, resp, nil
					}
				}
				// Fragment or generation still active: keep polling. The
				// model often pauses after emitting a simulated tool call
				// (<read_file ...>) that the web UI cannot execute;
				// returning the fragment would lose the rest of the answer.
				return false, "", nil
			}

			// Fallback 1: the answer block of this round, scoped by message
			// bubble (after the sent message) - no history, no deep-think
			// reasoning, no UI chrome.
			fallback := cleanBodyResponse(lastAnswerText(ctx, in.sentMessage))
			if fallback == "" {
				// Fallback 2: body text after the sent message (anchored on
				// the deep-think marker / the message itself), then clean up
				// known artifact patterns. Only accept clean, complete text;
				// keep polling on fragments instead of aborting on a
				// mid-response pause.
				fallback = cleanBodyResponse(extractAfterMessage(in.current, in.sentMessage))
			}
			// The body path also picks up code-block toolbar labels; strip
			// them like the .ds-markdown path does.
			fallback = stripUIChromePrefix(fallback)
			if isBusyErrorText(fallback) {
				return true, "", fmt.Errorf("%w: %s", ErrServerBusy, fallback)
			}
			if canExtract {
				if isTruncated(fallback) {
					return true, "", fmt.Errorf("%w (%d chars)", ErrTruncated, utf8.RuneCountInString(fallback))
				}
				if isCompleteResponse(fallback) {
					return true, fallback, nil
				}
			}

			// Stable page with no answer content: the request stalled
			// server-side. Count consecutive empty polls and fail fast
			// instead of waiting out the full poll budget.
			*in.emptyStableCount += 1
			if *in.emptyStableCount >= webChatEmptyStablePolls {
				return true, "", ErrServerBusy
			}
			return false, "", nil
		}
	} else {
		*in.stableCount = 0
		*in.emptyStableCount = 0
	}
	*in.lastText = in.current
	return false, "", nil
}

// webChatPollBudget returns the number of polls webchatWait may perform.
//
// When the context carries a deadline (the tool framework derives it from the
// ask_expert timeout argument), the budget is the polls remaining until that
// deadline — so a caller-passed timeout (e.g. 1200s) actually extends the
// wait instead of being capped at webChatMaxPolls. The +1 margin absorbs the
// sub-interval remainder so the loop outlives the deadline and lets the
// select's ctx.Done branch surface the framework timeout. Without a deadline,
// the default webChatMaxPolls (300 × 2s = 600s) applies as a safety net
// against runaway polling.
func webChatPollBudget(ctx context.Context) int {
	if deadline, ok := ctx.Deadline(); ok {
		n := int(time.Until(deadline)/webChatPollInterval) + 1
		if n < 1 {
			return 1
		}
		return n
	}
	return webChatMaxPolls
}

// textareaCleared reports whether the chat textarea is empty (a successful
// send clears it immediately). Evaluation failure is treated as "not
// cleared" so the send-ack window stays conservative.
func textareaCleared(ctx context.Context) bool {
	var cleared bool
	if err := chromedp.Evaluate(jsTextareaCleared, &cleared).Do(ctx); err != nil {
		return false
	}
	return cleared
}

// resendFailedMessage detects the failed-send retry button that the site
// renders next to a rejected message bubble and clicks it. The button text
// is returned for logging; clicked=false means no retry button is visible
// (nothing was wrong or the failure UI was already handled).
func resendFailedMessage(ctx context.Context) (matched string, clicked bool) {
	var result map[string]any
	if err := chromedp.Evaluate(jsResendFailedFmt, &result).Do(ctx); err != nil {
		return "", false
	}
	found, _ := result["found"].(bool)
	if !found {
		return "", false
	}
	matched, _ = result["matched"].(string)
	return matched, true
}

// continueGenerationButton detects the 「继续生成」 (Continue) button that the
// site renders on a server-interrupted assistant message, and returns its
// viewport centre so the caller can dispatch a REAL mouse click there (see
// clickTrustedAt for why a synthetic click cannot work). found=false means no
// such button is visible - the normal state of a healthy round.
func continueGenerationButton(ctx context.Context) (continueDetect, error) {
	var result map[string]any
	if err := chromedp.Evaluate(jsContinueGeneration, &result).Do(ctx); err != nil {
		return continueDetect{}, err
	}
	if present, _ := result["found"].(bool); !present {
		return continueDetect{}, nil
	}
	d := continueDetect{present: true}
	d.label, _ = result["label"].(string)
	d.clickable, _ = result["clickable"].(bool)
	// Guarded type assertions: a malformed/partial result must not silently
	// become the zero coordinates, which would dispatch a click at (0,0).
	x, okX := result["x"].(float64)
	y, okY := result["y"].(float64)
	if !okX || !okY {
		return continueDetect{}, fmt.Errorf("continue button: non-numeric coordinates in detector result")
	}
	d.x, d.y = x, y
	return d, nil
}

// clickTrustedAt dispatches a real mouse click at the given viewport
// coordinates through the browser's own input pipeline.
//
// WHY this exists (do NOT "simplify" it back into a JS click): the
// 「继续生成」 button is the ONLY element in the whole DeepSeek bundle whose
// click handler validates isTrusted (verified 2026-09-13 against
// main.d69e3d8c16.js, page commit-id 5d128f98):
//
//	onClick: e => { try { const t = e.nativeEvent; if (!t) return false;
//	    return t.isTrusted && t instanceof Event } catch (e) { return false } }
//	    && u({chatSessionId, messageId, allowParallelStreams})
//
// Synthetic events (el.click(), dispatchEvent(new MouseEvent(...))) carry
// isTrusted=false and are silently dropped: no error, no resume - the round
// just returns the interrupted fragment. Events injected through CDP
// Input.dispatchMouseEvent (what chromedp.MouseClickXY uses) travel the
// browser's input pipeline and carry isTrusted=true. The other auto-click
// sites in this file (jsResendFailedFmt, jsClickUploadBtnFmt) have no such
// guard and stay synthetic on purpose; this one cannot.
func clickTrustedAt(ctx context.Context, x, y float64) error {
	return chromedp.Run(
		ctx,
		chromedp.MouseEvent(input.MouseMoved, x, y),
		chromedp.MouseClickXY(x, y),
	)
}

// resendStaleTextarea re-dispatches the Enter sequence when the message is
// still sitting in the textarea after the send-ack window closed — the
// original dispatch was ignored (focus loss, React not listening). Uses
// jsSendEnterOnly so the send-button fallback cannot race the resend state
// reset with a second submission.
func resendStaleTextarea(ctx context.Context) error {
	var out map[string]any
	if err := chromedp.Evaluate(jsSendEnterOnly, &out).Do(ctx); err != nil {
		return err
	}
	if ok, _ := out["success"].(bool); !ok {
		msg, _ := out["error"].(string)
		return fmt.Errorf("stale textarea resend: %s", msg)
	}
	return nil
}

// isGenerationActive checks whether the assistant is still generating a response
// by evaluating DOM signals (stop button visibility, textarea disabled state).
// Returns false if generation appears complete or the DOM state is indeterminate.
func isGenerationActive(ctx context.Context) bool {
	span, ctx := clog.StartSpanFromContext(ctx, "isGenerationActive")
	defer span.Finish()

	var active bool
	if err := chromedp.Evaluate(jsIsGenerationActive, &active).Do(ctx); err != nil {
		return false // evaluation failure → be conservative, assume not active
	}
	return active
}

// getAssistantText returns the concatenated markdown of all
// .ds-assistant-message-main-content elements in the main content area,
// or "" if the selector doesn't match (e.g. DeepSeek changed their DOM).
// The text may include pre-existing conversation history; webchatWait
// strips it via mdBaseline.
func getAssistantText(ctx context.Context) string {
	span, ctx := clog.StartSpanFromContext(ctx, "getAssistantText")
	defer span.Finish()

	var parts []string
	if err := chromedp.Evaluate(jsGetAssistantText, &parts).Do(ctx); err != nil {
		return ""
	}
	return joinMarkdown(parts)
}

// lastAnswerText returns the innerText of THIS round's answer block:
// the last .ds-assistant-message-main-content after the bubble holding
// the sent message. Scoped extraction — no history, no deep-think
// reasoning — so continued conversations return only the new text
// without any bodyText anchoring. "" when the selector misses.
func lastAnswerText(ctx context.Context, sentMessage string) string {
	span, ctx := clog.StartSpanFromContext(ctx, "lastAnswerText")
	defer span.Finish()

	var text string
	if err := chromedp.Evaluate(fmt.Sprintf(jsLastAnswerAfterFmt, quoteJS(sentMessage)), &text).Do(ctx); err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

// idbGetAssistant reads the assistant's answer for the CURRENT
// conversation from the site's IndexedDB conversation cache. The URL's
// conversation ID keys the history-message record; the last ASSISTANT
// message's RESPONSE fragments carry the original markdown of the reply
// and its THINK fragments the deep-think reasoning. ok=false means "not
// readable yet" (record not persisted, stale pre-send snapshot, or no
// conversation ID in the URL); status distinguishes a complete reply
// ("FINISHED") from an in-flight one.
//
// sentMessage and notBeforeMS guard against stale records (a continued
// conversation's cached record may still show the PREVIOUS round as
// FINISHED): the record must contain this round's user message and be
// written after the send.
func idbGetAssistant(ctx context.Context, sentMessage string, notBeforeMS int64) (text, reasoning, status string, tokens int, ok bool) {
	js := fmt.Sprintf(jsIDBGetAnswerFmt, quoteJS(sentMessage), notBeforeMS)
	var result map[string]any
	if err := evaluateAwait(ctx, js, &result); err != nil {
		return "", "", "", 0, false
	}
	found, _ := result["found"].(bool)
	if !found {
		return "", "", "", 0, false
	}
	status, _ = result["status"].(string)
	text, _ = result["text"].(string)
	reasoning, _ = result["reason"].(string)
	// tokens is a JSON number (float64 after unmarshal); the site never
	// emits fractional counts, so a plain int64 conversion is exact.
	if t, ok := result["tokens"].(float64); ok && t > 0 {
		tokens = int(t)
	}
	return text, reasoning, status, tokens, true
}

// idbGetLastAssistant reads the LAST assistant message of the CURRENT
// conversation from the site's IndexedDB conversation cache — the
// continuation point of a resumed conversation (HandleWebChatResume). No
// sent-message anchor and no freshness window: there is no "this round", the
// caller wants whatever the expert last said. Returns the original markdown
// (RESPONSE fragments) plus the message status; ok=false when the record is
// unreadable yet or no assistant message exists.
func idbGetLastAssistant(ctx context.Context) (text, status string, ok bool) {
	var result map[string]any
	if err := evaluateAwait(ctx, jsIDBGetLastAssistantFmt, &result); err != nil {
		return "", "", false
	}
	found, _ := result["found"].(bool)
	if !found {
		return "", "", false
	}
	status, _ = result["status"].(string)
	text, _ = result["text"].(string)
	return text, status, true
}

// webchatExtractReloadedIDB reloads the conversation page and reads THIS
// round's answer from the site's IndexedDB cache — the structured source
// with the ORIGINAL markdown (RESPONSE fragments), the deep-think
// reasoning (THINK fragments) and this round's token count (the
// accumulated_token_usage difference between the round's ASSISTANT and USER
// messages, 0 when unavailable).
//
// WHY reload: the site persists its conversation record ONCE per page
// load. During a live send the record is either the pre-send snapshot (a
// continued conversation) or absent entirely (a new conversation), and it
// is never updated by polling — only a navigation triggers the write. By
// the time the caller reaches here the DOM wait has confirmed the reply is
// FINISHED, so the reloaded record carries the complete round.
//
// NOT after a mid-generation reload: the snapshot would be WIP and the
// site never refreshes it on the same page — hence the strict order
// (wait first, reload second).
//
// Returns ok=false when IDB is not readable (URL missing, login lost,
// record still absent after the grace polls); the caller then keeps the
// DOM-extracted text, an empty reasoning and a zero token count.
func webchatExtractReloadedIDB(ctx context.Context, conversationURL, sentMessage string) (content, reasoning string, tokens int, ok bool) {
	if conversationURL == "" {
		return "", "", 0, false
	}
	// Bound the reload itself: a hung navigation must degrade to the
	// DOM-extracted text, not stall the whole send. WaitReady without a
	// deadline would wait forever on a broken page.
	wctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := chromedp.Navigate(conversationURL).Do(wctx); err != nil {
		return "", "", 0, false
	}
	if err := chromedp.WaitReady("textarea", chromedp.ByQuery).Do(wctx); err != nil {
		return "", "", 0, false
	}
	// The record write trails the DOM render slightly; poll briefly for a
	// FINISHED snapshot that contains this round's user message. The text
	// must also pass the same acceptance checks as the DOM path
	// (answerUsable): a server-overload notice or a truncated reply the
	// site stored with status=FINISHED must never replace the already
	// validated DOM answer with itself.
	deadline := time.Now().Add(webChatIDBPollWindow)
	for {
		if text, think, status, tk, found := idbGetAssistant(ctx, sentMessage, time.Now().UnixMilli()); found {
			if status == "FINISHED" && answerUsable(text) {
				return text, think, tk, true
			}
		}
		if time.Now().After(deadline) {
			return "", "", 0, false
		}
		select {
		case <-ctx.Done():
			return "", "", 0, false
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// ReadLastAssistant navigates the session's browser to the conversation and
// reads the LAST assistant message's original markdown from the site's
// IndexedDB cache — the continuation point of a resumed conversation. Unlike
// Send it sends nothing: this is a read-only probe (HandleWebChatResume).
// ok=false means the record is unreadable (URL missing, login lost, no
// assistant message yet, or the record write never landed within the poll
// window).
func (s *webChatSession) ReadLastAssistant(ctx context.Context, conversationURL string) (content, status string, ok bool) {
	span, ctx := clog.StartSpanFromContext(ctx, "webChatSessionReadLast")
	defer span.Finish()
	tabCtx, err := s.ensureTab(ctx)
	if err != nil {
		return "", "", false
	}
	return webchatReadLastAssistant(tabCtx, conversationURL)
}

// webchatReadLastAssistant is the transport-level read of the last assistant
// message: navigate to the conversation, wait for the composer, then poll the
// IDB record for a FINISHED assistant message (the record write trails the
// navigation, so a single immediate read can miss it — same window as
// webchatExtractReloadedIDB). No sent-message anchoring: a resumed
// conversation has no "this round" yet, so the newest-first FIRST
// role=ASSISTANT entry IS the continuation point (see
// jsIDBGetLastAssistantFmt).
//
// The IDB poll MUST run inside a chromedp action: cdp.Execute resolves the
// message executor from the action context that chromedp.Run injects (via
// cdp.WithExecutor); after Run returns, a bare tab context carries no
// executor and every evaluateAwait call fails with ErrInvalidContext, so
// the poll is a second Run with its own deadline rather than a loop after
// the first one.
//
// The FIRST Run must take the LONG-LIVED tab context, never a derived
// deadline context: chromedp binds the target's CDP connection to the
// context of the first Run, and cancelling that context later tears the
// connection down — every later Run (the poll below and every follow-up
// Send in the tool loop) then fails with "context canceled" even though
// the tab context is still alive. Per-step timeouts live inside the actions
// (waitForChatTextarea's 60s bound), mirroring webchatSend.
func webchatReadLastAssistant(ctx context.Context, conversationURL string) (content, status string, ok bool) {
	if conversationURL == "" {
		return "", "", false
	}
	actions := []chromedp.Action{
		chromedp.Navigate(conversationURL),
		chromedp.WaitReady("body"),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return waitForChatTextarea(ctx)
		}),
		// Let React finish rendering before poking IndexedDB; mirrors
		// webchatSend's settle delay.
		chromedp.Sleep(3 * time.Second),
	}
	if err := chromedp.Run(ctx, actions...); err != nil {
		return "", "", false
	}
	// Second Run: the poll window gets its own budget so a slow composer can
	// never eat into it, and this is where the action context supplies the
	// cdp executor the IDB read needs. The deadline context here only scopes
	// the actions; the target connection stays bound to the first Run's
	// long-lived context.
	pctx, pcancel := context.WithTimeout(ctx, webChatIDBPollWindow+5*time.Second)
	defer pcancel()
	var text, st string
	var found bool
	if err := chromedp.Run(pctx, chromedp.ActionFunc(func(actCtx context.Context) error {
		deadline := time.Now().Add(webChatIDBPollWindow)
		for {
			if t, s, f := idbGetLastAssistant(actCtx); f {
				text, st, found = t, s, true
				return nil
			}
			if time.Now().After(deadline) {
				return nil
			}
			select {
			case <-actCtx.Done():
				return nil
			case <-time.After(500 * time.Millisecond):
			}
		}
	})); err != nil {
		return "", "", false
	}
	return text, st, found
}

// evaluateAwait evaluates a JS expression and awaits the resulting promise.
// chromedp.Evaluate does not await promises, but the copy-button flow is
// async by nature (hover, click, clipboard), so this helper drives
// runtime.Evaluate directly.
func evaluateAwait(ctx context.Context, expression string, out any) error {
	v, exp, err := runtime.Evaluate(expression).WithAwaitPromise(true).WithReturnByValue(true).Do(ctx)
	if err != nil {
		return err
	}
	if exp != nil {
		return fmt.Errorf("js exception: %s", exp.Text)
	}
	if v.Value == nil {
		return fmt.Errorf("js result is null")
	}
	return json.Unmarshal(v.Value, out)
}

// webchatGrantClipboard grants clipboard read/write permission to
// chat.deepseek.com so navigator.clipboard.readText() works from the
// automated session. The site's own copy handler usually writes via
// writeText — captured by the hook in jsCopyLatestAnswerFmt even when the
// native write is rejected — but the permission also covers the pages
// where the hook is bypassed.
func webchatGrantClipboard(ctx context.Context) error {
	span, ctx := clog.StartSpanFromContext(ctx, "webchatGrantClipboard")
	defer span.Finish()
	for _, name := range []string{"clipboard-read", "clipboard-write"} {
		perm := &browser.PermissionDescriptor{Name: name, AllowWithoutSanitization: true}
		if err := browser.SetPermission(perm, browser.PermissionSettingGranted).
			WithOrigin("https://chat.deepseek.com").Do(ctx); err != nil {
			return fmt.Errorf("grant %s: %w", name, err)
		}
	}
	return nil
}

// cleanBodyResponse removes DeepSeek UI chrome artifacts from
// body-text-after-message output. These artifacts appear when the
// .ds-markdown selector fails and we fall back to body.innerText.
func cleanBodyResponse(raw string) string {
	lines := strings.Split(raw, "\n")
	filtered := lines[:0]
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Standalone citation references like "- 2", "- 10".
		if matchCitationLine(trimmed) {
			continue
		}

		// DeepSeek UI labels that appear at page bottom.
		switch trimmed {
		case "深度思考", "Deep Think", "智能搜索", "联网搜索",
			"内容由 AI 生成，请仔细甄别",
			"内容由AI生成，请仔细甄别":
			continue
		}

		// "已阅读 N 个网页" / "N 个网页" — search summary line.
		if strings.HasSuffix(trimmed, "个网页") {
			continue
		}

		// The deep-think status line ("已思考（用时 1 秒）") that the new
		// UI renders right before the answer. It is UI chrome, not model
		// output — the answer follows it in body text.
		if thinkingMarkerRE.MatchString(trimmed) {
			continue
		}

		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// thinkingMarkerRE matches DeepSeek's deep-think status line rendered
// before each answer, e.g. "已思考（用时 1 秒）", "已思考（28 秒）" or
// "已思考 12s". It requires a duration marker (用时 + digits, or bare
// digits + optional 秒/s) so a real answer line that merely STARTS with
// "已思考" is never stripped. Line-anchored via (?m).
var thinkingMarkerRE = regexp.MustCompile(`(?m)^已思考[（(]?(用时\s*\d+[^）)\n]*|\s*\d+\s*(秒|s)?)?[）)]?$`)

// matchCitationLine reports whether s is a standalone citation
// reference like "- 2" or "-10" or "— 10".
var citationLineRE = regexp.MustCompile(`^[-–—]\s*\d+$`)

func matchCitationLine(s string) bool {
	return citationLineRE.MatchString(s)
}

// extractAfterMessage returns the body text that follows the sent message
// of THIS round. It finds the last deep-think marker ("已思考（用时 N
// 秒）") that sits strictly AFTER a message occurrence — that occurrence
// is the chat copy (not a quote inside the answer), and that marker is
// this round's preamble — then cuts after the marker. Without an
// applicable marker it cuts after the last occurrence of the message text.
//
// The previous baseline-prefix diff is not viable anymore: DeepSeek
// navigates from the new-chat home to the conversation page after sending,
// which rebuilds the whole body prefix (sidebar + history), so a suffix at
// len(baseline) slices garbage (or nothing). The sent message is a stable
// anchor: it appears verbatim in the chat area before the answer, and its
// last occurrence is the chat copy (the sidebar title comes earlier in
// body text).
func extractAfterMessage(current, message string) string {
	norm := strings.ReplaceAll(current, "\r\n", "\n")
	anchor := strings.TrimSpace(strings.ReplaceAll(message, "\r\n", "\n"))
	if anchor == "" {
		return ""
	}

	// All occurrences of the anchor, ascending.
	var occs []int
	for start := 0; start < len(norm); {
		idx := strings.Index(norm[start:], anchor)
		if idx < 0 {
			break
		}
		occs = append(occs, start+idx)
		start += idx + len(anchor)
	}

	// Prefer the LAST message occurrence that is followed by a thinking
	// marker: that occurrence is the chat copy, not a quote inside the
	// answer, and the marker is this round's preamble (old rounds' markers
	// are above the message and are skipped).
	markers := thinkingMarkerRE.FindAllStringIndex(norm, -1)
	cut := -1
	for _, o := range occs {
		mEnd := -1
		for _, m := range markers {
			if m[0] > o+len(anchor) && m[1] > mEnd {
				mEnd = m[1]
			}
		}
		if mEnd > 0 {
			cut = mEnd
		}
	}
	if cut > 0 {
		return strings.TrimSpace(norm[cut:])
	}
	if len(occs) > 0 {
		return strings.TrimSpace(norm[occs[len(occs)-1]+len(anchor):])
	}
	return ""
}

// stripBaselinePrefix removes pre-existing conversation history from an
// extracted response so continued conversations return only the new text.
// If the prefix doesn't match (page rebuilt), the full text is kept rather
// than losing content.
func stripBaselinePrefix(resp, mdBaseline string) string {
	if mdBaseline != "" && strings.HasPrefix(resp, mdBaseline) {
		resp = strings.TrimSpace(resp[len(mdBaseline):])
	}
	return resp
}

// toolCallOpenRE matches the opening of a simulated tool call, e.g.
// "<read_file ...>". dscli's role prompts (review.md etc.) instruct the
// model to call read_file; the DeepSeek web UI cannot execute tools, so the
// model emits the call line and pauses. Extracting at that point would
// return a useless fragment. Only known tool names followed by an argument
// list match, so a legitimate short answer like "<b>bold</b>" is not
// rejected.
var toolCallOpenRE = regexp.MustCompile(`^<(read_file|write_file|shell|code_edit|code_search|search_file_with_pattern|flycheck|sql)\s`)

// minCompleteResponseLen is the minimum rune length below which an
// extraction result may still be an incomplete fragment (e.g. a simulated
// tool call line) rather than a real answer.
const minCompleteResponseLen = 600

// isCompleteResponse reports whether an extraction result is usable: it
// must be non-empty, free of replacement characters (U+FFFD appears when a
// misaligned slice cuts a multi-byte rune), and not merely the opening of a
// simulated tool call.
func isCompleteResponse(s string) bool {
	if s == "" || strings.ContainsRune(s, '\uFFFD') {
		return false
	}
	if utf8.RuneCountInString(s) > minCompleteResponseLen {
		return true // long text with a body is a real answer
	}
	t := strings.TrimLeft(s, " \t>")
	return !(toolCallOpenRE.MatchString(t) && !strings.Contains(t, "<tool_result"))
}

// answerUsable reports whether an extracted answer is acceptable: it must
// not be an overload notice, must not be structurally truncated, and must
// look like a complete response. It gates the IndexedDB path in
// webchatExtractReloadedIDB, applying the same checks the DOM path in
// webchatWait does. webchatWait cannot use this boolean directly: there
// the checks are classification checks (busy → ErrServerBusy, truncated →
// ErrTruncated, incomplete → keep polling), distinctions this collapse
// would lose.
func answerUsable(text string) bool {
	if isBusyErrorText(text) {
		return false
	}
	if isTruncated(text) {
		return false
	}
	return isCompleteResponse(text)
}

// uiChromeLineRE matches a line of DeepSeek's code-block toolbar that
// innerText includes before the code itself: the language label ("go",
// "json", ...) and the Copy/Download buttons ("Copy", "Download"). They
// are UI chrome, not model output.
var uiChromeLineRE = regexp.MustCompile(`(?i)^(json|yaml|yml|xml|html|css|js|javascript|ts|typescript|python|py|go|golang|java|c|cc|cpp|c\+\+|rust|rs|bash|sh|shell|text|plaintext|copy|download|复制|下载)$`)

// stripUIChromePrefix removes leading code-block toolbar lines from s.
// DeepSeek renders a ```json fence as a toolbar (language label + Copy +
// Download buttons) instead of the fence text, so the extracted answer
// starts with "json\nCopy\nDownload\n{...". These labels would otherwise
// defeat prefix-based checks (e.g. the JSON truncation detection below)
// and pollute the text handed to callers. Only the leading lines are
// stripped — a standalone "Copy" or "Download" line deeper in real prose
// is left untouched.
func stripUIChromePrefix(s string) string {
	lines := strings.Split(s, "\n")
	i := 0
	for i < len(lines) && i < 5 {
		if !uiChromeLineRE.MatchString(strings.TrimSpace(lines[i])) {
			break
		}
		i++
	}
	if i == 0 {
		return s
	}
	return strings.Join(lines[i:], "\n")
}

// isTruncated reports whether s shows clear structural signs of being cut
// off mid-generation. Detection is deliberately conservative - only signals
// a complete answer would never exhibit:
//
//   - An unclosed markdown code fence: the text ends inside a fenced block.
//     Fences are recognised per CommonMark - a fence opens only at the start
//     of a line (up to three leading spaces), with a run of three or more
//     backticks or tildes, and closes on a line whose run of the same
//     character is at least as long. A ``` inside a line is ordinary text
//     (an explanation of the syntax, or a code block quoting a fence), so it
//     never opens or closes anything.
//   - A JSON document that starts like a document (an object or array
//     containing a quoted key) but never terminates: the first JSON value
//     does not decode. JSON followed by prose is not a JSON document and is
//     not flagged.
//
// webchatWait turns such text into ErrTruncated instead of handing a
// silently incomplete answer to the caller.
func isTruncated(s string) bool {
	// DeepSeek's code-block toolbar labels (language name, Copy/Download
	// buttons) prefix the extracted code text. They are UI chrome, not
	// model output, and would defeat the prefix-based JSON check below.
	t := stripUIChromePrefix(strings.TrimSpace(s))

	if t == "" {
		return false
	}
	if hasUnclosedFence(t) {
		return true
	}
	if (strings.HasPrefix(t, "{") || strings.HasPrefix(t, "[")) && strings.Contains(t, `":`) {
		return isTruncatedJSONDocument(t)
	}
	return false
}

// hasUnclosedFence reports whether t ends inside a markdown fenced code
// block. A fence opens at the start of a line - at most three leading spaces
// (four or more indent a code block instead) - with a run of three or more
// backticks or tildes, and closes on a line whose run of the SAME character
// is at least as long and followed only by whitespace (CommonMark). Only
// spaces count as indentation: a tab advances to the next four-column tab
// stop, so a tab-indented line is indented code, not a fence. A backtick
// fence whose info string contains a backtick is not a fence either
// (CommonMark restricts only backtick fences; a tilde fence's info string
// may contain tildes).
//
// Counting ``` occurrences cannot work: a complete answer may legitimately
// carry an odd number of them when a code block quotes a fence, and a lone
// fence inside a line is prose. The open/close state is the only reliable
// signal.
func hasUnclosedFence(t string) bool {
	var (
		open     bool
		fenceCh  byte
		fenceLen int
	)
	for line := range strings.SplitSeq(t, "\n") {
		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) > 3 {
			continue // four+ spaces indent a code block, not a fence
		}
		ch, n := fenceRun(trimmed)
		if n < 3 {
			continue
		}
		if !open {
			if ch == '`' && strings.ContainsRune(trimmed[n:], '`') {
				continue // a backtick fence's info string cannot contain backticks
			}
			open, fenceCh, fenceLen = true, ch, n
			continue
		}
		if ch == fenceCh && n >= fenceLen && strings.TrimSpace(trimmed[n:]) == "" {
			open = false
		}
	}
	return open
}

// fenceRun returns the fence character and run length when s starts with a
// run of three or more backticks or tildes; (0, 0) otherwise.
func fenceRun(s string) (ch byte, n int) {
	if s == "" || (s[0] != '`' && s[0] != '~') {
		return 0, 0
	}
	ch = s[0]
	for n < len(s) && s[n] == ch {
		n++
	}
	if n < 3 {
		return 0, 0
	}
	return ch, n
}

// isTruncatedJSONDocument reports whether t is a JSON document cut off
// mid-generation: its first JSON value must fail to decode. A text whose
// first value decodes cleanly is either a complete document or JSON followed
// by prose, and neither is a truncated JSON document.
func isTruncatedJSONDocument(t string) bool {
	var raw json.RawMessage
	return json.NewDecoder(strings.NewReader(t)).Decode(&raw) != nil
}

// maxBusyErrorLen bounds the busy-error detection to short texts. A real
// expert answer is typically much longer than an overload notice, so a
// long response that merely mentions a phrase ("try again later" inside a
// recommendation) must never be classified as a busy error.
const maxBusyErrorLen = 200

// busyErrorRE matches known DeepSeek overload notices, both Chinese and
// English. These mirror the official API error semantics for 429 (rate
// limit), 500 (server error) and 503 (overloaded) — the transient cases
// whose documented remedy is a brief wait and retry. Permanent errors
// (400/401/402/422) are not listed because retrying them is pointless.
var busyErrorRE = regexp.MustCompile(`(?i)` +
	`服务器忙|服务器繁忙|服务繁忙|系统繁忙|系统正忙|请求过于频繁|操作过于频繁|` +
	`网络异常|网络错误|发送失败|请稍后再试|请稍后重试|请稍候再试|服务器开小差|暂不可用|` +
	`server busy|service is busy|try again later|too many requests|rate limit|` +
	`service unavailable|server error|temporarily unavailable|overloaded`)

// isBusyErrorText reports whether s looks like a server-overload notice
// rather than a real answer: short text matching a known overload phrase.
// Returning such a notice as the expert's answer would silently poison the
// caller's decision-making, so webchatWait turns it into ErrServerBusy.
func isBusyErrorText(s string) bool {
	if s == "" || utf8.RuneCountInString(s) > maxBusyErrorLen {
		return false
	}
	return busyErrorRE.MatchString(s)
}

// quoteJS wraps s in a JS string literal (double quotes) with proper escaping.
func quoteJS(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	return "\"" + escaped + "\""
}

// --- conversation registry --------------------------------------------------
//
// The registry maps conversation IDs to chat.deepseek.com URLs so any saved
// conversation can be continued by ID ("keep=<id>"), by recency ("last"),
// or by URL (browser-copied). Every successful WebChat exchange registers
// the conversation automatically.

// maxSavedConversations caps the registry; the oldest entries are dropped
// when the cap is exceeded.
const maxSavedConversations = 100

// conversationEntry records one known conversation for continuation.
type conversationEntry struct {
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

// conversationRegistry persists known conversations keyed by conversation ID.
type conversationRegistry struct {
	Sessions map[string]conversationEntry `json:"sessions"`
}

// conversationIDRE matches the conversation ID inside a DeepSeek chat URL:
// https://chat.deepseek.com/a/chat/s/<id>
// The host is anchored so lookalike paths on other domains never match
// (a non-DeepSeek URL must not yield an ID that could collide with a real
// conversation's registry key).
var conversationIDRE = regexp.MustCompile(`^https://chat\.deepseek\.com/a/chat/s/([A-Za-z0-9_-]+)`)

// ConversationIDFromURL extracts the conversation ID from a chat.deepseek.com
// conversation URL, or "" if the URL does not look like one.
func ConversationIDFromURL(url string) string {
	m := conversationIDRE.FindStringSubmatch(url)
	if m == nil {
		return ""
	}
	return m[1]
}

// conversationRegistryPath returns the path to the registry file, located
// alongside the Chrome profile directory.
func conversationRegistryPath() (string, error) {
	dir, err := chromeUserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "webchat_sessions.json"), nil
}

// loadConversationRegistry loads the registry, or returns an empty one when
// the file does not exist yet. For backwards compatibility, a legacy
// webchat_session.json (the single last-conversation file) is migrated into
// the registry the first time it is loaded.
func loadConversationRegistry() (*conversationRegistry, error) {
	reg := &conversationRegistry{Sessions: map[string]conversationEntry{}}
	path, err := conversationRegistryPath()
	if err != nil {
		return reg, err
	}
	data, err := os.ReadFile(path)
	if err == nil {
		if uerr := json.Unmarshal(data, reg); uerr == nil && reg.Sessions != nil {
			return reg, nil
		}
		// Corrupt file: rebuild from scratch below.
		reg = &conversationRegistry{Sessions: map[string]conversationEntry{}}
	} else if !errors.Is(err, os.ErrNotExist) {
		return reg, err
	}

	// Migrate the legacy single-conversation file if present.
	dir, err := chromeUserDataDir()
	if err != nil {
		return reg, err
	}
	if legacy, lerr := os.ReadFile(filepath.Join(dir, "webchat_session.json")); lerr == nil {
		var old struct {
			URL string `json:"url"`
		}
		if json.Unmarshal(legacy, &old) == nil && old.URL != "" {
			reg.register(old.URL)
			if serr := reg.save(); serr != nil {
				return reg, serr
			}
		}
	}
	return reg, nil
}

// save writes the registry atomically (temp file + rename) so a crash in the
// middle cannot corrupt the previously saved state.
func (r *conversationRegistry) save() error {
	path, err := conversationRegistryPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// register adds or refreshes a conversation in the registry. The ID is
// extracted from the URL; entries with an unknown ID shape are keyed by the
// URL itself. The registry is trimmed to maxSavedConversations after the
// update.
func (r *conversationRegistry) register(url string) {
	id := ConversationIDFromURL(url)
	if id == "" {
		id = url
	}
	entry, ok := r.Sessions[id]
	if !ok {
		entry = conversationEntry{}
	}
	entry.URL = url
	entry.UpdatedAt = time.Now().Format(time.RFC3339)
	r.Sessions[id] = entry
	r.trim()
}

// trim drops the oldest entries beyond maxSavedConversations. RFC3339
// timestamps sort lexicographically in time order.
func (r *conversationRegistry) trim() {
	if len(r.Sessions) <= maxSavedConversations {
		return
	}
	ids := make([]string, 0, len(r.Sessions))
	for id := range r.Sessions {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		return r.Sessions[ids[i]].UpdatedAt > r.Sessions[ids[j]].UpdatedAt
	})
	kept := make(map[string]conversationEntry, maxSavedConversations)
	for _, id := range ids[:maxSavedConversations] {
		kept[id] = r.Sessions[id]
	}
	r.Sessions = kept
}

// latest returns the URL of the most recently updated conversation.
func (r *conversationRegistry) latest() (string, error) {
	var best conversationEntry
	var found bool
	for _, e := range r.Sessions {
		if !found || e.UpdatedAt > best.UpdatedAt {
			best = e
			found = true
		}
	}
	if !found {
		return "", fmt.Errorf("no saved conversations yet")
	}
	return best.URL, nil
}

// resolve maps a Keep value to a conversation URL to navigate to. "" (new
// conversation) stays "". "last" selects the most recently updated entry. A
// full http(s) URL is used as-is (pre-specified, e.g. copied from a browser;
// the caller registers it). Any other value is looked up as a conversation
// ID: exact key first, then URL match, then URL suffix match.
func (r *conversationRegistry) resolve(keep string) (string, error) {
	if keep == "" {
		return "", nil
	}
	if keep == "last" {
		return r.latest()
	}
	if strings.HasPrefix(keep, "http://") || strings.HasPrefix(keep, "https://") {
		return keep, nil
	}
	if entry, ok := r.Sessions[keep]; ok {
		return entry.URL, nil
	}
	// Suffix/URL match: collect ALL matches so an ambiguous shorthand fails
	// loudly instead of silently picking an arbitrary conversation.
	var matches []string
	for _, entry := range r.Sessions {
		if entry.URL == keep || strings.HasSuffix(entry.URL, "/"+keep) {
			matches = append(matches, entry.URL)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("conversation %q not found (use keep=\"list\" to see saved conversations)", keep)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("conversation %q is ambiguous (%d matches); use the full conversation ID or URL", keep, len(matches))
	}
}

// resolveConversation is the file-backed entry point: it resolves the Keep
// option and registers pre-specified URLs so they can be referenced by ID
// later. Returns "" for a new conversation.
func resolveConversation(keep string) (string, error) {
	if keep == "" {
		return "", nil
	}
	if strings.HasPrefix(keep, "http://") || strings.HasPrefix(keep, "https://") {
		// Pre-specified URL: use directly and register for later ID lookup.
		_ = registerConversation(keep)
		return keep, nil
	}
	reg, err := loadConversationRegistry()
	if err != nil {
		return "", err
	}
	return reg.resolve(keep)
}

// registerConversation is the file-backed wrapper of registry.register. The
// whole read-modify-write is serialized with a file lock so two concurrent
// WebChat calls (e.g. two AI sessions in different processes) cannot lose
// each other's entries; the kernel releases the lock if the process dies.
func registerConversation(url string) error {
	span, _ := clog.StartSpanFromContext(context.Background(), "registerConversation")
	defer span.Finish()
	lk, err := lockfile.LockDB("webchat_sessions")
	if err != nil {
		return err
	}
	defer lk.Close()
	reg, err := loadConversationRegistry()
	if err != nil {
		return err
	}
	reg.register(url)
	return reg.save()
}

// ConversationInfo describes one saved conversation for listing.
type ConversationInfo struct {
	ID        string
	URL       string
	UpdatedAt string
}

// ListConversations returns all saved conversations, most recent first.
func ListConversations() ([]ConversationInfo, error) {
	reg, err := loadConversationRegistry()
	if err != nil {
		return nil, err
	}
	infos := make([]ConversationInfo, 0, len(reg.Sessions))
	for id, e := range reg.Sessions {
		infos = append(infos, ConversationInfo{ID: id, URL: e.URL, UpdatedAt: e.UpdatedAt})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].UpdatedAt > infos[j].UpdatedAt
	})
	return infos, nil
}
