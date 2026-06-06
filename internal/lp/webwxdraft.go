// Package lp provides web page reading via lightpanda browser with CDP.
package lp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/browser"
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

// WebWxDraft creates a WeChat draft from a local HTML file using chromeDP.
// It opens the local HTML, reads its content, then navigates to mp.weixin.qq.com
// for the user to manually log in and create a draft.
//
// The function handles:
//  1. Opening the local HTML file to extract body content and images
//  2. Launching Chrome at mp.weixin.qq.com
//  3. Guiding the user through manual login and navigation to the draft editor
//  4. Setting the article title, author, and body content
//  5. Uploading images referenced in the HTML
//  6. Saving the draft
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
	fmt.Fprintf(os.Stderr, "📝 请在浏览器中:\n")
	fmt.Fprintf(os.Stderr, "   1. 登录 mp.weixin.qq.com\n")
	fmt.Fprintf(os.Stderr, "   2. 进入「内容管理 → 草稿箱 → 新的创作 → 文章」\n")
	fmt.Fprintf(os.Stderr, "   3. 将标题设为: %s\n", params.Title)
	if params.Author != "" {
		fmt.Fprintf(os.Stderr, "   4. 将作者设为: %s\n", params.Author)
	}
	fmt.Fprintf(os.Stderr, "⏳ 你有 5 分钟完成登录和导航...\n")

	// Build allocator options - NOT headless so user can see and interact.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(userDataDir),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-session-crashed-bubble", true),
		chromedp.NoSandbox,
		// Not headless: the user needs to see and interact with the page.
	}

	allocCtx, allocCancel := chromedp.NewExecAllocator(ctx, opts...)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	// Graceful browser shutdown.
	defer func() {
		if err := chromedp.Run(tabCtx, browser.Close()); err != nil {
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				fmt.Fprintf(os.Stderr, "⚠️ browser.Close() failed: %v\n", err)
			}
		}
	}()

	// --- Phase 2: Open the local HTML first to verify it renders ---
	fmt.Fprintf(os.Stderr, "🔍 预览本地 HTML...\n")
	fileURL := "file://" + params.HTMLPath
	if err := chromedp.Run(tabCtx,
		chromedp.Navigate(fileURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(1*time.Second),
	); err != nil {
		return fmt.Errorf("打开本地 HTML 失败: %w", err)
	}

	// Get the rendered body innerHTML.
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

	// --- Phase 4: Wait for user to log in and navigate to the editor ---
	fmt.Fprintf(os.Stderr, "⏳ 等待登录...\n")

	// Poll until we detect we're on the draft editor page or timeout.
	loginCtx, loginCancel := context.WithTimeout(tabCtx, 5*time.Minute)
	defer loginCancel()

	if err := waitForDraftEditor(loginCtx); err != nil {
		return fmt.Errorf("等待登录/导航超时: %w", err)
	}

	fmt.Fprintf(os.Stderr, "✅ 已检测到草稿编辑器\n")

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

	// --- Phase 8: Upload images ---
	if len(images) > 0 {
		fmt.Fprintf(os.Stderr, "📤 上传 %d 张图片...\n", len(images))
		for i, img := range images {
			fmt.Fprintf(os.Stderr, "   [%d/%d] %s\n", i+1, len(images), img.AbsPath)
			if err := uploadWxImage(tabCtx, img.AbsPath); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  上传图片 %s 失败: %v (请手动上传)\n", img.AbsPath, err)
			}
		}
	}

	fmt.Fprintf(os.Stderr, "✅ 操作完成！请检查草稿内容后手动保存。\n")
	return nil
}

// waitForDraftEditor polls until the WeChat draft editor page is detected.
// It looks for known editor elements (title input, contenteditable area, etc.).
func waitForDraftEditor(ctx context.Context) error {
	pollInterval := 2 * time.Second
	maxPolls := 150 // 5 minutes

	// Selectors that indicate the draft editor page has loaded.
	// WeChat's editor changes frequently, so we probe for multiple known patterns.
	editorDetectors := []string{
		`document.querySelector('iframe#ueditor_0') !== null`,
		`document.querySelector('[contenteditable="true"]') !== null`,
		`document.querySelector('div#js_editor_title') !== null`,
		`document.querySelector('input#title') !== null`,
		`document.querySelector('div.rich_media_area') !== null`,
		// WeChat new editor patterns:
		`typeof window.ue !== 'undefined' && window.ue !== null`,
		`document.querySelector('div.editor-content') !== null`,
	}

	for range maxPolls {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}

		for _, detector := range editorDetectors {
			var found bool
			if err := chromedp.Run(ctx, chromedp.Evaluate(detector, &found)); err != nil {
				continue
			}
			if found {
				// Found the editor. Give it a moment to fully initialize.
				chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
				return nil
			}
		}
	}
	return fmt.Errorf("未检测到草稿编辑器页面，请确认已登录并导航到文章编辑页")
}

