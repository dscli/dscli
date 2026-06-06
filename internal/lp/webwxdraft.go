// Package lp provides web page reading via lightpanda browser with CDP.
package lp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

// WeChatDraftParams holds parameters for creating a WeChat draft.
type WeChatDraftParams struct {
	// HTMLPath is the path to the local HTML file to publish.
	HTMLPath string

	// Title is the article title.
	Title string

	// Author is the article author name.
	Author string

	// Debug enables page inspection output for troubleshooting selectors.
	Debug bool

	// Timeout for the whole operation.
	Timeout time.Duration
}

// ImageRef describes an image referenced in the HTML.
type ImageRef struct {
	// Src is the src attribute value from the img tag.
	Src string

	// AbsPath is the resolved absolute file path on disk.
	AbsPath string
}

// pageState represents the current WeChat MP page state.
type pageState int

const (
	stateUnknown       pageState = iota
	stateLogin                   // on the login page (QR code / password form)
	stateAccountSelect           // multiple account selection page
	stateDashboard               // logged in, on the main dashboard
	stateDraftList               // on the draft list page
	stateEditor                  // on the article editor page
)

// ---------------------------------------------------------------------------
// Existing helpers (extractImages, extractAttr, resolveImagePath, bodyContent,
// quoteJSString) — unchanged from the original.
// ---------------------------------------------------------------------------

// extractImages uses simple string scanning to find all <img> tags in the
// HTML content and resolve their src attributes to absolute paths relative
// to htmlBaseDir.
//
// TODO: replace with tree-sitter HTML once python-tree-sitter-html is ready.
func extractImages(bodyHTML string, htmlBaseDir string) []ImageRef {
	var images []ImageRef
	seen := make(map[string]bool) // dedup by resolved absolute path

	remaining := bodyHTML

	for {
		// Find the next <img tag (case-insensitive).
		imgIdx := strings.Index(strings.ToLower(remaining), "<img")
		if imgIdx == -1 {
			break
		}
		// Find the closing > of this tag.
		rest := remaining[imgIdx+4:]
		closeIdx := strings.IndexByte(rest, '>')
		if closeIdx == -1 {
			break
		}
		tagContent := rest[:closeIdx]

		// Extract src="..." from the tag content.
		src := extractAttr(tagContent, "src")
		if src != "" && !seen[src] {
			seen[src] = true
			absPath := resolveImagePath(src, htmlBaseDir)
			if absPath != "" {
				images = append(images, ImageRef{
					Src:     src,
					AbsPath: absPath,
				})
			}
		}

		remaining = rest[closeIdx+1:]
	}
	return images
}

// extractAttr extracts a quoted attribute value from HTML tag content.
// It handles both single and double quotes.
func extractAttr(tagContent, attrName string) string {
	lower := strings.ToLower(tagContent)
	// Search for attrName="...
	search := attrName + "="
	idx := strings.Index(lower, search)
	if idx == -1 {
		return ""
	}
	after := tagContent[idx+len(search):]
	if len(after) == 0 {
		return ""
	}
	quote := after[0]
	if quote != '"' && quote != '\'' {
		return ""
	}
	end := strings.IndexByte(after[1:], quote)
	if end == -1 {
		return ""
	}
	return after[1 : end+1]
}

// resolveImagePath resolves a potentially relative image src to an absolute path.
func resolveImagePath(src, baseDir string) string {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") ||
		strings.HasPrefix(src, "//") || strings.HasPrefix(src, "data:") {
		return "" // remote or inline, skip
	}
	// Clean the path and resolve relative to the HTML file's directory.
	cleaned := filepath.Clean(src)
	if filepath.IsAbs(cleaned) {
		return cleaned
	}
	abs := filepath.Join(baseDir, cleaned)
	if _, err := os.Stat(abs); err == nil {
		return abs
	}
	// Try basename in baseDir as fallback.
	base := filepath.Base(cleaned)
	abs = filepath.Join(baseDir, base)
	if _, err := os.Stat(abs); err == nil {
		return abs
	}
	return ""
}

// bodyContent extracts the <body> innerHTML from a full HTML document.
// If the content is already a fragment (no <body> tag), returns it as-is.
func bodyContent(htmlContent string) string {
	lower := strings.ToLower(htmlContent)
	bodyStart := strings.Index(lower, "<body")
	if bodyStart == -1 {
		// No body tag — treat as a fragment.
		return htmlContent
	}
	// Find the start of body content: after the opening > of <body ...>
	gt := strings.IndexByte(htmlContent[bodyStart:], '>')
	if gt == -1 {
		return htmlContent
	}
	contentStart := bodyStart + gt + 1

	bodyEnd := strings.LastIndex(lower, "</body>")
	if bodyEnd == -1 || bodyEnd <= contentStart {
		return htmlContent[contentStart:]
	}
	return htmlContent[contentStart:bodyEnd]
}

// quoteJSString escapes a string for use as a JavaScript string literal.
func quoteJSString(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "\\r")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return `"` + s + `"`
}

// ---------------------------------------------------------------------------
// Page inspection (debug mode)
// ---------------------------------------------------------------------------

