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
	"time"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/lockfile"
	"github.com/nanjj/clog"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/page"
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

const (
	deepseekChatURL = "https://chat.deepseek.com"

	// Polling configuration for response detection.
	webChatPollInterval     = 2 * time.Second // interval between polls
	webChatStablePolls      = 3               // text unchanged for this many polls = tentative done
	webChatExtendedPolls    = 10              // additional stable polls before force-extraction (escape hatch)
	webChatMaxPolls         = 300             // max polls before timeout (600s total)
	webChatConfirmPolls     = 5               // send-ack window: polls to observe send confirmation (10s)
	webChatEmptyStablePolls = 30              // stable-with-no-content polls (60s) before treating as server busy

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

	// jsSelectModeFmt switches the model selector to the requested mode
	// (flash = 快速模式, pro = 专家模式, vision = 识图模式). %s is the
	// quoted mode name. DeepSeek renders the selector as radio buttons with
	// data-model-type attributes; the strategy prefers the structural
	// attribute and falls back to label text:
	//   data-model-type="default" → 快速模式 (flash)
	//   data-model-type="<other>" → 专家模式 / V4 Pro (pro)
	// The third radio (识图模式 / vision) is matched by label text or a
	// vision-ish attribute value.
	jsSelectModeFmt = `(() => {
		const want = %s;
		const radios = document.querySelectorAll('[data-model-type]');
		if (radios.length === 0) {
			return {success: false, error: 'no mode selector found'};
		}
		// Label text: the element's own text first, then its ancestors.
		// The radio's parent is a shared container whose text contains ALL
		// mode labels, so ancestor walks must never be the primary source.
		const labelOf = function(el) {
			var t = (el.textContent || '').trim();
			if (t) return t;
			var p = el.parentElement;
			for (var i = 0; i < 2 && p; i++) {
				t = (p.textContent || '').trim();
				if (t) return t;
				p = p.parentElement;
			}
			return '';
		};
		const find = function(pred) {
			for (const r of radios) {
				if (pred(r)) return r;
			}
			return null;
		};
		const lt = function(r) { return labelOf(r).toLowerCase(); };
		let target = null;
		let method = '';
		if (want === 'flash') {
			target = find(function(r) { return r.getAttribute('data-model-type') === 'default'; });
			if (target) { method = 'data-model-type=default'; }
			else {
				target = find(function(r) { return lt(r).indexOf('快速') !== -1; });
				if (target) method = 'label=快速模式';
			}
		} else if (want === 'pro') {
			target = find(function(r) { return r.getAttribute('data-model-type') === 'expert'; });
			if (target) { method = 'data-model-type=expert'; }
			else {
				// Fallback for older UIs: first non-default radio that is
				// not the vision one (flash is excluded by data-model-type,
				// vision by its own label text).
				target = find(function(r) {
					const t = r.getAttribute('data-model-type');
					const own = (r.textContent || '').toLowerCase();
					return t && t !== 'default' && own.indexOf('识图') === -1 && own.indexOf('vision') === -1;
				});
				if (target) { method = 'data-model-type=' + target.getAttribute('data-model-type'); }
				else {
					target = find(function(r) { return lt(r).indexOf('专家') !== -1; });
					if (target) method = 'label=专家模式';
				}
			}
			if (!target) {
				// Last resort for the two-radio UI: first non-default.
				target = find(function(r) { return r.getAttribute('data-model-type') !== 'default'; });
				if (target) method = 'data-model-type!=default';
			}
		} else if (want === 'vision') {
			// Attribute first: the real DOM uses data-model-type="vision",
			// and label matching is ambiguous — the flash radio's shared
			// ancestor container also contains the 识图 label.
			target = find(function(r) {
				const t = (r.getAttribute('data-model-type') || '').toLowerCase();
				return t.indexOf('vision') !== -1 || t.indexOf('image') !== -1;
			});
			if (target) { method = 'data-model-type=' + target.getAttribute('data-model-type'); }
			else {
				target = find(function(r) { return lt(r).indexOf('识图') !== -1 || lt(r).indexOf('vision') !== -1; });
				if (target) method = 'label=识图模式';
			}
		}
		if (!target) {
			return {success: false, error: 'mode not found: ' + want};
		}
		target.click();
		return {success: true, mode: want, method: method, modelType: target.getAttribute('data-model-type')};
	})()`

	// jsToggleChipFmt ensures a labeled toggle chip (深度思考, 智能搜索) is
	// in the wanted state. %s1 is a JSON array of label fragments, %s2 is
	// "true" or "false". Chips are buttons/labels whose text matches; the
	// active state is read from aria attributes, data-state, or class
	// names, and the chip is clicked only when its state differs from
	// wanted (clicking an already-active toggle would turn it OFF).
	jsToggleChipFmt = `(() => {
		const labels = %s;
		const wantActive = %s;
		const els = document.querySelectorAll('button, [role="button"], [role="switch"], [role="checkbox"], [role="radio"], label, [class*="toggle"], [class*="chip"], [class*="option"]');
		let best = null;
		for (const el of els) {
			const t = (el.textContent || '').trim();
			if (t.length === 0 || t.length > 30) continue;
			const hit = labels.some(function(l) { return t === l || t.indexOf(l) !== -1; });
			if (!hit) continue;
			// Skip plain text wrappers (span without role or chip class).
			const tag = el.tagName.toLowerCase();
			const role = el.getAttribute('role') || '';
			if (tag === 'span' && !role && !/\b(toggle|chip|option)\b/.test(el.className || '')) continue;
			best = el;
			break;
		}
		if (!best) {
			return {success: false, error: 'toggle not found: ' + labels.join('/')};
		}
		const cls = best.className || '';
		const stateAttr = best.getAttribute('aria-pressed') || best.getAttribute('aria-checked') ||
			best.getAttribute('aria-selected') || best.getAttribute('data-state') || '';
		const active = stateAttr === 'true' || stateAttr === 'active' || stateAttr === 'checked' ||
			stateAttr === 'selected' || /\b(active|selected|checked|on)\b/.test(cls);
		if (active === wantActive) {
			return {success: true, already: true, label: best.textContent.trim()};
		}
		best.click();
		return {success: true, clicked: true, wasActive: active, label: best.textContent.trim()};
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

	// jsUploadPreviewCountFmt counts blob/data-URL images, which is how the
	// chat renders upload previews. Used to confirm React picked up the files.
	jsUploadPreviewCountFmt = `(() => {
		const imgs = document.querySelectorAll('img[src^="blob:"], img[src^="data:image/"]');
		return {count: imgs.length};
	})()`

	// jsGetAssistantText extracts all assistant response text from
	// .ds-markdown elements in the MAIN content area (NOT sidebar/navigation).
	// It concatenates all blocks to handle responses split across multiple
	// elements (paragraphs, code blocks, lists).
	// NOTE: the result may include pre-existing conversation history (e.g.
	// continued conversations); webchatWait strips it via mdBaseline.
	jsGetAssistantText = `(() => {
	const all = document.querySelectorAll('.ds-markdown');
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
		return true;
	});
	if (els.length === 0) return '';
	// Concatenate ALL .ds-markdown elements, not just the last one.
	// Streaming responses may be split across multiple blocks.
	return els.map(function(el) { return el.innerText || ''; }).join('\n\n').trim();
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

	// jsSendEnter dispatches Enter keydown → keypress → keyup on the chat
	// textarea via JS.  Using KeyboardEvent dispatch instead of chromedp.KeyEvent

	// because the latter may not trigger React's event handling in a remote
	// allocator (chromium service) context.
	// The full sequence (keydown → keypress → keyup) matches what a real
	// keyboard produces, improving compatibility with frameworks that listen
	// for specific events.
	// Additionally, click the send button as a fallback to ensure the message
	// is submitted even if the KeyboardEvent dispatch doesn't trigger React's
	// submit handler (e.g. when focus is on the left sidebar conversation list
	// and the textarea isn't the active element).
	jsSendEnter = `(() => {
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
		// Fallback: if the KeyboardEvent dispatch didn't trigger React's
		// submit handler the textarea still has content — click the send
		// button as a backup. Only fire when ta.value is non-empty to
		// avoid double-submission.
		var sendBtn = document.querySelector('[role="button"].ds-button--primary');
		if (sendBtn && sendBtn.offsetParent !== null && ta.value.trim() !== '') {
			sendBtn.click();
		}
		return {success: true};
	})()`
)

