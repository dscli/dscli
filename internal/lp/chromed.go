package lp

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"

	"github.com/chromedp/chromedp"
)

// Chromium remote debugging constants.
// Services should be created via 'dscli service create dscli-chromium'.
// See 'dscli service create --help' for usage.
const (
	chromiumHost = "127.0.0.1"
	chromiumPort = "9228"
)

// chromiumAddr returns the host:port address for the chromium instance.
func chromiumAddr() string {
	return fmt.Sprintf("%s:%s", chromiumHost, chromiumPort)
}

// chromiumCDPURL queries the chromium HTTP endpoint at chromiumAddr()
// for the WebSocket debugger URL that chromedp uses to connect.
func chromiumCDPURL() (string, error) {
	url := fmt.Sprintf("http://%s/json/version", chromiumAddr())
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return "", fmt.Errorf("chromium HTTP endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()

	var v struct {
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return "", fmt.Errorf("chromium version JSON parse error: %w", err)
	}
	if v.WebSocketDebuggerURL == "" {
		return "", fmt.Errorf("chromium returned empty webSocketDebuggerUrl")
	}
	return v.WebSocketDebuggerURL, nil
}

// IsChromiumAvailable checks whether a chromium instance is listening on
// the expected address.  This is a cheap TCP-level probe — no JSON query.
func IsChromiumAvailable() bool {
	conn, err := net.DialTimeout("tcp", chromiumAddr(), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// ConnectChromium connects to a running chromium instance and returns a
// chromedp remote allocator context.  The caller must call the returned
// cancel function when done with the tab.
//
// Since the browser lifecycle is managed externally (via dscli service),
// only the tab context is cancelled — the browser itself is never closed.
func ConnectChromium(ctx context.Context) (context.Context, func(), error) {
	wsURL, err := chromiumCDPURL()
	if err != nil {
		return nil, nil, fmt.Errorf("connect chromium: %w", err)
	}

	allocCtx, allocCancel := chromedp.NewRemoteAllocator(ctx, wsURL, chromedp.NoModifyURL)
	tabCtx, tabCancel := chromedp.NewContext(allocCtx)

	// Return a combined cancel that cleans up the tab and allocator but
	// leaves the remote browser running.
	return tabCtx, func() {
		tabCancel()
		allocCancel()
	}, nil
}

// chromiumServiceCommand returns the *exec.Cmd that runs chromium as a
// background service with remote debugging enabled.
// This is used by the user via: dscli service create dscli-chromium <<EOF
func chromiumServiceCommand() (*exec.Cmd, error) {
	chromePath, err := findChrome()
	if err != nil {
		return nil, err
	}

	userDataDir, err := chromeUserDataDir()
	if err != nil {
		return nil, err
	}

	args := []string{
		chromePath,
		"--remote-debugging-port=" + chromiumPort,
		"--no-first-run",
		"--no-default-browser-check",
		"--disable-session-crashed-bubble",
		"--no-sandbox",
		"--disable-blink-features=AutomationControlled",
		"--user-data-dir=" + userDataDir,
		// Keep running even when no CDP client is connected.
		"--keep-alive-for-test",
	}

	cmd := exec.Command(args[0], args[1:]...)
	// Silence output from the background service.
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd, nil
}

// waitForInterrupt blocks until SIGINT (Ctrl+C) or SIGTERM is received.
// This is used to keep a locally-launched Chrome alive until the user
// finishes inspecting the browser.
func waitForInterrupt() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	signal.Stop(sigCh)
}
