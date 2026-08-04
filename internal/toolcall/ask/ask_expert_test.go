package ask

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

// capturedCall records the arguments passed to askExpertWithRoleFunc.
type capturedCall struct {
	input       string
	role        string
	system      string
	mode        string
	attachments []string
}

// captureAskExpert replaces askExpertWithRoleFunc with a recording mock and
// restores the previous value after the test. The mock still returns
// "[MOCK]" so tests that don't care about the call still behave.
func captureAskExpert(t *testing.T) *capturedCall {
	t.Helper()
	orig := askExpertWithRoleFunc
	calls := &capturedCall{}
	askExpertWithRoleFunc = func(_ context.Context, input, role, system, mode string, attachments []string) (string, error) {
		calls.input = input
		calls.role = role
		calls.system = system
		calls.mode = mode
		calls.attachments = attachments
		return "[MOCK]", nil
	}
	t.Cleanup(func() { askExpertWithRoleFunc = orig })
	return calls
}

// tempFileInCwd creates a file under the current working directory (isSafePath
// only allows cwd and subdirectories) and returns its relative name.
func tempFileInCwd(t *testing.T, content string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.CreateTemp(cwd, "askexpert-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(f.Name())
	t.Cleanup(func() { os.Remove(f.Name()) })
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return name
}

func TestAskExpertToolParameters(t *testing.T) {
	params, _ := askExpertTool.Parameters["properties"].(map[string]any)
	if params == nil {
		t.Fatal("tool parameters missing properties")
	}
	for _, key := range []string{"input", "attachments", "mode", "timeout"} {
		if _, ok := params[key]; !ok {
			t.Errorf("tool parameters missing %q", key)
		}
	}
	for _, key := range []string{"content", "content_file", "summary", "role", "system"} {
		if _, ok := params[key]; ok {
			t.Errorf("tool parameter %q must be removed", key)
		}
	}
	required, _ := askExpertTool.Parameters["required"].([]string)
	if len(required) != 1 || required[0] != "input" {
		t.Errorf("required = %v, want [input]", required)
	}
	// The schema must not forbid the new optional parameters.
	if addProps, _ := askExpertTool.Parameters["additionalProperties"].(bool); addProps {
		t.Error("additionalProperties must be false for a strict schema")
	}
}

func TestHandleAskExpertDefaults(t *testing.T) {
	calls := captureAskExpert(t)
	ctx := context.Background()
	args := toolcall.ToolArgs{"input": "How do I design a retry policy?"}

	result, _, err := handleAskExpert(ctx, args)
	if err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if result != "[MOCK]" {
		t.Errorf("result = %q, want [MOCK]", result)
	}
	// No persona is injected: role and system must be empty.
	if calls.role != "" {
		t.Errorf("role = %q, want empty", calls.role)
	}
	if calls.system != "" {
		t.Errorf("system = %q, want empty", calls.system)
	}
	if !strings.Contains(calls.input, "How do I design a retry policy?") {
		t.Errorf("input does not contain content: %q", calls.input)
	}
}

func TestHandleAskExpertAtFile(t *testing.T) {
	calls := captureAskExpert(t)
	content := "Question 7: compute cos(-2040°).\nQuestion 8: complex modulus.\n"
	name := tempFileInCwd(t, content)
	args := toolcall.ToolArgs{"input": "@" + name}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	// The exact disk content must be sent, byte for byte (no transcription).
	if !strings.Contains(calls.input, content) {
		t.Errorf("input does not contain exact file content:\n%q", calls.input)
	}
}

func TestHandleAskExpertAtPrefixTreatedAsText(t *testing.T) {
	calls := captureAskExpert(t)
	// Natural language starting with @ must not be treated as a file.
	args := toolcall.ToolArgs{"input": "@user 你怎么看这个方案"}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if !strings.Contains(calls.input, "@user 你怎么看这个方案") {
		t.Errorf("input = %q, want @-prefixed text passed through", calls.input)
	}
}

func TestHandleAskExpertAtMissingFileTreatedAsText(t *testing.T) {
	calls := captureAskExpert(t)
	// The file does not exist: lenient fallback sends the text verbatim.
	args := toolcall.ToolArgs{"input": "@no-such-file-xyz.txt"}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if !strings.Contains(calls.input, "@no-such-file-xyz.txt") {
		t.Errorf("input = %q, want @-prefixed text passed through", calls.input)
	}
}

func TestHandleAskExpertAtUnsafePathTreatedAsText(t *testing.T) {
	calls := captureAskExpert(t)
	// Unsafe paths are never read; they are sent as plain text.
	args := toolcall.ToolArgs{"input": "@../etc/passwd"}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if !strings.Contains(calls.input, "@../etc/passwd") {
		t.Errorf("input = %q, want @-prefixed text passed through", calls.input)
	}
}

func TestHandleAskExpertNoInput(t *testing.T) {
	captureAskExpert(t)
	for name, args := range map[string]toolcall.ToolArgs{
		"missing": {},
		"empty":   {"input": ""},
		"blank":   {"input": "   "},
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := handleAskExpert(context.Background(), args)
			if err == nil {
				t.Fatal("expected error for empty input, got nil")
			}
			if !strings.Contains(err.Error(), "input is required") {
				t.Errorf("error = %q, want input-required message", err)
			}
		})
	}
}

