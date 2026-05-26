package lp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// ErrLoginRequired is returned when the browser is not logged in to DeepSeek.
// Callers should trigger a visible login flow and retry.
var ErrLoginRequired = errors.New("login required — open visible browser to complete login")

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
)

// WebChat sends a message to chat.deepseek.com via a visible Chrome browser
// and returns the assistant's text response.
//
// It uses the same Chrome user data directory as DeepSeekLoginChromeOpts,
// so cookies from a prior login are automatically available. If not logged
// in, ErrLoginRequired is returned — the caller should trigger a visible
// login flow (WebChatLogin) and retry.
//
// Usage:
//
//	response, err := lp.WebChat(ctx, "hello")
//	if errors.Is(err, lp.ErrLoginRequired) {
//	    lp.WebChatLogin(ctx)  // opens visible browser for manual login
//	    response, err = lp.WebChat(ctx, "hello")  // retry
//	}
func WebChat(ctx context.Context, message string) (string, error) {
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
		chromedp.NoSandbox,
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	var baseline, response string

	err = chromedp.Run(tabCtx,
		// Navigate and wait for the SPA to hydrate.
		chromedp.Navigate(deepseekChatURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(3*time.Second),

		// Record baseline text before sending.
		chromedp.Evaluate("document.body ? document.body.innerText : ''", &baseline),

		// Set the textarea value (JS needed for React-controlled inputs).
		chromedp.ActionFunc(func(ctx context.Context) error {
			return webchatSetValue(ctx, message)
		}),

		// Brief delay then press Enter to send.
		chromedp.Sleep(500*time.Millisecond),
		chromedp.KeyEvent("\r"),

		// Wait for and extract the assistant response.
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			response, err = webchatWait(ctx, baseline)
			return err
		}),
	)
	if err != nil {
		return "", fmt.Errorf("webchat: %w", err)
	}

	return response, nil
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

// webchatWait polls the page text until the assistant response stabilizes.
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
				return extractResponse(baseline, current), nil
			}
		} else {
			stableCount = 0
		}
		lastText = current
	}

	return "", fmt.Errorf("response timeout after %d polls", webChatMaxPolls)
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

// WebChatLogin opens a visible Chrome browser for manual DeepSeek login.
// The user completes captcha/SMS in the browser window; cookies are saved
// to the shared Chrome profile for subsequent WebChat calls.
func WebChatLogin(ctx context.Context) error {
	return DeepSeekLoginChromeOpts(ctx, "", nil, true)
}