// inspectEditorPage probes the current page and prints useful information
// about DOM elements found for the key operations (title, content, image).
func inspectEditorPage(ctx context.Context) {
	fmt.Fprintf(os.Stderr, "🔍 编辑器页面探查:\n")

	type probe struct {
		label string
		js    string
	}
	probes := []probe{
		// Redact token from URL to avoid leaking session credentials.
		{"页面 URL", `window.location.href.replace(/token=\d+/, 'token=***')`},
		{"文档标题", `document.title`},
		{"标题输入框", `(() => {
			const candidates = [
				'input#title', 'div#js_editor_title', '.title_input',
				'div.editor-title', '[data-role="title"]', '.appmsg_title_input',
				'.title-input', 'h1.appmsg_title', 'div.title_wrp input',
				'div.appmsg_title', 'div.title', '#activity-title',
				'[contenteditable]', 'div.rich_media_area',
			];
			const results = [];
			for (const sel of candidates) {
				const el = document.querySelector(sel);
				if (el) {
					const tag = el.tagName;
					const attrs = el.getAttribute('contenteditable') ? 'contenteditable' :
						(el.className ? 'class=' + el.className.slice(0,40) : '');
					const text = (el.textContent || '').slice(0, 30);
					results.push(sel + ' → <' + tag + ' ' + attrs + '> "' + text + '"');
				}
			}
			return results.length > 0 ? results.join('\\n') : '(无匹配)';
		})()`},
		{"作者输入框", `(() => {
			const candidates = [
				'input#author', 'input[name="author"]', '[data-role="author"]',
				'.appmsg_author_input', 'a.author_edit', '.author_edit_btn',
				'div.author input', '.wx_dialog input',
			];
			const results = [];
			for (const sel of candidates) {
				const el = document.querySelector(sel);
				if (el) { results.push(sel + ' → <' + el.tagName + '>'); }
			}
			return results.length > 0 ? results.join('\\n') : '(无匹配)';
		})()`},
		{"编辑器内容区", `(() => {
			const candidates = [
				'iframe#ueditor_0', '[contenteditable="true"]',
				'div.rich_media_area', 'div.editor-content',
				'div.rich_media', '.ProseMirror', '.tiptap',
				'div.editor', '.rich_media_content',
				'[data-role="content"]', '.appmsg_content',
				'#js_content', 'div.rich_media_area_primary',
			];
			const results = [];
			for (const sel of candidates) {
				const el = document.querySelector(sel);
				if (el) {
					const tag = el.tagName;
					const info = el.id ? '#' + el.id : (el.className ? '.' + el.className.slice(0,30) : '');
					results.push(sel + ' → <' + tag + info + '>');
				}
			}
			return results.length > 0 ? results.join('\\n') : '(无匹配)';
		})()`},
		{"图片上传按钮", `(() => {
			const candidates = [
				'.edui-for-image .edui-button-body', '[title="图片"]',
				'[title="image"]', '.toolbar-btn', '.editor-toolbar button',
				'[data-role="add-image"]', '.add_image', '.js_add_image',
				'div.toolbar a', 'div.toolbar button',
			];
			const imageBtns = [];
			for (const sel of candidates) {
				const els = document.querySelectorAll(sel);
				if (els.length > 0) {
					for (const el of els) {
						const text = (el.textContent || '').trim().slice(0,20);
						imageBtns.push(sel + ' → <' + el.tagName + '> "' + text + '"');
					}
				}
			}
			// Also look for any button with image-related text
			const allBtns = document.querySelectorAll('a, button, span, li, div');
			for (const b of allBtns) {
				const t = (b.textContent || '').trim();
				if ((t === '图片' || t.indexOf('图片') !== -1 || t === 'image') && imageBtns.length < 20) {
					imageBtns.push('[text="' + t + '"] → <' + b.tagName + '>');
				}
			}
			return imageBtns.length > 0 ? imageBtns.join('\\n') : '(无匹配)';
		})()`},
		{"文件上传input", `(() => {
			const inputs = document.querySelectorAll('input[type="file"]');
			return inputs.length > 0 ? '找到 ' + inputs.length + ' 个 file input' : '(无)';
		})()`},
		{"保存按钮", `(() => {
			const candidates = [
				'a.btn_primary', '.toolbar-save', '.js_save',
				'[data-role="save"]', 'a, button',
			];
			const saves = [];
			for (const sel of candidates) {
				const els = document.querySelectorAll(sel);
				for (const el of els) {
					const t = (el.textContent || '').trim();
					if (t.indexOf('保存') !== -1 || t === '发布' || t === 'Submit') {
						saves.push('<' + el.tagName + '> "' + t + '"');
					}
				}
			}
			return saves.length > 0 ? saves.join('\\n') : '(无匹配)';
		})()`},
	}

	for _, p := range probes {
		var result string
		if err := chromedp.Run(ctx, chromedp.Evaluate(p.js, &result)); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: ❌ %v\n", p.label, err)
			continue
		}
		fmt.Fprintf(os.Stderr, "  %s:\n", p.label)
		for _, line := range strings.Split(result, "\n") {
			if line != "" {
				fmt.Fprintf(os.Stderr, "    %s\n", line)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Page-state detection
// ---------------------------------------------------------------------------

// detectPageState evaluates the current page to determine what state we're in.
func detectPageState(ctx context.Context) pageState {
	type check struct {
		state pageState
		js    string
	}
	checks := []check{
		{
			stateEditor,
			`(() => {
				const url = window.location.href;
				if (url.indexOf('appmsg_edit') !== -1) return true;
				const signals = [
					'iframe#ueditor_0', '[contenteditable="true"]',
					'div#js_editor_title', 'input#title',
					'div.rich_media_area', 'div.editor-content',
					'div.editor', '.rich_media',
					'.rich_media_content', '.rich_media_area_primary',
				];
				for (const s of signals) {
					if (document.querySelector(s)) return true;
				}
				if (typeof window.ue !== 'undefined' && window.ue) return true;
				return false;
			})()`,
		},
		{
			stateDraftList,
			`(() => {
				const url = window.location.href;
				if (url.indexOf('appmsg_list') !== -1 ||
				    url.indexOf('appmsg?') !== -1) return true;
				if (document.querySelector('.appmsg_list') ||
				    document.querySelector('.table_wrp')) return true;
				return false;
			})()`,
		},
		{
			stateAccountSelect,
			`(() => {
				const el = document.querySelector('.account_list_box');
				if (el && el.querySelectorAll('.account_item').length > 0) return true;
				return false;
			})()`,
		},
		{
			stateDashboard,
			`(() => {
				const url = window.location.href;
				if (url.indexOf('token=') !== -1 &&
				    url.indexOf('appmsg') === -1) return true;
				if (document.querySelector('#menuBar')) return true;
				if (document.querySelector('.nav_item')) return true;
				if (document.querySelector('.weui-desktop-account')) return true;
				return false;
			})()`,
		},
		{
			stateLogin,
			`(() => {
				if (document.querySelector('.login_frame')) return true;
				if (document.querySelector('.login__type__container')) return true;
				if (document.querySelector('#loginWrap')) return true;
				if (window.location.href.indexOf('token=') === -1 &&
				    !document.querySelector('#menuBar')) return true;
				return false;
			})()`,
		},
	}

	for _, c := range checks {
		var found bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(c.js, &found)); err != nil {
			continue
		}
		if found {
			return c.state
		}
	}
	return stateUnknown
}

// ---------------------------------------------------------------------------
// Login waiting & editor navigation
// ---------------------------------------------------------------------------

// waitForLogin polls until the user scans the QR code and the dashboard
// (or editor) appears.
func waitForLogin(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "⏳ 等待扫码登录...\n")

	pollInterval := 2 * time.Second
	maxPolls := 150 // 5 minutes

	for range maxPolls {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		state := detectPageState(ctx)
		switch state {
		case stateEditor:
			return nil
		case stateDashboard, stateDraftList:
			return nil
		case stateAccountSelect:
			fmt.Fprintf(os.Stderr, "  检测到多账户选择页，自动选择第一个账户...\n")
			trySelectFirstAccount(ctx)
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
		}
	}

	return fmt.Errorf("扫码登录超时（5分钟）")
}

// trySelectFirstAccount clicks the first account in the account selection page.
func trySelectFirstAccount(ctx context.Context) {
	script := `(() => {
		const items = document.querySelectorAll('.account_item, .account_list a');
		if (items.length > 0) { items[0].click(); return true; }
		return false;
	})()`
	var ok bool
	_ = chromedp.Run(ctx, chromedp.Evaluate(script, &ok))
}

// navigateToEditor attempts to reach the article editor page from the
// current state (dashboard or draft list).
func navigateToEditor(ctx context.Context) error {
	return navigateToEditorDepth(ctx, 0)
}

// navigateToEditorDepth is the recursive implementation with a depth guard
// to prevent infinite recursion if the page never reaches stateEditor.
func navigateToEditorDepth(ctx context.Context, depth int) error {
	if depth > 3 {
		return fmt.Errorf("导航递归深度超限 (超过3次重试)")
	}

	// --- Strategy A: Direct URL navigation with token extraction ---
	var token string
	if err := chromedp.Run(ctx, chromedp.Evaluate(
		`(() => {
			const m = window.location.href.match(/token=(\d+)/);
			return m ? m[1] : '';
		})()`, &token)); err == nil && token != "" {

		editorURL := fmt.Sprintf(
			"https://mp.weixin.qq.com/cgi-bin/appmsg?t=media/appmsg_edit_v2&action=edit&lang=zh_CN&token=%s&type=77",
			token,
		)
		fmt.Fprintf(os.Stderr, "  导航到编辑器 URL...\n")
		if err := chromedp.Run(ctx,
			chromedp.Navigate(editorURL),
			chromedp.WaitReady("body"),
			chromedp.Sleep(3*time.Second),
		); err == nil && detectPageState(ctx) == stateEditor {
			return nil
		}
	}

	// --- Strategy B: Click through the sidebar (draft list page) ---
	if detectPageState(ctx) == stateDraftList {
		createScripts := []string{
			`(() => {
				const btns = document.querySelectorAll('a, button, span, div');
				for (const b of btns) {
					const t = b.textContent.trim();
					if (t === '新的创作' || t === '新建' || t.indexOf('新创作') !== -1) {
						b.click(); return true;
					}
				}
				return false;
			})()`,
			`(() => {
				const btns = document.querySelectorAll('.new_appmsg, .js_new_appmsg, a[href*="edit"]');
				for (const b of btns) {
					b.click(); return true;
				}
				return false;
			})()`,
		}
		for _, s := range createScripts {
			var ok bool
			if chromedp.Run(ctx, chromedp.Evaluate(s, &ok)); ok {
				chromedp.Run(ctx, chromedp.Sleep(2 * time.Second))
				var clicked bool
				_ = chromedp.Run(ctx, chromedp.Evaluate(
					`(() => {
						const items = document.querySelectorAll('a, li, span');
						for (const el of items) {
							if (el.textContent.trim() === '文章') {
								el.click(); return true;
							}
						}
						return false;
					})()`, &clicked))
				chromedp.Run(ctx, chromedp.Sleep(3 * time.Second))
				if detectPageState(ctx) == stateEditor {
					return nil
				}
			}
		}
	}

	// --- Strategy C: Click sidebar nav to get to draft list ---
	navScripts := []string{
		`(() => {
			const items = document.querySelectorAll('a, span, li, div');
			for (const el of items) {
				const t = el.textContent.trim();
				if (t === '草稿箱' || t.indexOf('草稿') !== -1) {
					el.click(); return true;
				}
			}
			return false;
		})()`,
		`(() => {
			const items = document.querySelectorAll('a, span, li, div');
			for (const el of items) {
				const t = el.textContent.trim();
				if (t.indexOf('内容管理') !== -1 || t.indexOf('素材管理') !== -1) {
					el.click(); return true;
				}
			}
			return false;
		})()`,
		`(() => {
			const links = document.querySelectorAll('a[href*="appmsg"]');
			if (links.length > 0) { links[0].click(); return true; }
			return false;
		})()`,
	}
	for _, s := range navScripts {
		var ok bool
		if chromedp.Run(ctx, chromedp.Evaluate(s, &ok)); ok {
			chromedp.Run(ctx, chromedp.Sleep(3 * time.Second))
			chromedp.Run(ctx, chromedp.WaitReady("body"))
			return navigateToEditorDepth(ctx, depth+1)
		}
	}

	return fmt.Errorf("无法自动导航到草稿编辑器")
}

// waitForEditor polls until the editor page is fully loaded and ready.
func waitForEditor(ctx context.Context) error {
	fmt.Fprintf(os.Stderr, "⏳ 等待编辑器加载...\n")

	pollInterval := 1 * time.Second
	maxPolls := 30 // 30 seconds

	for range maxPolls {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		if detectPageState(ctx) == stateEditor {
			chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
			return nil
		}
	}
	return fmt.Errorf("编辑器加载超时")
}

// ---------------------------------------------------------------------------
// Editor interaction (title, author, content, images, save)
// ---------------------------------------------------------------------------

// setWxTitle sets the article title in the WeChat editor.
func setWxTitle(ctx context.Context, title string) error {
	quoted := quoteJSString(title)

	scripts := []string{
		// Classic selectors
		fmt.Sprintf(`(() => {
			const el = document.querySelector('input#title');
			if (el) { el.value = %s; el.dispatchEvent(new Event('input')); return true; }
			return false;
		})()`, quoted),
		fmt.Sprintf(`(() => {
			const el = document.querySelector('div#js_editor_title');
			if (el) { el.textContent = %s; return true; }
			return false;
		})()`, quoted),
		fmt.Sprintf(`(() => {
			const el = document.querySelector('.title_input');
			if (el) { el.value = %s; el.dispatchEvent(new Event('input')); return true; }
			return false;
		})()`, quoted),
		fmt.Sprintf(`(() => {
			const el = document.querySelector('div.editor-title');
			if (el) { el.textContent = %s; return true; }
			return false;
		})()`, quoted),
		// New editor selectors (2024+)
		fmt.Sprintf(`(() => {
			const el = document.querySelector('[data-role="title"], .appmsg_title_input, .title-input');
			if (el) {
				if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') {
					el.value = %s;
				} else {
					el.textContent = %s;
				}
				el.dispatchEvent(new Event('input', {bubbles: true}));
				return true;
			}
			return false;
		})()`, quoted, quoted),
		// Current WeChat editor (2025+): div.title with class js_title_main
		fmt.Sprintf(`(() => {
			const el = document.querySelector('div.title, div.js_title_main, .appmsg_edit_item.title, div[class*="title"][contenteditable]');
			if (el) {
				const ce = el.querySelector('[contenteditable]');
				const target = ce || el;
				target.textContent = %s;
				target.dispatchEvent(new Event('input', {bubbles: true}));
				return true;
			}
			return false;
		})()`, quoted),
		// Try div.title_wrp input
		fmt.Sprintf(`(() => {
			const el = document.querySelector('div.title_wrp input, div.title_wrp [contenteditable]');
			if (el) {
				if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') { el.value = %s; }
				else { el.textContent = %s; }
				el.dispatchEvent(new Event('input', {bubbles: true})); return true;
			}
			return false;
		})()`, quoted, quoted),
		// Try the first contenteditable element that looks like a title (short content)
		fmt.Sprintf(`(() => {
			const allCE = document.querySelectorAll('[contenteditable="true"]');
			for (const el of allCE) {
				const html = el.innerHTML.toLowerCase();
				if (html.length > 50 && (html.indexOf('<p') !== -1 || html.indexOf('<div') !== -1)) continue;
				el.textContent = %s;
				el.dispatchEvent(new Event('input', {bubbles: true}));
				return true;
			}
			return false;
		})()`, quoted),
		// Try any element with placeholder containing "标题" or "title"
		fmt.Sprintf(`(() => {
			const all = document.querySelectorAll('[placeholder]');
			for (const el of all) {
				const p = el.getAttribute('placeholder') || '';
				if (p.indexOf('标题') !== -1 || p.indexOf('title') !== -1 || p.indexOf('Title') !== -1) {
					if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') { el.value = %s; }
					else { el.textContent = %s; }
					el.dispatchEvent(new Event('input', {bubbles: true})); return true;
				}
			}
			return false;
		})()`, quoted, quoted),
		// Try h1 elements
		fmt.Sprintf(`(() => {
			const el = document.querySelector('h1.appmsg_title, h1.title, .appmsg_title');
			if (el) { el.textContent = %s; return true; }
			return false;
		})()`, quoted),
	}

	for _, script := range scripts {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &ok)); err != nil {
			continue
		}
		if ok {
			return nil
		}
	}

	return fmt.Errorf("未找到标题输入框")
}

