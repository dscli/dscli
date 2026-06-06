package lp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

// ErrLoginRequired is returned when the browser is not logged in to DeepSeek.
// Callers should trigger a visible login flow and retry.
var ErrLoginRequired = errors.New("login required — open visible browser to complete login")

// ErrNoConversation is returned by WebChatContinue when no previous
// conversation exists to continue.
var ErrNoConversation = errors.New("no previous conversation — start a new one first")

const (
	deepseekChatURL = "https://chat.deepseek.com"

	// Polling configuration for response detection.
	webChatPollInterval = 2 * time.Second // interval between polls
	webChatStablePolls  = 3               // text unchanged for this many polls = done
	webChatMaxPolls     = 60              // max polls before timeout (120s total)

	// JS snippet to set a textarea's value via the native setter (triggers
	// React/Vue change detection). The %s placeholder receives the JS-quoted
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

	// jsEnableDeepThink switches the model selector to expert/deepthink mode.
	// DeepSeek renders radio buttons with data-model-type attributes:
	//   data-model-type="default" → 快速模式 (quick)
	//   data-model-type="<other>"  → 专家模式 / V4 Pro (expert)
	//
	// Primary strategy: find the radio with data-model-type != "default" and
	// click it. This is stable across UI changes because the attribute is
	// structural, not textual.
	//
	// Fallback: if no data-model-type radios are found (older UI), search for
	// the thinking/deepthink toggle by text content.
	jsEnableDeepThink = `(() => {
	// Strategy 1: data-model-type radio buttons (current UI).
	for (const el of document.querySelectorAll('[data-model-type]')) {
		if (el.getAttribute('data-model-type') !== 'default') {
			el.click();
			return {success: true, modelType: el.getAttribute('data-model-type'), method: 'data-model-type'};
		}
	}
	// Strategy 2: text-based toggle (older UI).
	const patterns = ['深度思考','DeepThink','Deep Think',
		'V4 Pro','V4Pro','v4 pro','v4pro',
		'R1','V4','专家','深度'];
	for (const el of document.querySelectorAll('div,button,span,label')) {
		const t = (el.textContent || '').trim();
		if (t.length === 0 || t.length >= 40) continue;
		for (const p of patterns) {
			if (t.includes(p)) {
				el.click();
				return {success: true, clicked: t, method: 'text'};
			}
		}
	}
	return {success: false};
})()`

	// jsGetLastAssistantText extracts the text of the last assistant
	// message via the .ds-markdown class used by DeepSeek for rendered
	// markdown content. Returns "" when no assistant message exists.
	// This is preferred over body.innerText diff because it naturally
	// excludes UI chrome (search info, toggle labels, footer text).
	jsGetLastAssistantText = `(() => {
	const els = document.querySelectorAll('.ds-markdown');
	if (els.length === 0) return '';
	return els[els.length - 1].innerText || '';
})()`
	// jsSendEnter dispatches Enter keydown + keypress + keyup on the chat
	// textarea via JS.  Using KeyboardEvent dispatch instead of chromedp.KeyEvent
	// because the latter may not trigger React's event handling in a remote
	// allocator (chromium service) context.
	// The full sequence (keydown → keypress → keyup) matches what a real
	// keyboard produces, improving compatibility with frameworks that listen
	// for specific events.
	jsSendEnter = `(() => {
		const ta = document.querySelector('textarea');
		if (!ta) return {error: 'no textarea'};
		ta.focus();
		const opts = {
			key: 'Enter', code: 'Enter', keyCode: 13, which: 13,
			bubbles: true, cancelable: true,
		};
		ta.dispatchEvent(new KeyboardEvent('keydown', opts));
		ta.dispatchEvent(new KeyboardEvent('keypress', opts));
		ta.dispatchEvent(new KeyboardEvent('keyup', opts));
		return {success: true};
	})()`
)

// WebChat sends a message to chat.deepseek.com via a visible Chrome browser
// and returns the assistant's text response. Each call starts a **new**
// conversation; the conversation state is saved so it can be continued later
// via WebChatContinue.
//
// It uses the same Chrome user data directory as DeepSeekLoginChromeOpts,
// so cookies from a prior login are automatically available. If not logged
// in, a visible login flow is triggered automatically in the same browser
// session — no separate WebChatLogin call needed.
//
// New conversations automatically enable expert mode (V4 Pro).
//
// Usage:
//
//	response, err := lp.WebChat(ctx, "hello")
func WebChat(ctx context.Context, message string) (string, error) {
	return webChatWithURL(ctx, "", message)
}

// WebChatContinue sends a message continuing the last conversation saved
// by a previous WebChat call. The conversation state is loaded from the
// shared Chrome profile directory.
//
// Returns ErrNoConversation if no previous conversation exists.
func WebChatContinue(ctx context.Context, message string) (string, error) {
	convURL := loadConversationURL()
	if convURL == "" {
		return "", ErrNoConversation
	}
	return webChatWithURL(ctx, convURL, message)
}

