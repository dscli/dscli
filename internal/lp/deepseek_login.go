package lp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

const (
	deepseekSignInURL = "https://chat.deepseek.com/sign_in"
	maxLoginWait      = 30 * time.Second
)

// DefaultCookiePath returns the default path for the DeepSeek cookie file.
func DefaultCookiePath() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return "deepseek-cookies.json"
	}
	return filepath.Join(dir, ".dscli", "deepseek-cookies.json")
}

// DeepSeekLogin performs automated login via Lightpanda CDP.
// Prefer DeepSeekLoginChrome — Chrome correctly renders the Shumei captcha.
// Use this only as a Lightpanda fallback.
func DeepSeekLogin(ctx context.Context, phone string, codeReader func() (string, error)) error {
	cdpURL, isLocal := cdpEndpoint(deepseekSignInURL)
	if isLocal {
		if err := ensureLocalLightpanda(); err != nil {
			return err
		}
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, cdpURL)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	return deepseekLogin(tabCtx, phone, codeReader)
}

// deepseekLogin performs the shared DeepSeek login flow on an already
// established browser tab context (works with any CDP-based browser:
// Lightpanda, Chrome, etc.).
func deepseekLogin(tabCtx context.Context, phone string, codeReader func() (string, error)) error {
	// Give ample time for the user to receive SMS and type the code.
	loginCtx, loginCancel := context.WithTimeout(tabCtx, 5*time.Minute)
	defer loginCancel()

	fmt.Fprintf(os.Stderr, "🌐 正在打开 DeepSeek 登录页...\n")

	if err := chromedp.Run(loginCtx,
		chromedp.Navigate(deepseekSignInURL),
		chromedp.WaitReady("body"),
		chromedp.Sleep(4*time.Second),
	); err != nil {
		return fmt.Errorf("打开登录页失败: %w", err)
	}

	// Verify we are on the sign-in page.
	var currentURL string
	if err := chromedp.Run(loginCtx, chromedp.Location(&currentURL)); err != nil {
		return fmt.Errorf("检查当前页面失败: %w", err)
	}
	if !strings.Contains(currentURL, "sign_in") {
		// Already logged in — just save cookies and return.
		fmt.Fprintf(os.Stderr, "✅ 已处于登录状态（当前页面: %s）\n", currentURL)
		return saveDeepSeekCookies(loginCtx)
	}

	// Step 1: Enter phone number.
	fmt.Fprintf(os.Stderr, "📱 正在输入手机号 %s ...\n", phone)
	if err := setInputValue(loginCtx, "input[type='tel']", 0, phone); err != nil {
		return fmt.Errorf("输入手机号失败: %w", err)
	}
	chromedp.Run(loginCtx, chromedp.Sleep(500*time.Millisecond))

	// Step 2: Click "Send code" / "发送验证码".
	fmt.Fprintf(os.Stderr, "📤 正在发送验证码...\n")
	if err := clickButtonByText(loginCtx, "发送验证码", "Send code"); err != nil {
		return fmt.Errorf("发送验证码失败: %w", err)
	}

	// Step 3: Read verification code from user.
	code, err := codeReader()
	if err != nil {
		return fmt.Errorf("读取验证码失败: %w", err)
	}
	code = strings.TrimSpace(code)
	if len(code) < 4 {
		return fmt.Errorf("验证码长度不足（至少 4 位）")
	}

	// Step 4: Enter verification code.
	fmt.Fprintf(os.Stderr, "🔢 正在输入验证码...\n")
	if err := setInputValue(loginCtx, "input[type='tel']", 1, code); err != nil {
		return fmt.Errorf("输入验证码失败: %w", err)
	}
	chromedp.Run(loginCtx, chromedp.Sleep(500*time.Millisecond))

	// Step 5: Click "Log in" / "登录".
	fmt.Fprintf(os.Stderr, "🔑 正在登录...\n")
	if err := clickButtonByText(loginCtx, "登录", "Log in"); err != nil {
		return fmt.Errorf("点击登录按钮失败: %w", err)
	}

	// Step 6: Wait for redirect away from sign_in page.
	fmt.Fprintf(os.Stderr, "⏳ 等待登录完成...\n")
	deadline := time.After(maxLoginWait)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-loginCtx.Done():
			return fmt.Errorf("登录超时: %w", loginCtx.Err())
		case <-deadline:
			return fmt.Errorf("登录超时 (%v)，请检查验证码是否正确", maxLoginWait)
		case <-ticker.C:
			var url string
			if err := chromedp.Run(loginCtx, chromedp.Location(&url)); err != nil {
				continue
			}
			if !strings.Contains(url, "sign_in") {
				fmt.Fprintf(os.Stderr, "✅ 登录成功！\n")
				return saveDeepSeekCookies(loginCtx)
			}
			// Check for error messages on the page.
			var bodyText string
			chromedp.Run(loginCtx,
				chromedp.Evaluate("document.body ? document.body.innerText : ''", &bodyText),
			)
			if strings.Contains(bodyText, "incorrect") ||
				strings.Contains(bodyText, "Incorrect") ||
				strings.Contains(bodyText, "expired") ||
				strings.Contains(bodyText, "错误") {
				return fmt.Errorf("登录失败: %s",
					bodyText[:minStrLen(500, len(bodyText))])
			}
		}
	}
}