// setWxAuthor sets the author field in the WeChat editor.
func setWxAuthor(ctx context.Context, author string) error {
	quoted := quoteJSString(author)

	scripts := []string{
		fmt.Sprintf(`(() => {
			const el = document.querySelector('input#author');
			if (el) { el.value = %s; el.dispatchEvent(new Event('input')); return true; }
			return false;
		})()`, quoted),
		fmt.Sprintf(`(() => {
			const el = document.querySelector('input[name="author"]');
			if (el) { el.value = %s; el.dispatchEvent(new Event('input')); return true; }
			return false;
		})()`, quoted),
		fmt.Sprintf(`(() => {
			const el = document.querySelector('[data-role="author"], .appmsg_author_input');
			if (el) {
				if (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA') { el.value = %s; }
				else { el.textContent = %s; }
				el.dispatchEvent(new Event('input', {bubbles: true})); return true;
			}
			return false;
		})()`, quoted, quoted),
		fmt.Sprintf(`(() => {
			const el = document.querySelector('div.author input, .author_wrp input, [data-role="author"] input');
			if (el) { el.value = %s; el.dispatchEvent(new Event('input', {bubbles: true})); return true; }
			return false;
		})()`, quoted),
		`(() => {
			const btn = document.querySelector('a.author_edit, .author_edit_btn, [data-role="author-edit"]');
			if (btn) { btn.click(); return 'clicked'; }
			return false;
		})()`,
	}

	for _, script := range scripts {
		var result any
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &result)); err != nil {
			continue
		}
		switch v := result.(type) {
		case bool:
			if v {
				return nil
			}
		case string:
			if v == "clicked" {
				chromedp.Run(ctx, chromedp.Sleep(1 * time.Second))
				// If the dialog path fails, try remaining scripts instead of aborting.
				if err := setWxAuthorInput(ctx, author); err == nil {
					return nil
				}
				// Dialog input failed — continue to next script
			}
		}
	}

	return nil // non-fatal, user can set manually
}

