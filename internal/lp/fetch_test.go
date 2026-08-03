package lp

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
)

// TestFetchArgs verifies the fetch command-line construction through the
// full Fetch path (LookPath + arg building + runFetch), using a fake
// lightpanda binary on PATH that records its arguments one per line.
func TestFetchArgs(t *testing.T) {
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args.txt")
	bin := filepath.Join(dir, "lightpanda")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$FAKE_ARGS_FILE\"\necho '# Fake Markdown'\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_ARGS_FILE", argsFile)

	readArgs := func(t *testing.T) []string {
		t.Helper()
		data, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatalf("read args file: %v", err)
		}
		lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
		if len(lines) == 1 && lines[0] == "" {
			return nil
		}
		return lines
	}

	t.Run("defaults", func(t *testing.T) {
		out, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("Fetch() = %q, want fake markdown", out)
		}
		got := readArgs(t)
		want := []string{"fetch", "https://example.com", "--dump", "markdown", "--http-timeout", strconv.Itoa(httpTimeoutMS)}
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("args = %v, want %v", got, want)
		}
	})

	t.Run("all options", func(t *testing.T) {
		_, err := Fetch(context.Background(), "https://example.com", FetchOptions{
			Dump:        "html",
			TerminateMS: 5000,
			Proxy:       "socks5h://localhost:8777",
		})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		got := strings.Join(readArgs(t), " ")
		for _, want := range []string{
			"--dump html",
			"--http-timeout 300000",
			"--terminate-ms 5000",
			"--http-proxy socks5h://localhost:8777",
		} {
			if !strings.Contains(got, want) {
				t.Errorf("args %q missing %q", got, want)
			}
		}
	})

	t.Run("proxy from config", func(t *testing.T) {
		config.Set("lightpanda-proxy", "socks5h://localhost:9999")
		t.Cleanup(func() { config.Set("lightpanda-proxy", "") })
		_, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := strings.Join(readArgs(t), " "); !strings.Contains(got, "--http-proxy socks5h://localhost:9999") {
			t.Errorf("args %q missing config proxy", got)
		}
	})

	t.Run("proxy override wins", func(t *testing.T) {
		config.Set("lightpanda-proxy", "socks5h://localhost:9999")
		t.Cleanup(func() { config.Set("lightpanda-proxy", "") })
		_, err := Fetch(context.Background(), "https://example.com", FetchOptions{Proxy: "socks5h://localhost:1111"})
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
		if got := strings.Join(readArgs(t), " "); !strings.Contains(got, "--http-proxy socks5h://localhost:1111") {
			t.Errorf("args %q: explicit proxy should override config", got)
		}
	})
}

// TestRunFetchError verifies that a failing fetch surfaces stderr in the
// error message, so callers can diagnose 429/redirects.
func TestRunFetchError(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "lightpanda")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 'boom: 429 Too Many Requests' >&2\nexit 3\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	_, err := runFetch(context.Background(), bin, "fetch", "https://example.com")
	if err == nil {
		t.Fatal("runFetch() error = nil, want error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "boom: 429 Too Many Requests") {
		t.Errorf("error %q missing stderr content", msg)
	}
	if !strings.Contains(msg, "exit status") {
		t.Errorf("error %q missing exit status", msg)
	}
}

// TestFetchMissingBinary verifies the early error when lightpanda is not
// on PATH.
func TestFetchMissingBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // empty PATH, no lightpanda
	_, err := Fetch(context.Background(), "https://example.com", FetchOptions{})
	if err == nil {
		t.Fatal("Fetch() error = nil, want not-found error")
	}
	if !strings.Contains(err.Error(), "lightpanda not found") {
		t.Errorf("error = %v, want not-found message", err)
	}
}