// Mode selects which DeepSeek web chat mode to use.
type Mode string

const (
	// ModePro is 专家模式 (V4 Pro): deep think only, no uploads.
	ModePro Mode = "pro"
	// ModeFlash is 快速模式 (V4 Flash): deep think, smart search and uploads.
	ModeFlash Mode = "flash"
	// ModeVision is 识图模式 (V4 Vision): deep think and uploads.
	ModeVision Mode = "vision"
)

// validModes lists the modes accepted by validateWebChatOptions.
var validModes = map[Mode]bool{
	ModePro:    true,
	ModeFlash:  true,
	ModeVision: true,
}

// WebChatOptions configures a WebChat call.
type WebChatOptions struct {
	// Mode selects the web chat mode (ModePro, ModeFlash, ModeVision).
	// Empty means: pro for new conversations, vision when attachments are
	// given, and the conversation's existing mode is preserved when Keep
	// is true.
	Mode Mode

	// Attachments are image file paths uploaded to the chat. Only flash
	// and vision modes support uploads (up to 50 files, 100MB total).
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
}

// WebChatResult is the outcome of a WebChat call: the assistant's text
// response plus the final conversation URL, which contains the conversation
// ID usable with WebChatOptions.Keep to continue the same conversation
// later. URL is "" if it could not be determined.
type WebChatResult struct {
	Text string
	URL  string
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
// conversations use expert mode (V4 Pro). Use WebChatWithOptions for explicit
// mode selection, file uploads, and the full Keep value set.
func WebChat(ctx context.Context, message string) (string, error) {
	res, err := WebChatWithOptions(ctx, message, WebChatOptions{
		Keep: context.ContextValue(ctx, context.KeepKey, ""),
	})
	if err != nil {
		return "", err
	}
	return res.Text, nil
}

// WebChatWithOptions is WebChat with explicit mode, attachment and
// continuation options. Options are normalized, attachment paths resolved to
// absolute, and the result validated before a browser is launched, so bad
// input (unknown mode, missing/oversized attachments, unknown keep target)
// fails fast without starting Chrome.
//
// See WebChatOptions for the mode and Keep defaults.
func WebChatWithOptions(ctx context.Context, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "WebChatWithOptions")
	defer span.Finish()
	opts = normalizeWebChatOptions(opts)
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

// normalizeWebChatOptions fills in implicit mode choices.
func normalizeWebChatOptions(opts WebChatOptions) WebChatOptions {
	if opts.Mode == "" {
		switch {
		case len(opts.Attachments) > 0:
			opts.Mode = ModeVision // the multi-modal mode
		case opts.Keep == "":
			opts.Mode = ModePro // default for new conversations
		}
	}
	return opts
}

// validateWebChatOptions checks mode and attachment limits before launching
// a browser, so bad input fails fast without starting Chrome.
func validateWebChatOptions(opts WebChatOptions) error {
	if opts.Mode != "" && !validModes[opts.Mode] {
		return fmt.Errorf("unknown webchat mode %q (want flash, pro or vision)", opts.Mode)
	}
	if opts.Mode == ModePro && len(opts.Attachments) > 0 {
		return fmt.Errorf("attachments require flash or vision mode, got %q", opts.Mode)
	}
	return validateWebAttachments(opts.Attachments)
}

// webChatWithURL is the common implementation shared by new conversations
// (empty url) and continuation (saved url).
func webChatWithURL(ctx context.Context, conversationURL, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "webChatWithURL")
	defer span.Finish()
	ctx, cancel, err := NewChromium(ctx)
	if err != nil {
		return WebChatResult{}, err
	}
	defer cancel()
	ctx, close := chromedp.NewContext(ctx)
	defer close()
	response, finalURL, err := webchatSend(ctx, conversationURL, message, opts, 0)
	if err != nil {
		return WebChatResult{}, fmt.Errorf("webchat: %w", err)
	}

	// Every successful exchange registers (or refreshes) the conversation
	// in the registry so it can be continued later by ID or "last".
	if finalURL != "" {
		_ = registerConversation(finalURL, opts.Mode)
	}

	return WebChatResult{Text: response, URL: finalURL}, nil
}