// setWxAuthorInput sets the author input field after the dialog is opened.
func setWxAuthorInput(ctx context.Context, author string) error {
	quoted := quoteJSString(author)
	script := fmt.Sprintf(`(() => {
		const el = document.querySelector('div.wx_dialog input, .dialog input, [data-role="author-dialog"] input');
		if (el) {
			el.value = %s; el.dispatchEvent(new Event('input'));
			const confirm = document.querySelector(
				'div.wx_dialog a.btn_primary, .dialog .btn_primary, .dialog_confirm_btn, [node-type="ok"]'
			);
			if (confirm) { confirm.click(); }
			return true;
		}
		return false;
	})()`, quoted)

	var ok bool
	return chromedp.Run(ctx, chromedp.Evaluate(script, &ok))
}

// setWxContent sets the body HTML content in the WeChat editor.
func setWxContent(ctx context.Context, contentHTML string) error {
	quoted := quoteJSString(contentHTML)

	scripts := []string{
		// Pattern 1: UEditor iframe (ueditor_0)
		fmt.Sprintf(`(() => {
			const iframe = document.querySelector('iframe#ueditor_0');
			if (iframe) {
				const doc = iframe.contentDocument || iframe.contentWindow.document;
				doc.body.innerHTML = %s;
				return true;
			}
			return false;
		})()`, quoted),
		// Pattern 2: Any iframe with contenteditable body
		fmt.Sprintf(`(() => {
			const iframes = document.querySelectorAll('iframe');
			for (const f of iframes) {
				try {
					const doc = f.contentDocument || f.contentWindow.document;
					if (doc && doc.body && doc.body.contentEditable === 'true') {
						doc.body.innerHTML = %s;
						return true;
					}
				} catch(e) {}
			}
			return false;
		})()`, quoted),
		// Pattern 3: Direct contenteditable div (skip title-like elements)
		fmt.Sprintf(`(() => {
			const els = document.querySelectorAll('[contenteditable="true"]');
			for (const el of els) {
				const tag = el.tagName.toLowerCase();
				const cls = (el.className || '').toLowerCase();
				const id = (el.id || '').toLowerCase();
				const role = (el.getAttribute('data-role') || '').toLowerCase();
				// Skip title-like elements — title div is contenteditable and comes first in DOM
				if (id.indexOf('title') !== -1 || cls.indexOf('title') !== -1 || role === 'title') continue;
				// Skip standalone input elements (title or author field)
				if (tag === 'input') continue;
				el.innerHTML = %s;
				el.dispatchEvent(new Event('input', {bubbles: true}));
				return true;
			}
			return false;
		})()`, quoted),

		// Pattern 4: rich_media_area
		fmt.Sprintf(`(() => {
			const el = document.querySelector('div.rich_media_area, .rich_media_area_primary');
			if (el) {
				el.innerHTML = %s;
				return true;
			}
			return false;
		})()`, quoted),
		// Pattern 5: window.ue API
		fmt.Sprintf(`(() => {
			if (typeof window.ue !== 'undefined') {
				const editor = window.ue.getEditor();
				if (editor) {
					editor.setContent(%s);
					return true;
				}
			}
			return false;
		})()`, quoted),
		// Pattern 6: Common content selectors
		fmt.Sprintf(`(() => {
			const selectors = [
				'div.rich_media', '.rich_media_content',
				'[data-role="content"]', '.appmsg_content',
				'#js_content', '.js_rich_media_content',
			];
			for (const sel of selectors) {
				const el = document.querySelector(sel);
				if (el) {
					el.innerHTML = %s;
					return true;
				}
			}
			return false;
		})()`, quoted),
		// Pattern 7: ProseMirror / TipTap / contenteditable fallback (skip title elements)
		fmt.Sprintf(`(() => {
			const editors = document.querySelectorAll('.ProseMirror, .tiptap, [contenteditable]');
			for (const editor of editors) {
				const cls = (editor.className || '').toLowerCase();
				const id = (editor.id || '').toLowerCase();
				const role = (editor.getAttribute('data-role') || '').toLowerCase();
				// Skip title-like elements
				if (id.indexOf('title') !== -1 || cls.indexOf('title') !== -1 || role === 'title') continue;
				editor.innerHTML = %s;
				editor.dispatchEvent(new Event('input', {bubbles: true}));
				return true;
			}
			return false;
		})()`, quoted),
	}


	for _, script := range scripts {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &ok)); err != nil {
			continue
		}
		if ok {
			return nil
		}
	}

	return fmt.Errorf("未找到编辑器区域")
}

