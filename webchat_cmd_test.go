package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/outfmt"
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
	// An empty stdin is NOT rejected here: webchatRunE decides what it means
	// - with --keep it requests a resume, without it is a usage error.
	oldStdin := os.Stdin
	t.Cleanup(func() { os.Stdin = oldStdin })
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdin = r
	w.Close()

	cmd := newWebchatCmd()
	got, err := gatherWebchatInput(cmd, nil)
	if err != nil {
		t.Fatalf("gatherWebchatInput(empty stdin) must not fail here: %v", err)
	}
	if got != "" {
		t.Errorf("gatherWebchatInput(empty stdin) = %q, want empty", got)
	}
}

func TestGatherWebchatInputFileEmpty(t *testing.T) {
	f := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(f, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := newWebchatCmd()
	if err := cmd.Flags().Set("input", f); err != nil {
		t.Fatal(err)
	}
	got, err := gatherWebchatInput(cmd, nil)
	if err != nil {
		t.Fatalf("gatherWebchatInput(empty file) must not fail here: %v", err)
	}
	if got != "" {
		t.Errorf("gatherWebchatInput(empty file) = %q, want empty", got)
	}
}

func TestValidateWebchatInvocation(t *testing.T) {
	// Empty message + --keep = resume; empty without --keep is a usage
	// error; attachments never ride along with a resume.
	tests := []struct {
		name        string
		keep        string
		message     string
		attachments []string
		wantErr     bool
	}{
		{"message with keep", "abc", "继续", nil, false},
		{"message without keep", "", "新会话", nil, false},
		{"empty message with keep = resume", "abc", "", nil, false},
		{"empty message without keep", "", "", nil, true},
		{"attachments need a message", "abc", "", []string{"shot.png"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateWebchatInvocation(tt.keep, tt.message, tt.attachments)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateWebchatInvocation(%q, %q, %v) err = %v, wantErr %v",
					tt.keep, tt.message, tt.attachments, err, tt.wantErr)
			}
		})
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
	// Defaults: Role "" = plain chat (no role prompt injection); the
	// <shell> channel has no flag - the tool loop is always on.
	cmd := newWebchatOptionsCmd()
	opts, err := webchatOptionsFromFlags(cmd)
	if err != nil {
		t.Fatalf("webchatOptionsFromFlags(defaults): %v", err)
	}
	if opts.Role != "" {
		t.Errorf("default Role = %q, want \"\" (plain chat)", opts.Role)
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

// TestWebchatCmdHasNoShellFlag locks the CLI surface: the real webchat
// command must not expose --shell - the <shell> tool loop is always on and
// is not switchable.
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

// newWebchatRunCmd builds a webchat command carrying every flag webchatRunE
// reads (input/keep/attach/role), with --input pointing at inputFile. The
// context is set explicitly: webchatRunE opens a clog span on cmd.Context(),
// and cobra leaves it nil on a hand-built command.
func newWebchatRunCmd(inputFile string) *cobra.Command {
	cmd := &cobra.Command{Use: "webchat"}
	cmd.Flags().String("input", inputFile, "")
	cmd.Flags().String("keep", "", "")
	cmd.Flags().StringSlice("attach", nil, "")
	cmd.Flags().String("role", "", "")
	cmd.SetContext(context.Background())
	return cmd
}

// captureWebchatOutput routes outfmt through a buffer in the given mode and
// restores the globals on cleanup (same convention as chat_test.go /
// version_test.go).
func captureWebchatOutput(t *testing.T, mode string) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	outfmt.SetOutputWriter(&buf)
	outfmt.SetOutputMode(mode)
	t.Cleanup(func() {
		outfmt.SetOutputWriter(os.Stdout)
		outfmt.SetOutputMode("markdown")
	})
	return &buf
}

// TestWebchatRunEReplyGoesThroughOutfmt locks issue #37 for the send path: a
// one-shot prose reply (WebChatResult.Printed == false) must be printed
// through outfmt, so --org converts it like every tool-loop round does.
func TestWebchatRunEReplyGoesThroughOutfmt(t *testing.T) {
	f := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(f, []byte("## R"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldSend := webchatSendFunc
	t.Cleanup(func() { webchatSendFunc = oldSend })
	webchatSendFunc = func(ctx context.Context, message string, opts lp.WebChatOptions) (lp.WebChatResult, error) {
		return lp.WebChatResult{Content: "## R"}, nil
	}

	for _, tt := range []struct{ mode, want, unwanted string }{
		{"org", "** R", "## R"},
		{"markdown", "## R", "** R"},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			buf := captureWebchatOutput(t, tt.mode)
			if err := webchatRunE(newWebchatRunCmd(f), nil); err != nil {
				t.Fatalf("webchatRunE: %v", err)
			}
			got := buf.String()
			if !strings.Contains(got, tt.want) {
				t.Errorf("%s mode: output misses %q:\n%s", tt.mode, tt.want, got)
			}
			if strings.Contains(got, tt.unwanted) {
				t.Errorf("%s mode: output must not contain %q:\n%s", tt.mode, tt.unwanted, got)
			}
		})
	}
}

// TestWebchatRunESkipsAlreadyPrintedReply pins the Printed contract: a reply
// already rendered by the tool loop (outfmt.PrintContent) must not be
// re-printed by webchatRunE.
func TestWebchatRunESkipsAlreadyPrintedReply(t *testing.T) {
	f := filepath.Join(t.TempDir(), "msg.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldSend := webchatSendFunc
	t.Cleanup(func() { webchatSendFunc = oldSend })
	webchatSendFunc = func(ctx context.Context, message string, opts lp.WebChatOptions) (lp.WebChatResult, error) {
		return lp.WebChatResult{Content: "## SKIPPED", Printed: true}, nil
	}

	buf := captureWebchatOutput(t, "org")
	if err := webchatRunE(newWebchatRunCmd(f), nil); err != nil {
		t.Fatalf("webchatRunE: %v", err)
	}
	if got := buf.String(); strings.Contains(got, "SKIPPED") {
		t.Errorf("a Printed reply must not be re-printed:\n%s", got)
	}
}

// TestRunWebchatResumeGoesThroughOutfmt locks issue #37 for the resume path:
// the verbatim re-print of the conversation's last reply must go through
// outfmt too, so --org converts it.
func TestRunWebchatResumeGoesThroughOutfmt(t *testing.T) {
	oldResume := webchatResumeFunc
	t.Cleanup(func() { webchatResumeFunc = oldResume })
	webchatResumeFunc = func(ctx context.Context, opts lp.WebChatOptions) (lp.WebChatResult, error) {
		return lp.WebChatResult{Content: "## T\n\n**b**"}, nil
	}

	for _, tt := range []struct{ mode, want, unwanted string }{
		{"org", "** T", "## T"},
		{"markdown", "## T", "** T"},
	} {
		t.Run(tt.mode, func(t *testing.T) {
			buf := captureWebchatOutput(t, tt.mode)
			if err := runWebchatResume(t.Context(), lp.WebChatOptions{}); err != nil {
				t.Fatalf("runWebchatResume: %v", err)
			}
			got := buf.String()
			if !strings.Contains(got, tt.want) {
				t.Errorf("%s mode: output misses %q:\n%s", tt.mode, tt.want, got)
			}
			if strings.Contains(got, tt.unwanted) {
				t.Errorf("%s mode: output must not contain %q:\n%s", tt.mode, tt.unwanted, got)
			}
		})
	}
}