// webchatSend sends a message and returns the response plus the final page URL
// (which contains the conversation ID for continuation). If login is needed,
// it triggers a manual login flow in the same Chrome session and retries once.
//
// opts.Mode selects the web chat mode; an empty mode leaves the
// conversation's current mode untouched (used when continuing a
// conversation). opts.Attachments are image files uploaded before sending
// (flash/vision modes only; pro rejects them in validateWebChatOptions).
func webchatSend(tabCtx context.Context, conversationURL, message string, opts WebChatOptions, retry int) (string, string, error) {
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

	var baseline, response, finalURL, mdBaseline string

	// Base navigation and page hydration.
	actions := []chromedp.Action{
		chromedp.Navigate(navURL),
		chromedp.WaitReady("body"),
	}

	// For continuing conversations: wait longer for chat history to load.
	if !isNewConv {
		actions = append(
			actions,
			chromedp.Sleep(2*time.Second),
			// Wait for at least one .ds-markdown element (conversation loaded).
			chromedp.WaitVisible(".ds-markdown", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	} else {
		actions = append(
			actions,
			chromedp.Sleep(3*time.Second),
		)
	}

	// Apply the requested mode: model selector radio, deep think for every
	// mode, smart search for flash. Skipped when mode is "" (continue the
	// conversation with its existing mode).
	if opts.Mode != "" {
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			webchatApplyMode(ctx, opts.Mode)
			return nil
		}))
		// Pause for the toggle to take effect before textarea interaction.
		actions = append(actions, chromedp.Sleep(1*time.Second))
	}

	// Upload attachments (flash/vision modes only). Fatal on failure: files
	// the caller asked to attach must not be silently dropped.
	if len(opts.Attachments) > 0 {
		actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
			return webchatUpload(ctx, opts.Attachments)
		}))
	}

	actions = append(
		actions,
		// Record baseline text before sending.
		chromedp.Evaluate("document.body ? document.body.innerText : ''", &baseline),

		// Record the .ds-markdown baseline (all assistant text visible
		// before sending). webchatWait strips this prefix from the
		// extracted text so continued conversations don't include
		// pre-existing history.
		chromedp.Evaluate(jsGetAssistantText, &mdBaseline),

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
			response, err = webchatWait(ctx, baseline, mdBaseline)
			return err
		}),

		// Capture the final URL (contains conversation ID).
		chromedp.Location(&finalURL),
	)

	err := chromedp.Run(ctx, actions...)
	if err != nil {
		// If login is needed and we haven't retried yet, perform login
		// in the same Chrome session and retry once.
		if errors.Is(err, ErrLoginRequired) && retry == 0 {
			fmt.Fprintln(os.Stderr, "🔐 未登录，在浏览器窗口中完成登录...")
			if loginErr := deepseekLogin(ctx, "", nil, true); loginErr != nil {
				return "", "", fmt.Errorf("webchat login: %w", loginErr)
			}
			return webchatSend(ctx, conversationURL, message, opts, retry+1)
		}
		return "", "", fmt.Errorf("webchat: %w", err)
	}

	// Session info (keep:<id>) is surfaced by the caller (CLI / ask_expert),
	// not here, so the library layer stays silent about presentation.
	return response, finalURL, nil
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