// uploadWxImage uploads a single image to WeChat's media library.
func uploadWxImage(ctx context.Context, imagePath string) error {
	// Step 1: Try clicking the image toolbar button.
	imageBtnScripts := []string{
		// UEditor toolbar: image button
		`(() => {
			const btn = document.querySelector('.edui-for-image .edui-button-body');
			if (btn) { btn.click(); return true; }
			return false;
		})()`,
		// Toolbar button with title="图片"
		`(() => {
			const btn = document.querySelector('[title="图片"], [title="image"], [data-title="图片"]');
			if (btn) { btn.click(); return true; }
			return false;
		})()`,
		// Toolbar buttons with text content
		`(() => {
			const btns = document.querySelectorAll('.toolbar-btn, .editor-toolbar button, .toolbar button, [data-role="toolbar"] button');
			for (const b of btns) {
				const t = b.textContent.trim();
				if (t === '图片' || t.indexOf('图片') !== -1 || t === 'image') {
					b.click(); return true;
				}
			}
			return false;
		})()`,
		// Current WeChat editor (2025+): scan ALL elements for exact text "图片"
		`(() => {
			const all = document.querySelectorAll('a, button, span, li, div, i, em');
			for (const el of all) {
				if (el.textContent.trim() === '图片') {
					el.click(); return true;
				}
			}
			return false;
		})()`,
		// Toolbar: look for span with text "图片"
		`(() => {
			const spans = document.querySelectorAll('span');
			for (const s of spans) {
				if (s.textContent.trim() === '图片') {
					s.click(); return true;
				}
			}
			return false;
		})()`,
		// Try clicking "+" add button
		`(() => {
			const btns = document.querySelectorAll('[data-role="add-image"], .add_image, .js_add_image, .image_add, .insert_image');
			for (const b of btns) {
				b.click(); return true;
			}
			return false;
		})()`,
		// New WeChat editor: inline toolbar - look for image icon class
		`(() => {
			const icons = document.querySelectorAll('i[class*="image"], i[class*="picture"], span[class*="image"], span[class*="picture"]');
			for (const icon of icons) {
				const btn = icon.closest('a, button, span, div');
				if (btn && btn !== icon) { btn.click(); return true; }
				icon.click(); return true;
			}
			return false;
		})()`,
		// WeChat new editor: floating toolbar with "+" button
		`(() => {
			const btns = document.querySelectorAll('.add-btn, .js_add, .editor-add, [data-action="add"]');
			for (const b of btns) {
				b.click(); return true;
			}
			return false;
		})()`,
		// Toolbar inside an iframe
		`(() => {
			const iframes = document.querySelectorAll('iframe');
			for (const f of iframes) {
				try {
					const doc = f.contentDocument || f.contentWindow.document;
					if (!doc) continue;
					const btns = doc.querySelectorAll('[title="图片"], [title="image"], .toolbar-btn');
					for (const b of btns) {
						b.click(); return true;
					}
				} catch(e) {}
			}
			return false;
		})()`,
	}

	var dialogOpened bool
	for _, script := range imageBtnScripts {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &ok)); err != nil {
			continue
		}
		if ok {
			dialogOpened = true
			break
		}
	}

	if !dialogOpened {
		return fmt.Errorf("未找到图片上传按钮")
	}

	// Step 2: Wait for the image dialog / popup.
	chromedp.Run(ctx, chromedp.Sleep(2*time.Second))

	// Step 2.5: Click "本地上传" (local upload) option in the image dialog.
	localUploadScripts := []string{
		// Scan all elements for exact text "本地上传"
		`(() => {
			const all = document.querySelectorAll('a, button, span, li, div');
			for (const el of all) {
				if (el.textContent.trim() === '本地上传') {
					el.click(); return true;
				}
			}
			return false;
		})()`,
		// Try inside a dialog container
		`(() => {
			const dialog = document.querySelector('.wx_dialog, .dialog, [role="dialog"], .upload_dialog');
			if (!dialog) return false;
			const all = dialog.querySelectorAll('a, button, span, li, div');
			for (const el of all) {
				if (el.textContent.trim() === '本地上传') {
					el.click(); return true;
				}
			}
			return false;
		})()`,
		// Try tab/panel with text "本地上传"
		`(() => {
			const tabs = document.querySelectorAll('.tab, [data-role="tab"], .upload_tab, .tab_item, .js_tab');
			for (const tab of tabs) {
				if (tab.textContent.trim() === '本地上传') {
					tab.click(); return true;
				}
			}
			return false;
		})()`,
		// Try label/span with text "本地上传"
		`(() => {
			const labels = document.querySelectorAll('label, span, em, strong');
			for (const el of labels) {
				if (el.textContent.trim() === '本地上传') {
					el.click(); return true;
				}
			}
			return false;
		})()`,
	}

	var localClicked bool
	for _, script := range localUploadScripts {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &ok)); err != nil {
			continue
		}
		if ok {
			localClicked = true
			break
		}
	}

	if !localClicked {
		fmt.Fprintf(os.Stderr, "   ⚠️  未找到「本地上传」按钮，尝试直接查找文件输入框...\n")
	}
	chromedp.Run(ctx, chromedp.Sleep(1*time.Second))

	// Step 3: Find and set the file input.
	fileSelectors := []string{
		`input[type="file"]`,
		`div.upload_file_box input[type="file"]`,
		`div.wx_dialog input[type="file"]`,
		`.dialog input[type="file"]`,
		`[data-role="file-input"] input[type="file"]`,
		`div.upload_box input[type="file"]`,
		`div.media_upload input[type="file"]`,
	}

	var fileUploaded bool
	for _, sel := range fileSelectors {
		var found bool
		if err := chromedp.Run(ctx,
			chromedp.Evaluate(fmt.Sprintf(`document.querySelector('%s') !== null`, sel), &found),
		); err != nil || !found {
			continue
		}

		if err := chromedp.Run(ctx,
			chromedp.SetUploadFiles(sel, []string{imagePath}, chromedp.ByQuery),
			chromedp.Sleep(3*time.Second),
		); err == nil {
			fileUploaded = true
			break
		}
	}

	if !fileUploaded {
		fmt.Fprintf(os.Stderr, "   ⚠️  请手动上传图片: %s\n", imagePath)
	}

	// Step 4: Wait for upload completion.
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))

	// Step 5: Try to click confirm button.
	confirmScripts := []string{
		`(() => {
			const btn = document.querySelector('a.btn_primary');
			if (btn && btn.textContent.includes('确定')) { btn.click(); return true; }
			return false;
		})()`,
		`(() => {
			const btn = document.querySelector('.dialog_confirm_btn, .confirm-btn, [data-role="confirm"]');
			if (btn) { btn.click(); return true; }
			return false;
		})()`,
		`(() => {
			const btn = document.querySelector('[node-type="ok"], [node-type="save"]');
			if (btn) { btn.click(); return true; }
			return false;
		})()`,
		`(() => {
			const btn = document.querySelector('a.btn_primary');
			if (btn && btn.textContent.includes('插入')) { btn.click(); return true; }
			return false;
		})()`,
		// New editor: "完成" or "确认" button
		`(() => {
			const btns = document.querySelectorAll('a, button');
			for (const b of btns) {
				const t = b.textContent.trim();
				if (t === '完成' || t === '确认' || t === '确定') { b.click(); return true; }
			}
			return false;
		})()`,
	}

	var confirmed bool
	for _, script := range confirmScripts {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &ok)); err != nil {
			continue
		}
		if ok {
			confirmed = true
			chromedp.Run(ctx, chromedp.Sleep(2*time.Second))
			break
		}
	}

	if !confirmed {
		fmt.Fprintf(os.Stderr, "   ⚠️  请手动点击「确定」或「插入」完成图片上传\n")
	}

	return nil
}

