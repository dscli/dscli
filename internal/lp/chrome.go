package lp

import (
	"context"
	"fmt"
	"github.com/chromedp/cdproto/browser"
	"github.com/chromedp/chromedp"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// findChrome locates a Chrome/Chromium binary on the system.
func findChrome() (string, error) {
	for _, name := range []string{
		"chromium",
		"chromium-browser",
		"google-chrome",
		"google-chrome-stable",
		"chrome",
	} {
		if path, err := exec.LookPath(name); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("未找到 Chrome/Chromium，请安装后重试")
}

// chromeUserDataDir returns the persistent Chrome user data directory
// shared by login and webchat so cookies survive across sessions.
func chromeUserDataDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("无法获取用户目录: %w", err)
	}
	dir := filepath.Join(home, ".dscli", "chrome-profile")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建 Chrome profile 目录: %w", err)
	}
	return dir, nil
}

// DeepSeekLoginChrome performs login to chat.deepseek.com using a local
// Chrome/Chromium browser. This is preferred over the Lightpanda-native
// login because Chrome properly renders the Shumei captcha widget.
func DeepSeekLoginChrome(ctx context.Context, phone string, codeReader func() (string, error)) error {
	return DeepSeekLoginChromeOpts(ctx, phone, codeReader, false)
}

// DeepSeekLoginChromeOpts is like DeepSeekLoginChrome but allows disabling
// headless mode via the visible parameter (useful for manual captcha solving).
func DeepSeekLoginChromeOpts(ctx context.Context, phone string, codeReader func() (string, error), visible bool) error {
	chromePath, err := findChrome()
	if err != nil {
		return err
	}

	mode := "headless"
	if visible {
		mode = "visible"
	}
	fmt.Fprintf(os.Stderr, "🌐 使用 Chrome (%s, %s) 登录 DeepSeek...\n", chromePath, mode)

	userDataDir, err := chromeUserDataDir()
	if err != nil {
		return err
	}

	// Build allocator options. We use --headless=new (the newer headless
	// mode that is harder for sites to detect as automation) and disable
	// the "Chrome is being controlled by automated software" infobar.
	// UserDataDir ensures cookies persist across sessions — webchat reuses
	// the same profile so it picks up the login session automatically.
	opts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(chromePath),
		chromedp.UserDataDir(userDataDir),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-session-crashed-bubble", true),
		chromedp.NoSandbox,
	}
	if !visible {
		opts = append(opts, chromedp.Flag("headless", "new"))
	}

	// Create a context with timeout for the whole login process.
	// Give ample time: user needs to receive SMS and type the code.
	allocCtx, allocCancel := context.WithTimeout(ctx, 5*time.Minute)
	defer allocCancel()

	allocCtx, allocCancel = chromedp.NewExecAllocator(allocCtx, opts...)
	defer allocCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocCtx)
	defer tabCancel()

	// Graceful browser shutdown before the defers kill the process.
	defer func() {
		_ = chromedp.Run(tabCtx, browser.Close())
	}()

	return deepseekLogin(tabCtx, phone, codeReader, visible)
}