// saveDeepSeekCookies extracts cookies from the current browser context and
// writes them to the cookie file in Lightpanda-compatible JSON format.
func saveDeepSeekCookies(ctx context.Context) error {
	cookiePath := DefaultCookiePath()

	// Use CDP Network.getCookies wrapped in chromedp.ActionFunc so the
	// context has the proper CDP executor attached.
	var cookies []*network.Cookie
	if err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			cookies, err = network.GetCookies().WithURLs([]string{
				"https://chat.deepseek.com",
				"https://deepseek.com",
			}).Do(ctx)
			return err
		}),
	); err != nil {
		return fmt.Errorf("读取 cookies 失败: %w", err)
	}

	type cookieEntry struct {
		Name   string `json:"name"`
		Value  string `json:"value"`
		Domain string `json:"domain"`
		Path   string `json:"path"`
	}

	var entries []cookieEntry
	for _, c := range cookies {
		entries = append(entries, cookieEntry{
			Name:   c.Name,
			Value:  c.Value,
			Domain: c.Domain,
			Path:   c.Path,
		})
	}

	if len(entries) == 0 {
		return fmt.Errorf("未找到任何 cookie")
	}

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(cookiePath), 0755); err != nil {
		return fmt.Errorf("创建 cookie 目录失败: %w", err)
	}

	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 cookies 失败: %w", err)
	}

	if err := os.WriteFile(cookiePath, data, 0600); err != nil {
		return fmt.Errorf("写入 cookie 文件失败: %w", err)
	}

	fmt.Fprintf(os.Stderr, "💾 Cookies 已保存到 %s\n", cookiePath)
	fmt.Fprintf(os.Stderr, "   共 %d 个 cookie\n", len(entries))

	return nil
}

// ReadCodeFromStdin reads a verification code from stdin with a prompt.
func ReadCodeFromStdin() (string, error) {
	fmt.Print("\n📱 请输入 6 位短信验证码: ")
	reader := bufio.NewReader(os.Stdin)
	code, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(code), nil
}

// --- helper functions for CDP interaction ---

// setInputValue sets the value of an input element identified by CSS selector
// and index (0-based among matching elements) using the native setter to
// trigger React change detection.
func setInputValue(ctx context.Context, selector string, index int, value string) error {
	js := fmt.Sprintf(`(() => {
	const inputs = document.querySelectorAll(%s);
	if (inputs.length <= %d) return {error: 'input['+%d+'] not found (only '+inputs.length+' matched)'};
	const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, 'value').set;
	setter.call(inputs[%d], %s);
	inputs[%d].dispatchEvent(new Event('input', {bubbles: true}));
	inputs[%d].dispatchEvent(new Event('change', {bubbles: true}));
	return {success: true};
})()`, quoteJS(selector), index, index, index, quoteJS(value), index, index)

	var result map[string]any
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("js evaluate: %w", err)
	}
	if errMsg, ok := result["error"].(string); ok {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

// clickButtonByText clicks a visible button whose text content matches one of
// the candidate texts. Tries exact match first, then partial match.
func clickButtonByText(ctx context.Context, texts ...string) error {
	if len(texts) == 0 {
		return fmt.Errorf("no button texts provided")
	}

	// Build a JS array of candidate texts.
	quotedTexts := make([]string, len(texts))
	for i, t := range texts {
		quotedTexts[i] = quoteJS(t)
	}
	textsJSON := "[" + strings.Join(quotedTexts, ", ") + "]"

	js := fmt.Sprintf(`(() => {
	const candidates = %s;
	const buttons = document.querySelectorAll('button');
	// Try exact match first.
	for (const btn of buttons) {
		if (btn.offsetParent === null) continue;
		const txt = btn.textContent.trim();
		for (const c of candidates) {
			if (txt === c) { btn.click(); return {success: true, matched: c}; }
		}
	}
	// Fallback: try partial match (includes).
	for (const btn of buttons) {
		if (btn.offsetParent === null) continue;
		const txt = btn.textContent.trim();
		for (const c of candidates) {
			if (txt.includes(c)) { btn.click(); return {success: true, matched: c, partial: true}; }
		}
	}
	return {error: 'none of ' + JSON.stringify(candidates) + ' matched any button'};
})()`, textsJSON)

	var result map[string]any
	if err := chromedp.Run(ctx, chromedp.Evaluate(js, &result)); err != nil {
		return fmt.Errorf("js evaluate: %w", err)
	}
	if errMsg, ok := result["error"].(string); ok {
		return fmt.Errorf("%s", errMsg)
	}
	return nil
}

// minStrLen returns the minimum of two ints.
func minStrLen(a, b int) int {
	if a < b {
		return a
	}
	return b
}
