package lp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/userservice"
	"github.com/chromedp/chromedp"
)

// remoteHosts lists hosts that must be accessed via remote lightpanda
// (geo-restricted sites inaccessible from local network).
var remoteHosts = []string{
	"google.com",
	"www.google.com",
}

// isRemoteURL reports whether rawURL should be fetched via remote lightpanda.
func isRemoteURL(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return slices.Contains(remoteHosts, strings.ToLower(u.Host))
}

// ---- Function variables for test injection ----

// getFromCDP performs the actual CDP interaction to fetch markdown content.
var getFromCDP = defaultGetFromCDP

// isLocalAvailable checks if local lightpanda is listening on 127.2.2.9:9227.
var isLocalAvailable = defaultIsLocalAvailable

// lightpandaCmdExists checks if the lightpanda command exists and is valid.
var lightpandaCmdExists = defaultLightpandaCmdExists

// startLightpanda starts lightpanda serve and waits for it to be ready.
var startLightpanda = defaultStartLightpanda

// setupUserService creates and starts lightpanda as a user service
// (systemd on Linux, launchctl on macOS). Returns nil on success.
// If the platform doesn't support user services, returns an error
// so the caller can fall back to the child-process path.
var setupUserService = defaultSetupUserService

// startMu prevents concurrent auto-start attempts.
var startMu sync.Mutex

// startTried records whether we already attempted auto-start.
var startTried bool

// lightpandaServiceName is the user service name for lightpanda.
// On Linux this maps to ~/.config/systemd/user/<name>.service
// On macOS this maps to ~/Library/LaunchAgents/<name>.plist
const lightpandaServiceName = "dscli-lightpanda"

// ---- Default implementations ----

func defaultGetFromCDP(ctx context.Context, rawURL, cdpURL string) (string, error) {
	allocatorCtx, allocatorCancel := chromedp.NewRemoteAllocator(ctx, cdpURL, chromedp.NoModifyURL)
	defer allocatorCancel()

	tabCtx, tabCancel := chromedp.NewContext(allocatorCtx)
	defer tabCancel()

	tabCtx, timeoutCancel := context.WithTimeout(tabCtx, 60*time.Second)
	defer timeoutCancel()

	var markdownContent string

	err := chromedp.Run(tabCtx,
		chromedp.Navigate(rawURL),
		chromedp.ActionFunc(func(ctx context.Context) error {
			target := chromedp.FromContext(ctx).Target
			if target == nil {
				return fmt.Errorf("failed to get CDP target from context")
			}
			var result map[string]any
			if err := target.Execute(ctx, "LP.getMarkdown", nil, &result); err != nil {
				return fmt.Errorf("LP.getMarkdown: %w", err)
			}
			md, ok := result["markdown"].(string)
			if !ok {
				return fmt.Errorf("LP.getMarkdown response missing markdown field")
			}
			markdownContent = md
			return nil
		}),
	)
	if err != nil {
		return "", err
	}

	return markdownContent, nil
}

func defaultIsLocalAvailable() bool {
	host, port := localListenAddr()
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// localListenAddr returns host and port parsed from lightpanda-local-url config.
func localListenAddr() (host, port string) {
	raw := config.Get("lightpanda-local-url", "ws://127.2.2.9:9227")
	u, err := url.Parse(raw)
	if err != nil {
		return "127.2.2.9", "9227"
	}
	host = u.Hostname()
	port = u.Port()
	if host == "" {
		host = "127.2.2.9"
	}
	if port == "" {
		port = "9227"
	}
	return
}

func defaultLightpandaCmdExists() bool {
	path, err := exec.LookPath("lightpanda")
	if err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// lightpanda uses "version" subcommand, not "--version" flag.
	// Output is just version number (e.g. "0.3.0"), so we only
	// check exit code — no need to inspect stdout content.
	return exec.CommandContext(ctx, path, "version").Run() == nil
}

func defaultStartLightpanda() error {
	path, err := exec.LookPath("lightpanda")
	if err != nil {
		return fmt.Errorf("lightpanda 命令未找到")
	}

	host, port := localListenAddr()
	cmd := exec.Command(path, "serve", "--host", host, "--port", port)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动 lightpanda 失败: %w", err)
	}

	// Wait up to 15 seconds for lightpanda to become available.
	if err := waitForTCP(host, port, 15*time.Second); err != nil {
		cmd.Process.Kill()
		return fmt.Errorf("lightpanda 启动超时，请手动启动")
	}
	return nil
}
// ---- User service management ----

