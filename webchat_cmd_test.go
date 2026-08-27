package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/lp"
	"github.com/spf13/cobra"
)

// newWebchatCmd builds a webchat command with the --input flag registered,
// matching the default "-" used by the real command.
func newWebchatCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "webchat"}
	cmd.Flags().String("input", "-", "")
	return cmd
}

func TestGatherWebchatInputArgs(t *testing.T) {
	cmd := newWebchatCmd()
	got, err := gatherWebchatInput(cmd, []string{"hello"})
	if err != nil {
		t.Fatalf("gatherWebchatInput(args) error: %v", err)
	}
	if got != "hello" {
		t.Errorf("gatherWebchatInput(args) = %q, want %q", got, "hello")
	}
}

func TestGatherWebchatInputFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(f, []byte("  file message\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newWebchatCmd()
	if err := cmd.Flags().Set("input", f); err != nil {
		t.Fatal(err)
	}
	got, err := gatherWebchatInput(cmd, nil)
	if err != nil {
		t.Fatalf("gatherWebchatInput(file) error: %v", err)
	}
	if got != "file message" {
		t.Errorf("gatherWebchatInput(file) = %q, want %q", got, "file message")
	}
}

func TestGatherWebchatInputStdin(t *testing.T) {
	// The default --input "-" must read a piped stdin (echo ... | dscli webchat).
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	if _, err := w.WriteString("  piped message\n"); err != nil {
		t.Fatal(err)
	}
	w.Close()

	cmd := newWebchatCmd()
	got, err := gatherWebchatInput(cmd, nil)
	if err != nil {
		t.Fatalf("gatherWebchatInput(stdin) error: %v", err)
	}
	if got != "piped message" {
		t.Errorf("gatherWebchatInput(stdin) = %q, want %q", got, "piped message")
	}
}

func TestGatherWebchatInputStdinEmpty(t *testing.T) {
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	w.Close()

	cmd := newWebchatCmd()
	if _, err := gatherWebchatInput(cmd, nil); err == nil {
		t.Error("gatherWebchatInput(empty stdin) must fail")
	}
}

// newWebchatOptionsCmd builds a webchat command with the flags
// webchatOptionsFromFlags reads (keep/mode/attach/role), matching the real
// command's defaults - the contract this test locks.
func newWebchatOptionsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "webchat"}
	var keep string
	keepFlag := cmd.Flags().VarPF(&keepValue{&keep}, "keep", "", "")
	keepFlag.NoOptDefVal = "last"
	cmd.Flags().String("mode", "", "")
	cmd.Flags().StringSlice("attach", nil, "")
	cmd.Flags().String("role", "dev", "")
	cmd.Flags().Bool("plain", false, "")
	return cmd
}

func TestWebchatOptionsFromFlags(t *testing.T) {
	// Defaults: role "dev" (mirrors dscli chat), everything else empty.
	cmd := newWebchatOptionsCmd()
	opts, err := webchatOptionsFromFlags(cmd)
	if err != nil {
		t.Fatalf("webchatOptionsFromFlags(defaults): %v", err)
	}
	if opts.Role != "dev" {
		t.Errorf("default Role = %q, want %q (match dscli chat)", opts.Role, "dev")
	}
	if opts.Mode != "" || opts.Keep != "" || len(opts.Attachments) != 0 {
		t.Errorf("default options should be empty, got %+v", opts)
	}

	// --role review passes through.
	cmd = newWebchatOptionsCmd()
	if err := cmd.Flags().Set("role", "review"); err != nil {
		t.Fatal(err)
	}
	if opts, err := webchatOptionsFromFlags(cmd); err != nil || opts.Role != "review" {
		t.Errorf("Role = %q, err = %v; want review", opts.Role, err)
	}

	// An explicit empty --role= normalizes to "dev", exactly like dscli
	// chat (chat.go does role == "" -> "dev").
	cmd = newWebchatOptionsCmd()
	if err := cmd.Flags().Set("role", ""); err != nil {
		t.Fatal(err)
	}
	if opts, err := webchatOptionsFromFlags(cmd); err != nil || opts.Role != "dev" {
		t.Errorf("empty Role = %q, err = %v; want dev (match dscli chat)", opts.Role, err)
	}

	// --plain is the explicit plain-chat escape hatch: Role "" means
	// verbatim send, no DSML loop in HandleWebChat. It wins over --role.
	cmd = newWebchatOptionsCmd()
	if err := cmd.Flags().Set("plain", "true"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("role", "review"); err != nil {
		t.Fatal(err)
	}
	if opts, err := webchatOptionsFromFlags(cmd); err != nil || opts.Role != "" {
		t.Errorf("--plain Role = %q, err = %v; want empty (plain chat)", opts.Role, err)
	}

	// mode/keep/attach pass through unchanged.
	cmd = newWebchatOptionsCmd()
	if err := cmd.Flags().Set("mode", "vision"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("keep", "abc"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("attach", "shot.png"); err != nil {
		t.Fatal(err)
	}
	opts, err = webchatOptionsFromFlags(cmd)
	if err != nil {
		t.Fatalf("webchatOptionsFromFlags(all flags): %v", err)
	}
	if opts.Mode != lp.Mode("vision") || opts.Keep != "abc" ||
		len(opts.Attachments) != 1 || opts.Attachments[0] != "shot.png" {
		t.Errorf("options = %+v, want mode=vision keep=abc attach=[shot.png]", opts)
	}
}

func TestFormatConversationHint(t *testing.T) {
	const id = "abc-123_XYZ"
	url := "https://chat.deepseek.com/a/chat/s/" + id

	hint := formatConversationHint(url)
	if !strings.Contains(hint, "keep:"+id) {
		t.Errorf("hint missing keep:<id>, got: %q", hint)
	}
	if !strings.Contains(hint, "--keep="+id) {
		t.Errorf("hint missing copy-paste --keep=<id> command, got: %q", hint)
	}

	// A non-DeepSeek URL (no extractable ID) falls back to the raw URL.
	raw := "https://example.com/other"
	if hint := formatConversationHint(raw); !strings.Contains(hint, raw) {
		t.Errorf("hint should contain the raw URL, got: %q", hint)
	}

	// An empty URL yields no hint at all (defensive; callers guard too).
	if hint := formatConversationHint(""); hint != "" {
		t.Errorf("hint for empty URL should be empty, got: %q", hint)
	}
}