// webchatApplyMode switches the model selector to the requested mode and
// ensures the deep-think toggle (and smart search for flash) is on. Mode
// selection is best-effort: a UI change that breaks the selector is logged
// loudly but does not fail the chat (the page keeps its current mode).
// Deep think is inherent in expert mode, so chips are only toggled for
// flash and vision.
func webchatApplyMode(ctx context.Context, mode Mode) {
	var result map[string]any
	js := fmt.Sprintf(jsSelectModeFmt, quoteJS(string(mode)))
	if err := chromedp.Evaluate(js, &result).Do(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ 模式切换失败 (%s): %v\n", mode, err)
		return
	}
	if ok, _ := result["success"].(bool); !ok {
		msg, _ := result["error"].(string)
		fmt.Fprintf(os.Stderr, "⚠️ 模式切换失败 (%s): %s\n", mode, msg)
		return
	}
	method, _ := result["method"].(string)
	modelType, _ := result["modelType"].(string)
	switch mode {
	case ModeFlash:
		fmt.Fprintf(os.Stderr, "⚡ 已启用快速模式 (%s%s)\n", method, modelSuffix(modelType))
	case ModeVision:
		fmt.Fprintf(os.Stderr, "👁 已启用识图模式 (%s%s)\n", method, modelSuffix(modelType))
	default:
		fmt.Fprintf(os.Stderr, "🔬 已启用专家模式 (%s%s)\n", method, modelSuffix(modelType))
	}
	// Deep think is available in every mode except that expert mode has it
	// built in; flash and vision expose a chip.
	if mode == ModeFlash || mode == ModeVision {
		webchatToggleChip(ctx, []string{"深度思考", "Deep Think"}, true)
	}
	// Smart search is a flash-mode extra.
	if mode == ModeFlash {
		webchatToggleChip(ctx, []string{"智能搜索", "联网搜索", "Deep Search"}, true)
	}
}