// saveWxDraftWithVerify attempts to save the current WeChat draft and
// verifies that the save actually succeeded. Returns true if verified.
//
// Selector strategy: specific first (by ID, class), broad text match as
// last resort. Each click is followed by a verification check.
func saveWxDraftWithVerify(ctx context.Context) bool {
	saveScripts := []string{
		// 1. Most specific: by ID (current WeChat editor 2025+)
		`(() => {
			const el = document.querySelector('#js_article_save');
			if (el) { el.click(); return true; }
			return false;
		})()`,
		// 2. Class/attribute based selectors
		`(() => {
			const btns = document.querySelectorAll('.toolbar-save, .js_save, [data-role="save"], .save_btn, .btn_save');
			for (const b of btns) { b.click(); return true; }
			return false;
		})()`,
		// 3. aria-label / title
		`(() => {
			const all = document.querySelectorAll('[aria-label*="保存"], [title*="保存"], [data-action="save"]');
			for (const el of all) { el.click(); return true; }
			return false;
		})()`,
		// 4. a, button with exact text match (more targeted)
		`(() => {
			const btns = document.querySelectorAll('a, button');
			for (const b of btns) {
				const t = b.textContent.trim();
				if (t === '保存草稿' || t === '保存') { b.click(); return true; }
			}
			return false;
		})()`,
		// 5. Broad scan (last resort — too broad, but catches unusual layouts)
		`(() => {
			const btns = document.querySelectorAll('a, button, span, div');
			for (const b of btns) {
				const t = b.textContent.trim();
				if (t === '保存' || t === '保存草稿' || t.indexOf('保存') !== -1) {
					b.click(); return true;
				}
			}
			return false;
		})()`,
	}

	for _, script := range saveScripts {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &ok)); err != nil || !ok {
			continue
		}
		// Wait for the save action to complete and UI to update.
		chromedp.Run(ctx, chromedp.Sleep(2*time.Second))

		if verifySaveSuccess(ctx) {
			fmt.Fprintf(os.Stderr, "💾 草稿已保存\n")
			return true
		}
		// Clicked something but no confirmation — try the next selector.
	}

	return false
}