func TestHandleAskExpertAtFileEmpty(t *testing.T) {
	captureAskExpert(t)
	name := tempFileInCwd(t, "   \n\t ")
	args := toolcall.ToolArgs{"input": "@" + name}

	_, _, err := handleAskExpert(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for whitespace-only file, got nil")
	}
	if !strings.Contains(err.Error(), "file is empty") {
		t.Errorf("error = %q, want empty file message", err)
	}
}

func TestHandleAskExpertAtFileTooLarge(t *testing.T) {
	captureAskExpert(t)
	big := strings.Repeat("x", maxAttachmentSize+1)
	name := tempFileInCwd(t, big)
	args := toolcall.ToolArgs{"input": "@" + name}

	_, _, err := handleAskExpert(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for oversized file, got nil")
	}
	if !strings.Contains(err.Error(), "file too large") {
		t.Errorf("error = %q, want size-limit message", err)
	}
}

func TestReadContentFile(t *testing.T) {
	content := "exact question text"
	name := tempFileInCwd(t, content)

	got, err := readContentFile(name)
	if err != nil {
		t.Fatalf("readContentFile: %v", err)
	}
	if got != content {
		t.Errorf("readContentFile = %q, want %q", got, content)
	}

	if _, err := readContentFile("../blocked.txt"); err == nil {
		t.Error("expected unsafe path error")
	}
}

func TestReadContentFileSymlinkOutsideCwd(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// Target outside cwd: /tmp/... is outside the test working directory.
	outside := t.TempDir()
	target := filepath.Join(outside, "target.txt")
	if err := os.WriteFile(target, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(cwd, "askexpert-symlink-test.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink: %v", err)
	}
	t.Cleanup(func() { os.Remove(link) })

	_, err = readContentFile(filepath.Base(link))
	if err == nil {
		t.Fatal("expected error for symlink resolving outside cwd, got nil")
	}
	if !strings.Contains(err.Error(), "outside the current directory") {
		t.Errorf("error = %q, want outside-cwd message", err)
	}
}

// tempImageInCwd creates a fake image file (extension only) under cwd and
// returns its relative name. The content is never read for uploads, so any
// bytes pass.
func tempImageInCwd(t *testing.T, name string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cwd, name)
	if err := os.WriteFile(path, []byte("fake image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(path) })
	return name
}

func TestHandleAskExpertImageAttachment(t *testing.T) {
	calls := captureAskExpert(t)
	img := tempImageInCwd(t, "askexpert-test-screenshot.png")
	args := toolcall.ToolArgs{
		"input":       "What does this screenshot show?",
		"attachments": []string{img},
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if len(calls.attachments) != 1 || calls.attachments[0] != img {
		t.Errorf("attachments = %v, want [%s]", calls.attachments, img)
	}
	if strings.Contains(calls.input, "fake image bytes") {
		t.Error("image content must not be inlined into the request text")
	}
}

func TestHandleAskExpertMixedAttachments(t *testing.T) {
	calls := captureAskExpert(t)
	img := tempImageInCwd(t, "askexpert-test-mixed.png")
	text := tempFileInCwd(t, "text attachment content")
	args := toolcall.ToolArgs{
		"input":       "Question",
		"attachments": []string{img, text},
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if len(calls.attachments) != 1 || calls.attachments[0] != img {
		t.Errorf("attachments = %v, want only the image [%s]", calls.attachments, img)
	}
	if !strings.Contains(calls.input, "text attachment content") {
		t.Error("non-image attachment must be inlined as text")
	}
}

func TestHandleAskExpertUnsafeImageAttachment(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"input":       "Question",
		"attachments": []string{"../outside.png"},
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if len(calls.attachments) != 0 {
		t.Errorf("attachments = %v, want none (unsafe path must be dropped)", calls.attachments)
	}
}

func TestHandleAskExpertMode(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"input": "Question",
		"mode":  "flash",
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if calls.mode != "flash" {
		t.Errorf("mode = %q, want flash", calls.mode)
	}
}

func TestHandleAskExpertModeDefaultsToEmpty(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{"input": "Question"}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if calls.mode != "" {
		t.Errorf("mode = %q, want empty (auto-select in web chat layer)", calls.mode)
	}
}