// setWxTitle sets the article title in the WeChat editor.
func setWxTitle(ctx context.Context, title string) error {
	quoted := quoteJSString(title)

	// Try multiple known title field selectors.
	scripts := []string{
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
		// Author field might be in a form input or a dialog.
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
		// WeChat's new editor may show author in a dialog.
		`(() => {
			// Try clicking the author edit button first, then set the value.
			const btn = document.querySelector('a.author_edit');
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
				// Wait for the author dialog to appear, then set the input.
				chromedp.Run(ctx, chromedp.Sleep(1*time.Second))
				return setWxAuthorInput(ctx, author)
			}
		}
	}

	return nil // non-fatal, user can set manually
}

// setWxAuthorInput sets the author input field after the dialog is opened.
func setWxAuthorInput(ctx context.Context, author string) error {
	quoted := quoteJSString(author)
	script := fmt.Sprintf(`(() => {
		const el = document.querySelector('div.wx_dialog input');
		if (el) { el.value = %s; el.dispatchEvent(new Event('input')); 
			// Click confirm button
			const confirm = document.querySelector('div.wx_dialog a.btn_primary');
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

	// WeChat's editor uses an iframe-based rich text editor (UEditor or similar).
	// Strategy: find the iframe, access its document, and set body.innerHTML.
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
		// Pattern 3: Direct contenteditable div
		fmt.Sprintf(`(() => {
			const el = document.querySelector('[contenteditable="true"]');
			if (el) {
				el.innerHTML = %s;
				return true;
			}
			return false;
		})()`, quoted),
		// Pattern 4: New WeChat editor
		fmt.Sprintf(`(() => {
			const el = document.querySelector('div.rich_media_area');
			if (el) {
				el.innerHTML = %s;
				return true;
			}
			return false;
		})()`, quoted),
		// Pattern 5: Try window.ue (UEditor) API
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
	// WeChat's image upload flow:
	// 1. Click the image button in the editor toolbar
	// 2. In the dialog, click the "上传图片" tab/button
	// 3. Select the file via a file input
	// 4. Wait for upload to complete
	// 5. Insert the image into the editor

	// Try clicking the image toolbar button.
	imageBtnScripts := []string{
		// UEditor toolbar: image button
		`(() => {
			const btn = document.querySelector('.edui-for-image .edui-button-body');
			if (btn) { btn.click(); return true; }
			return false;
		})()`,
		// Toolbar button with image icon
		`(() => {
			const btn = document.querySelector('[title="图片"]');
			if (btn) { btn.click(); return true; }
			return false;
		})()`,
		// New editor: toolbar button area
		`(() => {
			const btns = document.querySelectorAll('.toolbar-btn');
			for (const b of btns) {
				if (b.textContent.includes('图片') || b.textContent.includes('image')) {
					b.click(); return true;
				}
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

	// Wait for the upload dialog to appear.
	chromedp.Run(ctx, chromedp.Sleep(2*time.Second))

	// Find and set the file input using chromedp.SetUploadFiles with selectors.
	fileSelectors := []string{
		`input[type="file"]`,
		`div.upload_file_box input[type="file"]`,
		`div.wx_dialog input[type="file"]`,
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
		// If still not uploaded, note it as non-fatal.
		fmt.Fprintf(os.Stderr, "   ⚠️  请手动上传图片: %s\n", imagePath)
	}

	// Wait for upload completion.
	chromedp.Run(ctx, chromedp.Sleep(3*time.Second))

	// Try to click "确定" or "插入" button to insert the uploaded image.
	var confirmed bool
	confirmScripts := []string{
		`(() => { const btn = document.querySelector('a.btn_primary'); if(btn && btn.textContent.includes('确定')) { btn.click(); return true; } return false; })()`,
		`(() => { const btn = document.querySelector('.dialog_confirm_btn'); if(btn) { btn.click(); return true; } return false; })()`,
		`(() => { const btn = document.querySelector('[node-type="ok"]'); if(btn) { btn.click(); return true; } return false; })()`,
	}

	for _, script := range confirmScripts {
		var ok bool
		if err := chromedp.Run(ctx, chromedp.Evaluate(script, &ok)); err != nil {
			continue
		}
		if ok {
			confirmed = true
			break
		}
	}

	if !confirmed {
		// Non-fatal: user can manually confirm.
		fmt.Fprintf(os.Stderr, "   ⚠️  请手动点击「确定」或「插入」完成图片上传\n")
	}

	return nil
}