// webChatWithURL is the common implementation shared by WebChat
// (new conv, empty url) and WebChatContinue (saved url).
//
// It tries to use the running dscli-chromium service first. If the service is
// unavailable, it falls back to launching a local Chrome/Chromium instance.
// When local Chrome is used, the browser is closed after each call; when the
// remote service is used, the browser stays running.
func webChatWithURL(ctx context.Context, conversationURL, message string) (string, error) {
	var tabCtx context.Context

	// Try remote chromium service first.
	if IsChromiumAvailable() {
		ctx2, cancel, err := ConnectChromium(ctx)
		if err == nil {
			tabCtx = ctx2
			defer cancel()
			fmt.Fprintf(os.Stderr, "🌐 连接到 Chromium 服务 (%s)\n", chromiumAddr())
		}
	}

	// Fall back to launching local Chrome.
	if tabCtx == nil {
		chromePath, err := findChrome()
		if err != nil {
			return "", err
		}
		userDataDir, err := chromeUserDataDir()
		if err != nil {
			return "", err
		}

		opts := []chromedp.ExecAllocatorOption{
			chromedp.ExecPath(chromePath),
			chromedp.UserDataDir(userDataDir),
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
			chromedp.Flag("no-first-run", true),
			chromedp.Flag("no-default-browser-check", true),
			chromedp.Flag("disable-session-crashed-bubble", true),
			chromedp.NoSandbox,
		}

		allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
		defer allocCancel()

		var tabCancel func()
		tabCtx, tabCancel = chromedp.NewContext(allocCtx)
		defer tabCancel()

		// Graceful browser shutdown before the defers kill the process.
		defer func() {
			if err := chromedp.Run(tabCtx, browser.Close()); err != nil {
				if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
					fmt.Fprintf(os.Stderr, "⚠️ browser.Close() failed: %v\n", err)
				}
			}
		}()
	}

	response, finalURL, err := webchatSend(tabCtx, conversationURL, message, 0)
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
func webchatSend(tabCtx context.Context, conversationURL, message string, retry int) (string, string, error) {
	navURL := conversationURL
	if navURL == "" {
		navURL = deepseekChatURL
	}
	isNewConv := (conversationURL == "")

	if !isNewConv {
		fmt.Fprintf(os.Stderr, "📋 继续会话: %s\n", conversationURL)
	}

	var baseline, response, finalURL string

	// Base navigation and page hydration.
	actions := []chromedp.Action{
		chromedp.Navigate(navURL),
		chromedp.WaitReady("body"),
	}

	// For continuing conversations: wait longer for chat history to load.
	if !isNewConv {
		actions = append(actions,
			chromedp.Sleep(2*time.Second),
			// Wait for at least one .ds-markdown element (conversation loaded).
			chromedp.WaitVisible(".ds-markdown", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
	} else {
		actions = append(actions,
			chromedp.Sleep(3*time.Second),
		)
	}

	// Enable expert/deepthink mode (V4 Pro) for all conversations.
	// This improves review quality and response depth.
	actions = append(actions, chromedp.ActionFunc(func(ctx context.Context) error {
		var result map[string]any
		if err := chromedp.Evaluate(jsEnableDeepThink, &result).Do(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️ 专家模式启用失败: %v\n", err)
			return nil // non-fatal
		}
		if ok, _ := result["success"].(bool); ok {
			method, _ := result["method"].(string)
			switch method {
			case "data-model-type":
				mt, _ := result["modelType"].(string)
				fmt.Fprintf(os.Stderr, "🔬 已启用专家模式 (model=%s)\n", mt)
			default:
				fmt.Fprintln(os.Stderr, "🔬 已启用专家模式 (V4 Pro)")
			}
		}
		return nil
	}))
	// Pause for the toggle to take effect before textarea interaction.
	actions = append(actions, chromedp.Sleep(1*time.Second))

	actions = append(actions,
		// Record baseline text before sending.
		chromedp.Evaluate("document.body ? document.body.innerText : ''", &baseline),

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
			response, err = webchatWait(ctx, baseline)
			return err
		}),

		// Capture the final URL (contains conversation ID).
		chromedp.Location(&finalURL),
	)

	err := chromedp.Run(tabCtx, actions...)
	if err != nil {
		// If login is needed and we haven't retried yet, perform login
		// in the same Chrome session and retry once.
		if errors.Is(err, ErrLoginRequired) && retry == 0 {
			fmt.Fprintln(os.Stderr, "🔐 未登录，在浏览器窗口中完成登录...")
			if loginErr := deepseekLogin(tabCtx, "", nil, true); loginErr != nil {
				return "", "", fmt.Errorf("webchat login: %w", loginErr)
			}
			return webchatSend(tabCtx, conversationURL, message, retry+1)
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

// webchatWait polls until the assistant response stabilizes, then extracts
// it via the .ds-markdown element (preferred) or body-text diff (fallback).
func webchatWait(ctx context.Context, baseline string) (string, error) {
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
				// Preferred: extract from the last .ds-markdown element.
				// This naturally excludes UI chrome (search info,
				// toggle labels, footer text).
				if resp := getLastAssistantText(ctx); resp != "" {
					return resp, nil
				}
				// Fallback: diff body text against baseline, then
				// clean up known artifact patterns.
				return cleanBodyResponse(extractResponse(baseline, current)), nil
			}
		} else {
			stableCount = 0
		}
		lastText = current
	}

	return "", fmt.Errorf("response timeout after %d polls", webChatMaxPolls)
}

// getLastAssistantText returns the text of the last .ds-markdown element,
// or "" if the selector doesn't match (e.g. DeepSeek changed their DOM).
func getLastAssistantText(ctx context.Context) string {
	var text string
	if err := chromedp.Evaluate(jsGetLastAssistantText, &text).Do(ctx); err != nil {
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

// extractResponse computes the text added after baseline.
func extractResponse(baseline, current string) string {
	if len(current) > len(baseline) {
		return strings.TrimSpace(current[len(baseline):])
	}
	return ""
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

// WebChatLogin opens a visible Chrome browser for manual DeepSeek login.
// The user completes captcha/SMS in the browser window; cookies are saved
// to the shared Chrome profile for subsequent WebChat calls.
func WebChatLogin(ctx context.Context) error {
	return DeepSeekLoginChromeOpts(ctx, "", nil, true)
}
