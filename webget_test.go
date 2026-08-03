package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	"github.com/spf13/cobra"
)

// writeFakeLightpanda creates a fake lightpanda binary on PATH that records
// its arguments one per line in argsFile and emits --json output selected by
// FAKE_MODE ("" = HTTP 200 with markdown, status404 = HTTP 404, raw = garbage).
func writeFakeLightpanda(t *testing.T, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "lightpanda")
	script := `#!/bin/sh
printf '%s\n' "$@" >> "$FAKE_ARGS_FILE"
case "$FAKE_MODE" in
  status404) echo '{"url":"u","http_status":404,"content":"not found"}' ;;
  raw) echo 'not json' ;;
  *) echo '{"url":"u","http_status":200,"content":"# Fake Markdown\n"}' ;;
esac
`
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_ARGS_FILE", argsFile)
	// Defensive: pin the mode to the default in case the developer's shell
	// exported FAKE_MODE. Subtests that need another mode override it.
	t.Setenv("FAKE_MODE", "")
}

// clearProxyConfig removes proxy config keys so tests never depend on the
// developer's machine configuration.
func clearProxyConfig(t *testing.T) {
	t.Helper()
	config.SetValue("lightpanda-http-proxy", nil)
	config.SetValue("lightpanda-additional-proxy-domains", nil)
	t.Cleanup(func() {
		config.SetValue("lightpanda-http-proxy", nil)
		config.SetValue("lightpanda-additional-proxy-domains", nil)
	})
}

// runWebget executes webReaderRunE through a minimal cobra command and
// returns captured stdout plus the returned error (mirroring main.go, which
// prints the error to stderr and exits 1).
func runWebget(t *testing.T, args ...string) (stdout string, err error) {
	t.Helper()
	cmd := &cobra.Command{
		Use:           "webget <url>",
		RunE:          webReaderRunE,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().Int("timeout", 330, "")
	cmd.Flags().String("dump", "markdown", "")
	cmd.Flags().Bool("force-proxy", false, "")
	cmd.Flags().String("output", "", "")
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs(args)
	// Execute must run before out.String() is evaluated - Go evaluates
	// function arguments left to right in a return statement.
	err = cmd.Execute()
	return out.String(), err
}

func TestWebgetRunE(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeLightpanda(t, argsFile)
	clearProxyConfig(t)

	t.Run("success prints markdown", func(t *testing.T) {
		out, err := runWebget(t, "https://example.com")
		if err != nil {
			t.Fatalf("runWebget() error = %v", err)
		}
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("stdout = %q, want Fake Markdown", out)
		}
	})

	t.Run("dump flag forwarded to lightpanda", func(t *testing.T) {
		runWebget(t, "https://example.com", "--dump", "html")
		data, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Join(strings.Fields(string(data)), " ")
		if !strings.Contains(args, "--dump html") {
			t.Errorf("lightpanda args %q missing --dump html", args)
		}
	})

	t.Run("http error returned", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "status404")
		_, err := runWebget(t, "https://example.com")
		if err == nil || !strings.Contains(err.Error(), "读取网页失败") {
			t.Errorf("error = %v, want 读取网页失败", err)
		}
	})

	t.Run("missing URL rejected", func(t *testing.T) {
		_, err := runWebget(t)
		if err == nil || !strings.Contains(err.Error(), "URL 参数") {
			t.Errorf("error = %v, want URL 参数 hint", err)
		}
	})

	t.Run("missing scheme rejected", func(t *testing.T) {
		_, err := runWebget(t, "example.com")
		if err == nil || !strings.Contains(err.Error(), "http:// 或 https://") {
			t.Errorf("error = %v, want scheme hint", err)
		}
	})

	t.Run("uppercase scheme accepted", func(t *testing.T) {
		// RFC 3986: URL schemes are case-insensitive.
		out, err := runWebget(t, "HTTPS://example.com")
		if err != nil {
			t.Fatalf("runWebget() error = %v", err)
		}
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("stdout = %q, want Fake Markdown", out)
		}
	})

	t.Run("invalid dump rejected", func(t *testing.T) {
		_, err := runWebget(t, "https://example.com", "--dump", "pdf")
		if err == nil || !strings.Contains(err.Error(), "--dump 仅支持") {
			t.Errorf("error = %v, want dump enum hint", err)
		}
	})

	t.Run("bad json returned", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "raw")
		_, err := runWebget(t, "https://example.com")
		if err == nil || !strings.Contains(err.Error(), "读取网页失败") {
			t.Errorf("error = %v, want 读取网页失败", err)
		}
	})

	t.Run("force-proxy goes straight through proxy", func(t *testing.T) {
		config.Set("lightpanda-http-proxy", "socks5h://localhost:9999")
		runWebget(t, "https://example.com", "--force-proxy")
		data, err := os.ReadFile(argsFile)
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Join(strings.Fields(string(data)), " ")
		// Single invocation (probe skipped) with the proxy flag present.
		if !strings.Contains(args, "--http-proxy socks5h://localhost:9999") {
			t.Errorf("lightpanda args %q missing forced proxy", args)
		}
	})

	t.Run("output writes file and keeps stdout", func(t *testing.T) {
		outFile := filepath.Join(t.TempDir(), "out.md")
		out, err := runWebget(t, "https://example.com", "--output", outFile)
		if err != nil {
			t.Fatalf("runWebget() error = %v", err)
		}
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("stdout = %q, want Fake Markdown", out)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		if string(data) != out {
			t.Errorf("file = %q, want %q", string(data), out)
		}
	})

	t.Run("output with line number inserts", func(t *testing.T) {
		outFile := filepath.Join(t.TempDir(), "out.md")
		if err := os.WriteFile(outFile, []byte("# Title\nold\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := runWebget(t, "https://example.com", "--output", outFile+":2"); err != nil {
			t.Fatalf("runWebget() error = %v", err)
		}
		data, err := os.ReadFile(outFile)
		if err != nil {
			t.Fatalf("read output file: %v", err)
		}
		got := string(data)
		if !strings.HasPrefix(got, "# Title\n") || !strings.HasSuffix(got, "\nold\n") {
			t.Errorf("file = %q, want inserted between # Title and old", got)
		}
		if !strings.Contains(got, "Fake Markdown") {
			t.Errorf("file = %q, want fetched content", got)
		}
	})

	t.Run("output invalid line rejected", func(t *testing.T) {
		_, err := runWebget(t, "https://example.com", "--output", "out.md:0")
		if err == nil || !strings.Contains(err.Error(), "positive integer") {
			t.Errorf("error = %v, want positive integer hint", err)
		}
	})
}