// defaultSetupUserService creates and starts lightpanda as a user service
// (systemd on Linux, launchctl on macOS). It is the default implementation
// of setupUserService.
//
// Returns nil on success. Returns an error if the platform doesn't support
// user services, or if service creation/start fails.
func defaultSetupUserService() error {
	lightpandaPath, err := exec.LookPath("lightpanda")
	if err != nil {
		return fmt.Errorf("lightpanda 命令未找到")
	}

	host, port := localListenAddr()
	execStart := fmt.Sprintf("%s serve --host %s --port %s", lightpandaPath, host, port)

	desc := "Lightpanda Browser (dscli)"
	if err := userservice.Create(lightpandaServiceName, desc, execStart); err != nil {
		return fmt.Errorf("创建用户服务失败: %w", err)
	}

	if err := userservice.Start(lightpandaServiceName); err != nil {
		return fmt.Errorf("启动用户服务失败: %w", err)
	}

	return waitForTCP(host, port, 15*time.Second)
}

// waitForTCP polls a TCP address until it accepts connections or times out.
func waitForTCP(host, port string, timeout time.Duration) error {
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	addr := net.JoinHostPort(host, port)
	for {
		select {
		case <-deadline:
			// Final check before giving up.
			conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
			if err == nil {
				conn.Close()
				return nil
			}
			return fmt.Errorf("等待 %s 超时", addr)
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
			if err == nil {
				conn.Close()
				return nil
			}
		}
	}
}

// ---- Public API ----

// Get fetches a web page via lightpanda CDP and returns its markdown content.
//
// It automatically routes to remote lightpanda for geo-restricted hosts
// (listed in remoteHosts), and uses local lightpanda for all other URLs.
// If remote is not configured, even remoteHosts fall back to local.
//
// If local lightpanda is not running but the lightpanda command is available,
// Get will auto-start lightpanda serve in the background before fetching.
//
// Config keys used (from ~/.dscli/config.dscli):
//   - lightpanda-local-url   (default: ws://127.2.2.9:9227)
//   - lightpanda-remote-url  (default: "")
//   - lightpanda-remote-token (default: "")
func Get(ctx context.Context, rawURL string) (string, error) {
	cdpURL, isLocal := cdpEndpoint(rawURL)

	if isLocal {
		if err := ensureLocalLightpanda(); err != nil {
			return "", err
		}
	}

	markdown, err := getFromCDP(ctx, rawURL, cdpURL)
	if err != nil {
		if isLocal {
			host, port := localListenAddr()
			return "", fmt.Errorf(
				"lightpanda 连接失败: %w\n\n请确保 lightpanda 已启动:\n"+
				"  lightpanda serve --host %s --port %s",
				err, host, port,
			)
		}
		return "", fmt.Errorf("lightpanda remote 连接失败: %w", err)
	}

	return markdown, nil
}

// ensureLocalLightpanda ensures local lightpanda is running, starting it if
// needed. Only one start attempt is made per process lifetime.
//
// It first tries to set up a user service (systemd on Linux, launchctl on
// macOS, lifecycle independent of dscli). If the platform doesn't support
// user services or setup fails, it falls back to starting lightpanda as a
// child process (lifecycle tied to dscli).
func ensureLocalLightpanda() error {
	if isLocalAvailable() {
		return nil
	}

	startMu.Lock()
	defer startMu.Unlock()

	// Double-check after acquiring lock (another goroutine may have started it).
	if isLocalAvailable() {
		return nil
	}

	if startTried {
		return fmt.Errorf("lightpanda 自动启动失败，请手动启动")
	}
	startTried = true

	if !lightpandaCmdExists() {
		return fmt.Errorf("lightpanda 未安装，请访问 https://lightpanda.io 安装")
	}

	// Preferred: user service (lifecycle independent of dscli).
	if err := setupUserService(); err == nil {
		return nil
	}

	// Fallback: child process (lifecycle tied to dscli).
	return startLightpanda()
}

// cdpEndpoint returns the WebSocket URL and whether it's a local endpoint.
//
// For hosts in remoteHosts: returns remote URL if configured, otherwise
// falls back to local. For all other hosts: returns local URL.
func cdpEndpoint(rawURL string) (cdpURL string, isLocal bool) {
	if isRemoteURL(rawURL) {
		remoteURL := config.Get("lightpanda-remote-url", "")
		remoteToken := config.Get("lightpanda-remote-token", "")
		if remoteURL != "" {
			if remoteToken != "" {
				if strings.Contains(remoteURL, "?") {
					return remoteURL + "&token=" + remoteToken, false
				}
				return remoteURL + "?token=" + remoteToken, false
			}
			return remoteURL, false
		}
		// Remote not configured — fallback to local.
	}
	return config.Get("lightpanda-local-url", "ws://127.2.2.9:9227"), true
}