// modelSuffix formats an optional model type for the mode log line.
func modelSuffix(modelType string) string {
	if modelType == "" {
		return ""
	}
	return ", model=" + modelType
}

// webchatToggleChip ensures a labeled toggle chip is in the wanted state.
// Best-effort: missing chips (e.g. smart search in pro mode) and state
// detection failures are logged but never fail the chat. Chips already in
// the wanted state are left untouched silently.
func webchatToggleChip(ctx context.Context, labels []string, wantActive bool) {
	quoted := make([]string, len(labels))
	for i, l := range labels {
		quoted[i] = quoteJS(l)
	}
	want := "false"
	if wantActive {
		want = "true"
	}
	js := fmt.Sprintf(jsToggleChipFmt, "["+strings.Join(quoted, ", ")+"]", want)
	var result map[string]any
	if err := chromedp.Evaluate(js, &result).Do(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ 开关设置失败 (%s): %v\n", strings.Join(labels, "/"), err)
		return
	}
	if ok, _ := result["success"].(bool); !ok {
		msg, _ := result["error"].(string)
		fmt.Fprintf(os.Stderr, "⚠️ %s\n", msg)
		return
	}
	if already, _ := result["already"].(bool); already {
		return // already in the wanted state
	}
	label, _ := result["label"].(string)
	wasActive, _ := result["wasActive"].(bool)
	from, to := "关", "开"
	if wasActive {
		from = "开"
	}
	if !wantActive {
		to = "关"
	}
	fmt.Fprintf(os.Stderr, "🔘 %s: %s → %s\n", label, from, to)
}

// Web chat upload limits enforced by chat.deepseek.com.
const (
	webUploadMaxFiles = 50
	webUploadMaxTotal = 100 << 20 // 100MB total
)

