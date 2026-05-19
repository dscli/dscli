package lp

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestGet_EndToEnd 从用户角度验证 Get 的完整链路：
//
//  1. 启动本地 HTTP 测试服务器（返回已知 HTML）
//  2. 启动 lightpanda 浏览器
//  3. 调用 Get 获取页面
//  4. 验证返回的 Markdown 内容
//
// 此测试依赖 lightpanda 命令可用；未安装时自动跳过。
// 通过真实 CDP 协议交互，能发现 CLI 参数变更、协议不兼容、
// 超时配置错误等问题——这些是单元测试覆盖不到的。
func TestGet_EndToEnd(t *testing.T) {
	lightpandaPath, err := exec.LookPath("lightpanda")
	if err != nil {
		t.Skip("lightpanda not installed, skipping end-to-end test")
	}

	// 启动本地 HTTP 测试服务器，返回已知 HTML。
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
  <h1>Hello World</h1>
  <p>This is a test paragraph.</p>
</body>
</html>`)
	}))
	defer srv.Close()

	// 使用非默认端口，避免与用户已运行的 lightpanda 冲突。
	const testHost = "127.2.2.8"
	const testPort = "9230"

	// 启动 lightpanda。
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, lightpandaPath, "serve",
		"--obey-robots",
		"--host", testHost,
		"--port", testPort,
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lightpanda: %v", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait() //nolint:errcheck
	}()

	// 等待 lightpanda 就绪。
	if !waitForLightpanda(t, testHost, testPort, 15*time.Second) {
		t.Fatal("lightpanda did not become ready in time")
	}

	// 配置 lp 包使用测试 lightpanda。
	withConfig(t, "lightpanda-local-url", fmt.Sprintf("ws://%s:%s", testHost, testPort))

	// 调用 Get —— 这是被测的公开 API。
	got, err := Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get(%q): %v", srv.URL, err)
	}

	// 验证 Markdown 输出包含页面内容。
	if !strings.Contains(got, "Hello World") {
		t.Errorf("markdown missing 'Hello World':\n%s", got)
	}
	if !strings.Contains(got, "test paragraph") {
		t.Errorf("markdown missing 'test paragraph':\n%s", got)
	}
}

// waitForLightpanda 轮询 TCP 端口直到 lightpanda 接受连接或超时。
func waitForLightpanda(t *testing.T, host, port string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.After(timeout)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	addr := net.JoinHostPort(host, port)
	for {
		select {
		case <-deadline:
			return false
		case <-ticker.C:
			conn, err := net.DialTimeout("tcp", addr, 1*time.Second)
			if err == nil {
				conn.Close()
				return true
			}
		}
	}
}
