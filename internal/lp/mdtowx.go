package lp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
)

const (
	quailyMdToWxURL = "https://quaily.com/tools/markdown-to-wx/"

	mdtowxPollInterval = 200 * time.Millisecond
	mdtowxMaxPolls     = 100 // 20s total

	// jsMdToWxSetContent sets the OverType editor textarea value via the
	// native setter (bypasses OverType's internal buffer) and dispatches
	// an input event to trigger OverType's change callback, which updates
	// Vue's `source` reactive property and triggers the conversion pipeline.
	//
	// The %s placeholder receives the JS-quoted markdown content.
	jsMdToWxSetContent = `(() => {
		const ta = document.querySelector('textarea');
		if (!ta) return {error: 'textarea not found'};
		const setter = Object.getOwnPropertyDescriptor(
			HTMLTextAreaElement.prototype, 'value'
		).set;
		setter.call(ta, %s);
		ta.dispatchEvent(new Event('input', {bubbles: true}));
		return {success: true};
	})()`

	// jsMdToWxOutput returns the innerHTML of div#output, or "" if not ready.
	jsMdToWxOutput = `(() => {
		const el = document.getElementById('output');
		if (!el || !el.innerHTML) return '';
		return el.innerHTML;
	})()`

	// jsMdToWxIsReady reports whether div#output exists with content.
	jsMdToWxIsReady = `(() => {
		const el = document.getElementById('output');
		return el !== null && el.innerHTML.length > 0;
	})()`
)

// MdToWx converts a markdown string to WeChat-compatible HTML via the
// quaily.com markdown-to-wx tool, using a local Chrome/Chromium browser.
//
// The returned HTML is a rich-text fragment (not a full document) suitable
// for pasting into the WeChat Official Account editor or for use as the
// body content in a WeChat draft.
func MdToWx(ctx context.Context, mdContent string) (string, error) {
	if mdContent == "" {
		return "", fmt.Errorf("empty markdown content")
	}

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
		chromedp.Flag("headless", "new"),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-session-crashed-bubble", true),
		chromedp.NoSandbox,
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	// Graceful browser shutdown before the defers kill the process.
	defer func() {
		if err := chromedp.Run(tabCtx, browser.Close()); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "⚠️ browser.Close() failed: %v\n", err)
			}
		}
	}()

	var htmlContent string

	err = chromedp.Run(tabCtx,
		// Navigate and wait for the page (Vue app + OverType editor) to hydrate.
		chromedp.Navigate(quailyMdToWxURL),
		chromedp.WaitReady("body"),
		chromedp.WaitVisible("textarea", chromedp.ByQuery),
		chromedp.Sleep(1*time.Second), // allow OverType to initialize

		// Inject the markdown content into the OverType textarea.
		chromedp.ActionFunc(func(ctx context.Context) error {
			return mdtowxSetContent(ctx, mdContent)
		}),

		// Wait for the rendered output and extract it.
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			htmlContent, err = mdtowxWaitOutput(ctx)
			return err
		}),
	)
	if err != nil {
		return "", fmt.Errorf("mdtowx: %w", err)
	}

	return htmlContent, nil
}

// mdtowxSetContent sets the OverType editor's textarea value via JavaScript,
// triggering the Vue reactivity chain: native setter → input event →
// OverType change callback → source watcher → renderWeChat().
func mdtowxSetContent(ctx context.Context, content string) error {
	quoted := quoteJS(content)
	var result map[string]any
	js := fmt.Sprintf(jsMdToWxSetContent, quoted)

	if err := chromedp.Evaluate(js, &result).Do(ctx); err != nil {
		return fmt.Errorf("set content: %w", err)
	}
	if errMsg, ok := result["error"].(string); ok {
		return fmt.Errorf("set content: %s", errMsg)
	}
	return nil
}

// mdtowxWaitOutput polls until div#output has content, then returns its HTML.
func mdtowxWaitOutput(ctx context.Context) (string, error) {
	for range mdtowxMaxPolls {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(mdtowxPollInterval):
		}

		var ready bool
		if err := chromedp.Evaluate(jsMdToWxIsReady, &ready).Do(ctx); err != nil {
			continue // transient error, retry
		}
		if !ready {
			continue
		}

		var html string
		if err := chromedp.Evaluate(jsMdToWxOutput, &html).Do(ctx); err != nil {
			return "", fmt.Errorf("read output: %w", err)
		}
		return html, nil
	}

	return "", fmt.Errorf("output timeout after %d polls (%.0fs)",
		mdtowxMaxPolls, float64(mdtowxMaxPolls)*mdtowxPollInterval.Seconds())
}