// validateWebAttachments checks the web chat upload limits: at most 50
// files and 100MB total. Non-image extensions are warned about (the page
// only recognizes text embedded in images) but not rejected.
func validateWebAttachments(files []string) error {
	if len(files) == 0 {
		return nil
	}
	if len(files) > webUploadMaxFiles {
		return fmt.Errorf("too many attachments: %d (max %d)", len(files), webUploadMaxFiles)
	}
	var total int64
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			return fmt.Errorf("attachment %s: %w", f, err)
		}
		total += info.Size()
		if !IsImageFile(f) {
			fmt.Fprintf(os.Stderr, "⚠️ 附件不是常见图片格式，网页版可能不支持: %s\n", f)
		}
	}
	if total > webUploadMaxTotal {
		return fmt.Errorf("attachments too large: %d bytes (max %d)", total, webUploadMaxTotal)
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

// webchatUpload attaches files to the chat via the hidden file input. The
// direct path sets files on the input node with CDP DOM.setFileInputFiles,
// which fires React's change handler without opening a native dialog. If
// the input is missing, the upload button is clicked with file-chooser
// interception enabled and the opened chooser is completed programmatically.
func webchatUpload(ctx context.Context, files []string) error {
	if err := validateWebAttachments(files); err != nil {
		return err
	}
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
// waits (best-effort) for preview thumbnails to confirm React picked them up.
func webchatSetUploadFiles(ctx context.Context, files []string) error {
	if err := chromedp.SetUploadFiles("input[type='file']", files, chromedp.ByQuery).Do(ctx); err != nil {
		return fmt.Errorf("set upload files: %w", err)
	}
	fmt.Fprintf(os.Stderr, "📎 已添加 %d 个附件\n", len(files))
	for range 10 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
		var countResult map[string]any
		if err := chromedp.Evaluate(jsUploadPreviewCountFmt, &countResult).Do(ctx); err != nil {
			continue
		}
		if n, _ := countResult["count"].(float64); int(n) >= len(files) {
			fmt.Fprintf(os.Stderr, "🖼 %d 个附件预览已就绪\n", int(n))
			return nil
		}
	}
	fmt.Fprintln(os.Stderr, "⚠️ 附件预览未确认，继续发送")
	return nil
}

// webchatWait polls until the assistant response stabilizes, then extracts
// it via the .ds-markdown element (preferred) or body-text diff (fallback).
//
// mdBaseline is the concatenated .ds-markdown text captured before sending;
// it is stripped from the extracted text so continued conversations return
// only the new response.
//
// To avoid premature extraction during streaming pauses (>6s), it uses a
// generation-active check: when stability is first detected, it checks
// whether DeepSeek is still generating (via DOM signals like the stop button).
// Only extracts when generation appears complete or the extended poll window
// expires (escape hatch after webChatExtendedPolls additional polls).
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
//
// The poll budget comes from webChatPollBudget: when the context carries a
// deadline (the tool framework derives it from the ask_expert timeout
// argument), we poll until that deadline instead of the hardcoded
// webChatMaxPolls — so a caller-passed timeout (e.g. 1200s) genuinely extends
// the wait for long generations (full 26-question papers can exceed 600s).
func webchatWait(ctx context.Context, baseline, mdBaseline string) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "webchatWait")
	defer span.Finish()
	var lastText string
	stableCount := 0
	emptyStableCount := 0
	sendAck := false
	polls := 0
	maxPolls := webChatPollBudget(ctx)

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

		// Send-ack window: the message must show evidence of submission
		// within the confirmation window. Any of new body content, an
		// active generation, or a cleared textarea counts as ack.
		// While unconfirmed we skip the normal stability logic so a
		// rejected submit fails fast with ErrSendRejected (10s) instead
		// of being misread as an empty stable page (60s) or timing out.
		if !sendAck {
			sendAck = current != baseline || isGenerationActive(ctx) || textareaCleared(ctx)
			if !sendAck {
				if polls >= webChatConfirmPolls {
					return "", ErrSendRejected
				}
				lastText = current
				continue
			}
		}

		if current == lastText && lastText != "" {
			stableCount++

			if stableCount >= webChatStablePolls {
				// Gated extraction: only return when generation appears complete
				// or the escape hatch fires.
				canExtract := !isGenerationActive(ctx) ||
					stableCount >= webChatStablePolls+webChatExtendedPolls

				// Preferred: extract from .ds-markdown elements.
				// This naturally excludes UI chrome (search info,
				// toggle labels, footer text).
				if resp := getAssistantText(ctx); resp != "" {
					resp = stripBaselinePrefix(resp, mdBaseline)
					// A short overload notice must never be returned as an
					// answer — it would poison the caller's decision-making.
					if isBusyErrorText(resp) {
						return "", fmt.Errorf("%w: %s", ErrServerBusy, resp)
					}
					if canExtract && isCompleteResponse(resp) {
						return resp, nil
					}
					// Fragment or generation still active: keep polling.
					// The model often pauses after emitting a simulated
					// tool call (<read_file ...>) that the web UI cannot
					// execute; returning the fragment would lose the rest
					// of the answer.
					continue
				}

				// Fallback: diff body text against baseline, then
				// clean up known artifact patterns. Only accept clean,
				// complete text; keep polling on fragments instead of
				// aborting on a mid-response pause.
				fallback := cleanBodyResponse(extractResponse(baseline, current))
				if isBusyErrorText(fallback) {
					return "", fmt.Errorf("%w: %s", ErrServerBusy, fallback)
				}
				if canExtract && isCompleteResponse(fallback) {
					return fallback, nil
				}

				// Stable page with no answer content: the request stalled
				// server-side. Count consecutive empty polls and fail fast
				// instead of waiting out the full 300-poll timeout.
				emptyStableCount++
				if emptyStableCount >= webChatEmptyStablePolls {
					return "", ErrServerBusy
				}
				continue
			}
		} else {
			stableCount = 0
			emptyStableCount = 0
		}
		lastText = current
	}

	return "", fmt.Errorf("response timeout after %d polls (%.0fs)", maxPolls, float64(maxPolls)*webChatPollInterval.Seconds())
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

