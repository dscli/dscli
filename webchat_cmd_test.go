package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
// webchatOptionsFromFlags reads (keep/attach/role), matching the real
// command's defaults - the contract this test locks. There is no shell flag:
// the shell channel is always on.
func newWebchatOptionsCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "webchat"}
	cmd.Flags().String("keep", "", "")
	cmd.Flags().StringSlice("attach", nil, "")
	cmd.Flags().String("role", "", "")
	return cmd
}

func TestWebchatOptionsFromFlags(t *testing.T) {
	// Defaults: Role "" = plain chat (no role prompt injection); ShellTool
	// is not a flag - the shell channel is always on.
	cmd := newWebchatOptionsCmd()
	opts, err := webchatOptionsFromFlags(cmd)
	if err != nil {
		t.Fatalf("webchatOptionsFromFlags(defaults): %v", err)
	}
	if opts.Role != "" {
		t.Errorf("default Role = %q, want \"\" (plain chat)", opts.Role)
	}
	if !opts.ShellTool {
		t.Error("ShellTool = false, want true (the shell channel is always on)")
	}
	if opts.Keep != "" || len(opts.Attachments) != 0 {
		t.Errorf("default keep/attachments should be empty, got %+v", opts)
	}

	// --role review passes through.
	cmd = newWebchatOptionsCmd()
	if err := cmd.Flags().Set("role", "review"); err != nil {
		t.Fatal(err)
	}
	if opts, err := webchatOptionsFromFlags(cmd); err != nil || opts.Role != "review" {
		t.Errorf("Role = %q, err = %v; want review", opts.Role, err)
	}

	// An explicit empty --role= is plain chat: no role prompt injection.
	cmd = newWebchatOptionsCmd()
	if err := cmd.Flags().Set("role", ""); err != nil {
		t.Fatal(err)
	}
	if opts, err := webchatOptionsFromFlags(cmd); err != nil || opts.Role != "" {
		t.Errorf("empty Role = %q, err = %v; want \"\" (plain chat)", opts.Role, err)
	}

	// keep/attach pass through unchanged.
	cmd = newWebchatOptionsCmd()
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
	if opts.Keep != "abc" ||
		len(opts.Attachments) != 1 || opts.Attachments[0] != "shot.png" {
		t.Errorf("options = %+v, want keep=abc attach=[shot.png]", opts)
	}
}

// TestWebchatOptionsShellAlwaysOn locks the no-flag contract: the shell
// channel is not switchable on the webchat CLI - ShellTool must be true
// with defaults and stay true when a role is set.
func TestWebchatOptionsShellAlwaysOn(t *testing.T) {
	cmd := newWebchatOptionsCmd()
	opts, err := webchatOptionsFromFlags(cmd)
	if err != nil {
		t.Fatalf("webchatOptionsFromFlags(defaults): %v", err)
	}
	if !opts.ShellTool {
		t.Error("ShellTool = false, want true (always on)")
	}

	cmd = newWebchatOptionsCmd()
	if err := cmd.Flags().Set("role", "dev"); err != nil {
		t.Fatal(err)
	}
	if opts, err = webchatOptionsFromFlags(cmd); err != nil {
		t.Fatalf("webchatOptionsFromFlags(--role dev): %v", err)
	}
	if !opts.ShellTool {
		t.Error("ShellTool = false with --role dev, want true")
	}
}

// TestWebchatCmdHasNoShellFlag locks the CLI surface: the real webchat
// command must not expose --shell - the channel is not optional
// (docs/task-shell-block.md).
func TestWebchatCmdHasNoShellFlag(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"webchat"})
	if err != nil || cmd == nil || cmd.Name() != "webchat" {
		t.Fatalf("webchat command not found under rootCmd (cmd=%v, err=%v)", cmd, err)
	}
	if f := cmd.Flags().Lookup("shell"); f != nil {
		t.Errorf("webchat must not expose --shell (the shell channel is always on); found: %+v", f)
	}
}

// TestWebchatKeepTakesExplicitValue pins the --keep grammar on the REAL
// command: like dscli chat's flags, --keep takes an explicit value, so both
// "--keep <id>" and "--keep=<id>" set it while the message stays a positional
// argument. Regression: the old NoOptDefVal ("last") fallback left the flag
// unset when the value was written with a space, silently sending the
// conversation ID as the message body.
func TestWebchatKeepTakesExplicitValue(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"webchat"})
	if err != nil || cmd == nil || cmd.Name() != "webchat" {
		t.Fatalf("webchat command not found under rootCmd (cmd=%v, err=%v)", cmd, err)
	}
	t.Cleanup(func() {
		// rootCmd.Find returns the shared command: reset the flag this test
		// parsed so later tests still see the default (new conversation).
		_ = cmd.Flags().Set("keep", "")
	})

	const id = "ff6ac088-4397-4f55-9c17-17cc5fb6719a"

	// "--keep <id> <message>": the id is the flag value (not the message) and
	// the space-joined positionals are the message, as in dscli chat.
	if err := cmd.ParseFlags([]string{"--keep", id, "继续", "讨论"}); err != nil {
		t.Fatalf("ParseFlags(--keep <id> <message>): %v", err)
	}
	opts, err := webchatOptionsFromFlags(cmd)
	if err != nil {
		t.Fatalf("webchatOptionsFromFlags: %v", err)
	}
	if opts.Keep != id {
		t.Errorf("Keep = %q, want %q (the space form must set the value)", opts.Keep, id)
	}
	msg, err := gatherWebchatInput(cmd, cmd.Flags().Args())
	if err != nil {
		t.Fatalf("gatherWebchatInput: %v", err)
	}
	if msg != "继续 讨论" {
		t.Errorf("message = %q, want %q", msg, "继续 讨论")
	}

	// "--keep=<id>": the equals form sets the same value.
	if err := cmd.ParseFlags([]string{"--keep=" + id, "继续"}); err != nil {
		t.Fatalf("ParseFlags(--keep=<id>): %v", err)
	}
	if opts, err = webchatOptionsFromFlags(cmd); err != nil || opts.Keep != id {
		t.Errorf("Keep = %q, err = %v; want %q", opts.Keep, err, id)
	}

	// "--keep=last": "last" is an explicit value, not a bare-flag default.
	if err := cmd.ParseFlags([]string{"--keep=last"}); err != nil {
		t.Fatalf("ParseFlags(--keep=last): %v", err)
	}
	if opts, err = webchatOptionsFromFlags(cmd); err != nil || opts.Keep != "last" {
		t.Errorf("Keep = %q, err = %v; want \"last\"", opts.Keep, err)
	}

	// A bare --keep without a value is a hard error, not a silent "last".
	if err := cmd.ParseFlags([]string{"--keep"}); err == nil {
		t.Error("ParseFlags(--keep) must fail: --keep needs an explicit value")
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
