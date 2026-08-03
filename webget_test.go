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
	t.Cleanup(func() { config.SetValue("lightpanda-http-proxy", nil) })
}

// runWebget executes webReaderRunE through a minimal cobra command and
// returns captured stdout/stderr.
func runWebget(t *testing.T, args ...string) (stdout, stderr string) {
	t.Helper()
	cmd := &cobra.Command{
		Use:           "webget <url>",
		Args:          cobra.ExactArgs(1),
		RunE:          webReaderRunE,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.Flags().Int("timeout", 330, "")
	cmd.Flags().String("dump", "markdown", "")
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
	return out.String(), errBuf.String()
}

func TestWebgetRunE(t *testing.T) {
	argsFile := filepath.Join(t.TempDir(), "args.txt")
	writeFakeLightpanda(t, argsFile)
	clearProxyConfig(t)

	t.Run("success prints markdown", func(t *testing.T) {
		out, errS := runWebget(t, "https://example.com")
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("stdout = %q, want Fake Markdown", out)
		}
		if errS != "" {
			t.Errorf("stderr = %q, want empty", errS)
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

	t.Run("http error reported on stderr", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "status404")
		_, errS := runWebget(t, "https://example.com")
		if !strings.Contains(errS, "Error: 读取网页失败") {
			t.Errorf("stderr = %q, want error prefix", errS)
		}
	})

	t.Run("missing scheme rejected", func(t *testing.T) {
		_, errS := runWebget(t, "example.com")
		if !strings.Contains(errS, "http:// 或 https://") {
			t.Errorf("stderr = %q, want scheme hint", errS)
		}
	})

	t.Run("uppercase scheme accepted", func(t *testing.T) {
		// RFC 3986: URL schemes are case-insensitive.
		out, errS := runWebget(t, "HTTPS://example.com")
		if !strings.Contains(out, "Fake Markdown") {
			t.Errorf("stdout = %q, want Fake Markdown", out)
		}
		if errS != "" {
			t.Errorf("stderr = %q, want empty", errS)
		}
	})

	t.Run("invalid dump rejected", func(t *testing.T) {
		_, errS := runWebget(t, "https://example.com", "--dump", "pdf")
		if !strings.Contains(errS, "--dump 仅支持") {
			t.Errorf("stderr = %q, want dump enum hint", errS)
		}
	})

	t.Run("bad json reported on stderr", func(t *testing.T) {
		t.Setenv("FAKE_MODE", "raw")
		_, errS := runWebget(t, "https://example.com")
		if !strings.Contains(errS, "Error: 读取网页失败") {
			t.Errorf("stderr = %q, want error prefix", errS)
		}
	})
}
