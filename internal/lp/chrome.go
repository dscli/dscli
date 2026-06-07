package lp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/chromedp/chromedp"
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

// NetworkCheck verifies that the target URL's host is reachable via TCP
// before launching a browser. This avoids wasting time on a truly down
// network — Chrome would otherwise report ERR_INTERNET_DISCONNECTED and
// fail after a much longer timeout.
//
// The check connects directly from the host, bypassing Chrome's network
// stack, so it only fails when the host actually has no connectivity.
// On URL parse errors the check is skipped (assume reachable).
func NetworkCheck(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil // can't parse, skip check
	}
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	addr := net.JoinHostPort(host, port)
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return fmt.Errorf("网络不可达 (%s): %w", host, err)
	}
	_ = conn.Close()
	return nil
}

// NewChromium creates a new chromedp ExecAllocator backed by a local
// Chrome/Chromium browser. The caller must call the returned cancel func
// to shut down the browser when done.
//
// Usage:
//
//	ctx, cancel, err := NewChromium(parentCtx)
//	if err != nil { return err }
//	defer cancel()
//	tabCtx, tabCancel := chromedp.NewContext(ctx)
//	defer tabCancel()
func NewChromium(ctx context.Context) (context.Context, func(), error) {
	chromePath, err := findChrome()
	if err != nil {
		return nil, nil, err
	}
	userDataDir, err := chromeUserDataDir()
	if err != nil {
		return nil, nil, err
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
	return allocCtx, allocCancel, nil
}
