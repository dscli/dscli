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
	input  string
	role   string
	system string
}

// captureAskExpert replaces askExpertWithRoleFunc with a recording mock and
// restores the previous value after the test. The mock still returns
// "[MOCK]" so tests that don't care about the call still behave.
func captureAskExpert(t *testing.T) *capturedCall {
	t.Helper()
	orig := askExpertWithRoleFunc
	calls := &capturedCall{}
	askExpertWithRoleFunc = func(_ context.Context, input, role, system string) (string, error) {
		calls.input = input
		calls.role = role
		calls.system = system
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
	for _, key := range []string{"content", "content_file", "summary", "attachments", "role", "system", "timeout"} {
		if _, ok := params[key]; !ok {
			t.Errorf("tool parameters missing %q", key)
		}
	}
	required, _ := askExpertTool.Parameters["required"].([]string)
	for _, key := range required {
		if key == "content" {
			t.Error("content must not be required: content or content_file must be provided")
		}
	}
	// The schema must not forbid the new optional parameters.
	if addProps, _ := askExpertTool.Parameters["additionalProperties"].(bool); addProps {
		t.Error("additionalProperties must be false for a strict schema")
	}
}

func TestHandleAskExpertDefaults(t *testing.T) {
	calls := captureAskExpert(t)
	ctx := context.Background()
	args := toolcall.ToolArgs{"content": "How do I design a retry policy?"}

	result, _, err := handleAskExpert(ctx, args)
	if err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if result != "[MOCK]" {
		t.Errorf("result = %q, want [MOCK]", result)
	}
	if calls.role != "expert" {
		t.Errorf("role = %q, want default expert", calls.role)
	}
	if calls.system != "" {
		t.Errorf("system = %q, want empty", calls.system)
	}
	if !strings.Contains(calls.input, "How do I design a retry policy?") {
		t.Errorf("input does not contain content: %q", calls.input)
	}
}

func TestHandleAskExpertWithRole(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"content": "Grade this math exam and tag knowledge points.",
		"role":    "teacher",
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if calls.role != "teacher" {
		t.Errorf("role = %q, want teacher", calls.role)
	}
	if calls.system != "" {
		t.Errorf("system = %q, want empty when only role is set", calls.system)
	}
}

func TestHandleAskExpertWithSystem(t *testing.T) {
	calls := captureAskExpert(t)
	system := "You are a senior math teacher with 10+ years of experience."
	args := toolcall.ToolArgs{
		"content": "Grade this exam.",
		"system":  system,
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if calls.system != system {
		t.Errorf("system = %q, want %q", calls.system, system)
	}
	// system wins: role falls back to default but is passed through.
	if calls.role != "expert" {
		t.Errorf("role = %q, want default expert", calls.role)
	}
}

func TestHandleAskExpertRoleAndSystem(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"content": "Question",
		"role":    "teacher",
		"system":  "Custom system prompt",
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	// Both reach the web chat layer; askExpertWebChat gives system priority.
	if calls.role != "teacher" {
		t.Errorf("role = %q, want teacher", calls.role)
	}
	if calls.system != "Custom system prompt" {
		t.Errorf("system = %q, want custom prompt", calls.system)
	}
}

func TestHandleAskExpertContentFile(t *testing.T) {
	calls := captureAskExpert(t)
	content := "Question 7: compute cos(-2040°).\nQuestion 8: complex modulus.\n"
	name := tempFileInCwd(t, content)
	args := toolcall.ToolArgs{"content_file": name}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	// The exact disk content must be sent, byte for byte (no transcription).
	if !strings.Contains(calls.input, content) {
		t.Errorf("input does not contain exact file content:\n%q", calls.input)
	}
	if calls.role != "expert" {
		t.Errorf("role = %q, want default expert", calls.role)
	}
}

func TestHandleAskExpertContentAndFileMutuallyExclusive(t *testing.T) {
	captureAskExpert(t)
	name := tempFileInCwd(t, "file content")
	args := toolcall.ToolArgs{
		"content":      "inline content",
		"content_file": name,
	}

	_, _, err := handleAskExpert(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for content + content_file, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want mutual-exclusion message", err)
	}
}

func TestHandleAskExpertNoContent(t *testing.T) {
	captureAskExpert(t)
	_, _, err := handleAskExpert(context.Background(), toolcall.ToolArgs{})
	if err == nil {
		t.Fatal("expected error for empty content and content_file, got nil")
	}
}

func TestHandleAskExpertEmptyRole(t *testing.T) {
	calls := captureAskExpert(t)
	// The LLM may send an explicit empty role string, which must not bypass
	// the "expert" default (ToolArgsValue only defaults on missing keys).
	args := toolcall.ToolArgs{
		"content": "Question",
		"role":    "",
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if calls.role != "expert" {
		t.Errorf("role = %q, want expert default for explicit empty string", calls.role)
	}
}

func TestHandleAskExpertContentFileUnsafePath(t *testing.T) {
	captureAskExpert(t)
	args := toolcall.ToolArgs{"content_file": "../etc/passwd"}
	_, _, err := handleAskExpert(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for unsafe path, got nil")
	}
	if !strings.Contains(err.Error(), "unsafe path") {
		t.Errorf("error = %q, want unsafe path message", err)
	}
}

func TestHandleAskExpertContentFileMissing(t *testing.T) {
	captureAskExpert(t)
	args := toolcall.ToolArgs{"content_file": "no-such-file-xyz.txt"}
	_, _, err := handleAskExpert(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to open") {
		t.Errorf("error = %q, want open failure message", err)
	}
}

func TestHandleAskExpertContentFileEmpty(t *testing.T) {
	captureAskExpert(t)
	name := tempFileInCwd(t, "   \n\t ")
	args := toolcall.ToolArgs{"content_file": name}
	_, _, err := handleAskExpert(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for whitespace-only file, got nil")
	}
	if !strings.Contains(err.Error(), "file is empty") {
		t.Errorf("error = %q, want empty file message", err)
	}
}

func TestHandleAskExpertContentFileTooLarge(t *testing.T) {
	captureAskExpert(t)
	big := strings.Repeat("x", maxAttachmentSize+1)
	name := tempFileInCwd(t, big)
	args := toolcall.ToolArgs{"content_file": name}
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
	// Target outside cwd: /etc/hostname on Linux; skip if not creatable.
	outside := t.TempDir() // /tmp/... — outside cwd
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

func TestAskExpertCustom(t *testing.T) {
	calls := captureAskExpert(t)
	ctx := context.Background()

	if _, err := AskExpertCustom(ctx, "input", "teacher", ""); err != nil {
		t.Fatalf("AskExpertCustom(role): %v", err)
	}
	if calls.role != "teacher" || calls.system != "" {
		t.Errorf("AskExpertCustom(role): got role=%q system=%q", calls.role, calls.system)
	}

	if _, err := AskExpertCustom(ctx, "input", "", "system text"); err != nil {
		t.Fatalf("AskExpertCustom(system): %v", err)
	}
	// Empty role falls back to expert; system is passed through.
	if calls.role != "expert" || calls.system != "system text" {
		t.Errorf("AskExpertCustom(system): got role=%q system=%q", calls.role, calls.system)
	}
}
