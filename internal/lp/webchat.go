package lp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/context"
	"github.com/nanjj/clog"

	"github.com/chromedp/cdproto/dom"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// ErrLoginRequired is returned when the browser is not logged in to DeepSeek.
// Callers should trigger a visible login flow and retry.
var ErrLoginRequired = errors.New("login required — open visible browser to complete login")

const (
	deepseekChatURL = "https://chat.deepseek.com"

	// Polling configuration for response detection.
	webChatPollInterval  = 2 * time.Second // interval between polls
	webChatStablePolls   = 3               // text unchanged for this many polls = tentative done
	webChatExtendedPolls = 10              // additional stable polls before force-extraction (escape hatch)
	webChatMaxPolls      = 300             // max polls before timeout (600s total)

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

	// Keep continues the last saved conversation instead of starting a
	// new one.
	Keep bool
}

// WebChat sends a message to chat.deepseek.com via a local Chrome/Chromium
// browser and returns the assistant's text response.
//
// The browser is launched fresh for each call and closed after the response is
// received. Cookies persist via the shared Chrome profile directory, so prior
// login state is available across calls.
//
// If ctx carries context.KeepKey set to true, WebChat attempts to continue the
// last saved conversation (loaded from the profile directory) rather than
// starting a new one. New conversations use expert mode (V4 Pro). Use
// WebChatWithOptions for explicit mode selection and file uploads.
func WebChat(ctx context.Context, message string) (string, error) {
	return WebChatWithOptions(ctx, message, WebChatOptions{
		Keep: context.ContextValue(ctx, context.KeepKey, false),
	})
}

// WebChatWithOptions is WebChat with explicit mode and attachment options.
// Options are normalized and validated before a browser is launched, so bad
// input (unknown mode, oversized attachments) fails fast without starting
// Chrome. See WebChatOptions for the mode defaults.
func WebChatWithOptions(ctx context.Context, message string, opts WebChatOptions) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "WebChatWithOptions")
	defer span.Finish()
	opts = normalizeWebChatOptions(opts)
	if err := validateWebChatOptions(opts); err != nil {
		return "", err
	}
	convURL := ""
	if opts.Keep {
		convURL = loadConversationURL()
	}
	return webChatWithURL(ctx, convURL, message, opts)
}

// normalizeWebChatOptions fills in implicit mode choices.
func normalizeWebChatOptions(opts WebChatOptions) WebChatOptions {
	if opts.Mode == "" {
		switch {
		case len(opts.Attachments) > 0:
			opts.Mode = ModeVision // the multi-modal mode
		case !opts.Keep:
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
func webChatWithURL(ctx context.Context, conversationURL, message string, opts WebChatOptions) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "webChatWithURL")
	defer span.Finish()
	ctx, cancel, err := NewChromium(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()
	ctx, close := chromedp.NewContext(ctx)
	defer close()
	response, finalURL, err := webchatSend(ctx, conversationURL, message, opts, 0)
	if err != nil {
		return "", fmt.Errorf("webchat: %w", err)
	}

	if finalURL != "" {
		_ = saveConversationState(finalURL)
	}

	return response, nil
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

	if finalURL != "" {
		fmt.Fprintf(os.Stderr, "💾 会话 URL: %s\n", finalURL)
	}

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
func webchatWait(ctx context.Context, baseline, mdBaseline string) (string, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "webchatWait")
	defer span.Finish()
	var lastText string
	stableCount := 0

	for range webChatMaxPolls {
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
				if fallback := cleanBodyResponse(extractResponse(baseline, current)); canExtract && isCompleteResponse(fallback) {
					return fallback, nil
				}
			}
		} else {
			stableCount = 0
		}
		lastText = current
	}

	return "", fmt.Errorf("response timeout after %d polls", webChatMaxPolls)
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

// quoteJS wraps s in a JS string literal (double quotes) with proper escaping.
func quoteJS(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	return "\"" + escaped + "\""
}

// --- conversation state persistence ------------------------------------------

// conversationState stores the last conversation info for continuation.
type conversationState struct {
	URL       string `json:"url"`
	UpdatedAt string `json:"updated_at"`
}

// conversationStatePath returns the path to the session state file,
// located alongside the Chrome profile directory.
func conversationStatePath() (string, error) {
	dir, err := chromeUserDataDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "webchat_session.json"), nil
}

// saveConversationState persists the conversation URL for later continuation.
func saveConversationState(convURL string) error {
	span, _ := clog.StartSpanFromContext(context.Background(), "saveConversationState")
	defer span.Finish()
	path, err := conversationStatePath()
	if err != nil {
		return err
	}
	state := conversationState{
		URL:       convURL,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// loadConversationURL loads the last saved conversation URL, or "" if none.
func loadConversationURL() string {
	path, err := conversationStatePath()
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var state conversationState
	if err := json.Unmarshal(data, &state); err != nil {
		return ""
	}
	return state.URL
}