// getAssistantText returns the concatenated text of all .ds-markdown
// elements in the main content area, or "" if the selector doesn't match
// (e.g. DeepSeek changed their DOM). The text may include pre-existing
// conversation history; webchatWait strips it via mdBaseline.
func getAssistantText(ctx context.Context) string {
	span, ctx := clog.StartSpanFromContext(ctx, "getAssistantText")
	defer span.Finish()

	var text string
	if err := chromedp.Evaluate(jsGetAssistantText, &text).Do(ctx); err != nil {
		return ""
	}
	return strings.TrimSpace(text)
}

// cleanBodyResponse removes DeepSeek UI chrome artifacts from
// body-text-diff output. These artifacts appear when the .ds-markdown
// selector fails and we fall back to body.innerText diff.
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

		filtered = append(filtered, line)
	}
	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

// matchCitationLine reports whether s is a standalone citation
// reference like "- 2" or "-10" or "— 10".
var citationLineRE = regexp.MustCompile(`^[-–—]\s*\d+$`)

func matchCitationLine(s string) bool {
	return citationLineRE.MatchString(s)
}

// extractResponse computes the text added after baseline. current must
// start with baseline: body.innerText changes during generation (the
// textarea clears after send, the stop button appears and disappears), so a
// naive suffix slice at len(baseline) would return garbage from a
// misaligned offset — e.g. a lone U+FFFD replacement character.
func extractResponse(baseline, current string) string {
	if len(current) > len(baseline) && strings.HasPrefix(current, baseline) {
		return strings.TrimSpace(current[len(baseline):])
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
	Mode      Mode   `json:"mode,omitempty"` // mode of the last exchange ("" = unknown)
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
			reg.register(old.URL, "")
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
// URL itself. An empty mode (a continuation preserved the conversation's own
// mode) keeps the previously recorded mode. The registry is trimmed to
// maxSavedConversations after the update.
func (r *conversationRegistry) register(url string, mode Mode) {
	id := ConversationIDFromURL(url)
	if id == "" {
		id = url
	}
	entry, ok := r.Sessions[id]
	if !ok {
		entry = conversationEntry{}
	}
	entry.URL = url
	if mode != "" {
		entry.Mode = mode
	}
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
		_ = registerConversation(keep, "")
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
func registerConversation(url string, mode Mode) error {
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
	reg.register(url, mode)
	return reg.save()
}

// ConversationInfo describes one saved conversation for listing.
type ConversationInfo struct {
	ID        string
	URL       string
	Mode      Mode
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
		infos = append(infos, ConversationInfo{ID: id, URL: e.URL, Mode: e.Mode, UpdatedAt: e.UpdatedAt})
	}
	sort.Slice(infos, func(i, j int) bool {
		return infos[i].UpdatedAt > infos[j].UpdatedAt
	})
	return infos, nil
}