// verifySaveSuccess checks the page for indicators that the draft was saved.
func verifySaveSuccess(ctx context.Context) bool {
	var result string
	err := chromedp.Run(ctx, chromedp.Evaluate(`(() => {
		// 1. Look for success toasts (e.g., "保存成功", "已保存")
		const toasts = document.querySelectorAll(
			'[class*="toast"], [class*="success"], .weui-toast, [class*="tips"]'
		);
		for (const t of toasts) {
			const txt = t.textContent.trim();
			if (txt === '保存成功' || txt === '已保存') return 'toast';
		}
		// 2. Check if the save button changed to "已保存"
		const btns = document.querySelectorAll('a, button');
		for (const b of btns) {
			if (b.textContent.trim() === '已保存') return 'saved';
		}
		// Note: deliberately NOT checking URL for appmsgid — that would
		// cause a false positive on the 2nd save (URL already has appmsgid
		// from the 1st save). Toast and button state are sufficient.
		return '';
	})()`, &result))
	if err != nil {
		return false
	}
	return result != ""
}


// ---------------------------------------------------------------------------
// Main entry point
// ---------------------------------------------------------------------------

// WebWxDraft creates a WeChat draft from a local HTML file using chromeDP.
//
// The flow is fully automated:
//  1. Read the local HTML, extract body content, and discover images.
//  2. Launch Chrome (visible, so the user can scan the QR code if needed).
//  3. Navigate to mp.weixin.qq.com.
//  4. Wait for the user to log in (QR scan), detect the dashboard, and
//     automatically navigate to the article editor.
//  5. Set the article title, author, and body content.
//  6. Upload any images referenced in the HTML.
//  7. Save the draft.
func WebWxDraft(ctx context.Context, params WeChatDraftParams) error {
	// --- Phase 0: Read and parse the local HTML ---
	fmt.Fprintf(os.Stderr, "📄 读取 HTML: %s\n", params.HTMLPath)

	rawHTML, err := os.ReadFile(params.HTMLPath)
	if err != nil {
		return fmt.Errorf("读取 HTML 文件失败: %w", err)
	}
	htmlBaseDir := filepath.Dir(params.HTMLPath)
	bodyHTML := bodyContent(string(rawHTML))

	// Extract image references.
	images := extractImages(bodyHTML, htmlBaseDir)
	if len(images) > 0 {
		fmt.Fprintf(os.Stderr, "🖼️  发现 %d 张图片:\n", len(images))
		for _, img := range images {
			fmt.Fprintf(os.Stderr, "   - %s\n", img.AbsPath)
		}
	}

	// --- Phase 1: Launch Chrome ---
	chromePath, err := findChrome()
	if err != nil {
		return err
	}
	userDataDir, err := chromeUserDataDir()
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stderr, "🌐 启动 Chrome (%s)...\n", chromePath)
	fmt.Fprintf(os.Stderr, "📝 请用微信扫描浏览器中的二维码登录（如果尚未登录）\n")

	// Build allocator options — NOT headless so the user can scan the QR code.
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
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)

	// saveFailed controls browser close behavior at the end.
	// Default false: close Chrome on normal exit or early error.
	// Set true when automation reaches the end but saves aren't verified.
	var saveFailed bool
	defer func() {
		tabCancel()
		if saveFailed {
			fmt.Fprintf(os.Stderr, "🔴 自动保存未确认，请手动保存草稿后关闭浏览器\n")
		} else {
			allocCancel() // closes the browser process
		}
	}()

	// --- Phase 2: Preview the local HTML ---
	fmt.Fprintf(os.Stderr, "🔍 预览本地 HTML...\n")
	fileURL := "file://" + params.HTMLPath
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(fileURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		return fmt.Errorf("打开本地 HTML 失败: %w", err)
	}

	// Get the rendered body innerHTML for later use.
	var renderedBody string
	if err := chromedp.Run(tabCtx,
		chromedp.Evaluate(`document.body.innerHTML`, &renderedBody),
	); err != nil {
		return fmt.Errorf("读取 body 内容失败: %w", err)
	}
	if renderedBody == "" {
		renderedBody = bodyHTML
	}

	// --- Phase 3: Navigate to mp.weixin.qq.com ---
	fmt.Fprintf(os.Stderr, "🌐 导航到 mp.weixin.qq.com...\n")
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate("https://mp.weixin.qq.com"),
		chromedp.WaitReady("body"),
	); err != nil {
		return fmt.Errorf("打开 mp.weixin.qq.com 失败: %w", err)
	}

	// --- Phase 4: Wait for login + automatically navigate to the editor ---
	loginCtx, loginCancel := context.WithTimeout(tabCtx, 5*time.Minute)
	defer loginCancel()

	if err := waitForLogin(loginCtx); err != nil {
		return fmt.Errorf("登录超时: %w", err)
	}

	// Navigate to the editor (if not already there).
	if detectPageState(tabCtx) != stateEditor {
		fmt.Fprintf(os.Stderr, "🧭 自动导航到草稿编辑器...\n")
		navCtx, navCancel := context.WithTimeout(tabCtx, 60*time.Second)
		if err := navigateToEditor(navCtx); err != nil {
			navCancel()
			return fmt.Errorf("导航到编辑器失败: %w\n  请手动进入「内容管理 → 草稿箱 → 新的创作 → 文章」", err)
		}
		navCancel()
	}

	// Wait for the editor to be fully initialized.
	if err := waitForEditor(tabCtx); err != nil {
		return fmt.Errorf("编辑器加载超时: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✅ 已检测到草稿编辑器\n")

	// --- Debug: inspect the editor page (if --debug is set) ---
	if params.Debug {
		inspectEditorPage(tabCtx)
	}

	// --- Phase 5: Set title ---
	if params.Title != "" {
		fmt.Fprintf(os.Stderr, "📝 设置标题...\n")
		if err := setWxTitle(tabCtx, params.Title); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  设置标题失败: %v (请手动设置)\n", err)
		}
	}

	// --- Phase 6: Set author ---
	if params.Author != "" {
		fmt.Fprintf(os.Stderr, "✍️  设置作者...\n")
		if err := setWxAuthor(tabCtx, params.Author); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  设置作者失败: %v (请手动设置)\n", err)
		}
	}

	// --- Phase 7: Set body content ---
	fmt.Fprintf(os.Stderr, "📋 粘贴正文内容...\n")
	if err := setWxContent(tabCtx, renderedBody); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  粘贴正文失败: %v (请手动粘贴)\n", err)
	}

	// --- Phase 8: Save draft (first save — content only) ---
	fmt.Fprintf(os.Stderr, "💾 保存草稿...\n")
	save1OK := saveWxDraftWithVerify(tabCtx)
	if !save1OK {
		fmt.Fprintf(os.Stderr, "⚠️  第一次保存未确认\n")
	}

	// --- Phase 9: Upload images ---
	if len(images) > 0 {
		fmt.Fprintf(os.Stderr, "📤 上传 %d 张图片...\n", len(images))
		for i, img := range images {
			fmt.Fprintf(os.Stderr, "   [%d/%d] %s\n", i+1, len(images), img.AbsPath)
			if err := uploadWxImage(tabCtx, img.AbsPath); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  上传图片 %s 失败: %v (请手动上传)\n", img.AbsPath, err)
			}
		}
	}

	// --- Phase 10: Save draft again (with images) ---
	fmt.Fprintf(os.Stderr, "💾 保存草稿...\n")
	save2OK := saveWxDraftWithVerify(tabCtx)
	if !save2OK {
		fmt.Fprintf(os.Stderr, "⚠️  第二次保存未确认\n")
	}

	if !save1OK || !save2OK {
		saveFailed = true // leave browser open for manual save
		return nil
	}

	fmt.Fprintf(os.Stderr, "✅ 草稿已成功保存并关闭浏览器\n")
	return nil
}
