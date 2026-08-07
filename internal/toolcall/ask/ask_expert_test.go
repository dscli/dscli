package ask

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/lp"
	"github.com/dscli/dscli/internal/toolcall"
)

// capturedCall records the arguments passed to askExpertWithRoleFunc.
type capturedCall struct {
	input       string
	role        string
	system      string
	mode        string
	keep        string
	attachments []string
}

// captureAskExpert replaces askExpertWithRoleFunc with a recording mock and
// restores the previous value after the test. The mock still returns
// "[MOCK]" so tests that don't care about the call still behave.
func captureAskExpert(t *testing.T) *capturedCall {
	t.Helper()
	orig := askExpertWithRoleFunc
	calls := &capturedCall{}
	askExpertWithRoleFunc = func(_ context.Context, input, role, system, mode, keep string, attachments []string) (string, string, error) {
		calls.input = input
		calls.role = role
		calls.system = system
		calls.mode = mode
		calls.keep = keep
		calls.attachments = attachments
		return "[MOCK]", "", nil
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
	for _, key := range []string{"input", "attachments", "mode", "keep", "timeout"} {
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
	// Target outside the sandbox (cwd/home/temp): a real system file.
	target := outsideSandboxPath(t)
	if target == "" {
		t.Skip("no candidate file outside cwd/home/temp found")
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

	if _, _, err := handleAskExpert(context.Background(), args); err == nil {
		t.Fatal("expected error for unsafe attachment, got nil")
	}
	if calls.input != "" {
		t.Errorf("expert was called with %q, want no call (unsafe path must abort)", calls.input)
	}
	if len(calls.attachments) != 0 {
		t.Errorf("attachments = %v, want none (unsafe path must abort before upload)", calls.attachments)
	}
}

func TestHandleAskExpertUnsafeInlineAttachment(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"input":       "Question",
		"attachments": []string{"../secret.txt"},
	}

	if _, _, err := handleAskExpert(context.Background(), args); err == nil {
		t.Fatal("expected error for unsafe inline attachment, got nil")
	}
	if calls.input != "" {
		t.Errorf("expert was called with %q, want no call (unsafe path must abort)", calls.input)
	}
}

func TestHandleAskExpertTempImageAttachment(t *testing.T) {
	calls := captureAskExpert(t)
	img := filepath.Join(os.TempDir(), "askexpert-test-tmp-upload.png")
	if err := os.WriteFile(img, []byte("fake image bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(img) })
	args := toolcall.ToolArgs{
		"input":       "What does this show?",
		"attachments": []string{img},
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if len(calls.attachments) != 1 || calls.attachments[0] != img {
		t.Errorf("attachments = %v, want [%s] (temp dir image must be uploadable)", calls.attachments, img)
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
	if calls.keep != "" {
		t.Errorf("keep = %q, want empty (new conversation by default)", calls.keep)
	}
}

func TestHandleAskExpertKeepPassedThrough(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"input": "再仔细看这张图，是不是白发罗小黑？",
		"keep":  "abc123def456",
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if calls.keep != "abc123def456" {
		t.Errorf("keep = %q, want abc123def456", calls.keep)
	}
}

func TestHandleAskExpertLastPassedThrough(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"input": "继续上次的话题",
		"keep":  "last",
	}

	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if calls.keep != "last" {
		t.Errorf("keep = %q, want last", calls.keep)
	}
}

func TestHandleAskExpertConversationIDInResult(t *testing.T) {
	orig := askExpertWithRoleFunc
	t.Cleanup(func() { askExpertWithRoleFunc = orig })
	askExpertWithRoleFunc = func(_ context.Context, _, _, _, _, _ string, _ []string) (string, string, error) {
		return "专家回答", "https://chat.deepseek.com/a/chat/s/conv12345", nil
	}

	result, _, err := handleAskExpert(context.Background(), toolcall.ToolArgs{"input": "Question"})
	if err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if !strings.Contains(result, "conversation_id: conv12345") {
		t.Errorf("result missing conversation_id suffix:\n%s", result)
	}
}

func TestHandleAskExpertNoConversationURL(t *testing.T) {
	// When the conversation URL is unknown, the result must not claim an ID.
	orig := askExpertWithRoleFunc
	t.Cleanup(func() { askExpertWithRoleFunc = orig })
	askExpertWithRoleFunc = func(_ context.Context, _, _, _, _, _ string, _ []string) (string, string, error) {
		return "[MOCK]", "", nil
	}

	result, _, err := handleAskExpert(context.Background(), toolcall.ToolArgs{"input": "Question"})
	if err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if strings.Contains(result, "conversation_id") {
		t.Errorf("result must not contain conversation_id when URL is unknown:\n%s", result)
	}
}

func TestHandleAskExpertKeepList(t *testing.T) {
	calls := captureAskExpert(t)
	args := toolcall.ToolArgs{
		"input": "unused",
		"keep":  "list",
	}

	result, _, err := handleAskExpert(context.Background(), args)
	if err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	// The list path must not call the expert function (no message is sent).
	if calls.input != "" {
		t.Errorf("mock was called with input %q, want no call for keep=list", calls.input)
	}
	// The registry may be empty or populated on the dev machine; either way
	// the result is a human-readable list mentioning conversations.
	if !strings.Contains(strings.ToLower(result), "conversation") {
		t.Errorf("keep=list result does not mention conversations:\n%s", result)
	}
}

// tempFileInHome creates a file under the user's home directory (allowed by
// isSafePath since the HOME sandbox was added) and returns both the ~/
// relative form and the absolute path. Skips when HOME is unavailable.
func tempFileInHome(t *testing.T, content string) (tildeName, absName string) {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skipf("cannot determine home directory: %v", err)
	}
	f, err := os.CreateTemp(home, "askexpert-home-test-*.txt")
	if err != nil {
		t.Skipf("cannot create file in home %s: %v", home, err)
	}
	name := f.Name()
	t.Cleanup(func() { os.Remove(name) })
	if _, err := f.WriteString(content); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return filepath.Join("~", filepath.Base(name)), name
}

func TestReadContentFileHomeTilde(t *testing.T) {
	content := "question from home via tilde"
	tildeName, _ := tempFileInHome(t, content)

	got, err := readContentFile(tildeName)
	if err != nil {
		t.Fatalf("readContentFile(%q): %v", tildeName, err)
	}
	if got != content {
		t.Errorf("readContentFile = %q, want %q", got, content)
	}
}

func TestReadContentFileHomeAbsolute(t *testing.T) {
	content := "question from home via absolute path"
	_, absName := tempFileInHome(t, content)

	got, err := readContentFile(absName)
	if err != nil {
		t.Fatalf("readContentFile(%q): %v", absName, err)
	}
	if got != content {
		t.Errorf("readContentFile = %q, want %q", got, content)
	}
}

func TestReadContentFileOutsideHomeRejected(t *testing.T) {
	outside := outsideSandboxPath(t)
	if outside == "" {
		t.Skip("no candidate file outside cwd/home/temp found")
	}

	if _, err := readContentFile(outside); err == nil {
		t.Fatal("expected error for absolute path outside the sandbox, got nil")
	} else if !strings.Contains(err.Error(), "unsafe path") {
		t.Errorf("error = %q, want unsafe-path message", err)
	}
}

func TestReadContentFileHomeTraversalRejected(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skipf("cannot determine home directory: %v", err)
	}
	// ~/../escapes.txt cleans to <parent-of-home>/escapes.txt, outside HOME.
	// Build the raw string: filepath.Join would Clean "~"+"/.." away first.
	escaped := "~/../askexpert-escape-test.txt"
	if _, err := readContentFile(escaped); err == nil {
		t.Fatal("expected error for ~/.. traversal, got nil")
	} else if !strings.Contains(err.Error(), "unsafe path") {
		t.Errorf("error = %q, want unsafe-path message", err)
	}
}

func TestReadContentFileHomeSymlinkEscape(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skipf("cannot determine home directory: %v", err)
	}
	target := outsideSandboxPath(t)
	if target == "" {
		t.Skip("no candidate file outside cwd/home/temp found")
	}
	link := filepath.Join(home, "askexpert-home-symlink-test.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("cannot create symlink in home: %v", err)
	}
	t.Cleanup(func() { os.Remove(link) })

	// Both the absolute and the ~/ form must be rejected: the symlink
	// resolves outside the sandbox even though its text path is inside.
	for _, name := range []string{link, filepath.Join("~", filepath.Base(link))} {
		if _, err := readContentFile(name); err == nil {
			t.Errorf("expected error for symlink %q resolving outside sandbox, got nil", name)
		} else if !strings.Contains(err.Error(), "outside the current directory, home directory, or temp directory") {
			t.Errorf("error = %q, want outside-sandbox message", err)
		}
	}
}

// TestHandleAskExpertHomeImageAttachment verifies that a ~/-referenced image
// is expanded to an absolute path before it reaches the upload layer: the
// CDP/web-chat code opens attachment files with plain os.Open, which does
// not expand a leading ~ (regression test for the HOME sandbox change).
func TestHandleAskExpertHomeImageAttachment(t *testing.T) {
	calls := captureAskExpert(t)
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skipf("cannot determine home directory: %v", err)
	}
	name := filepath.Join(home, "askexpert-home-image-test.png")
	if err := os.WriteFile(name, []byte("fake image bytes"), 0o600); err != nil {
		t.Skipf("cannot create image in home %s: %v", home, err)
	}
	t.Cleanup(func() { os.Remove(name) })

	args := toolcall.ToolArgs{
		"input":       "What does this show?",
		"attachments": []string{filepath.Join("~", filepath.Base(name))},
	}
	if _, _, err := handleAskExpert(context.Background(), args); err != nil {
		t.Fatalf("handleAskExpert: %v", err)
	}
	if len(calls.attachments) != 1 {
		t.Fatalf("attachments = %v, want exactly one", calls.attachments)
	}
	if calls.attachments[0] != name {
		t.Errorf("attachment = %q, want expanded absolute path %q", calls.attachments[0], name)
	}
}

// TestIsSafePathDotsInName verifies that names containing ".." as a
// substring (e.g. "..hidden") are not mistaken for traversal components.
func TestIsSafePathDotsInName(t *testing.T) {
	if !isSafePath("~/..hidden-file.txt") {
		t.Error("isSafePath must allow names like ..hidden-file.txt")
	}
	if isSafePath("../escape.txt") {
		t.Error("isSafePath must reject a real .. traversal component")
	}
}

// TestIsSafePathTemp verifies that the system temp directory (e.g. /tmp) is
// an allowed sandbox root, while traversal through it stays rejected.
func TestIsSafePathTemp(t *testing.T) {
	tmp := filepath.Join(os.TempDir(), "ask-expert-tmp-test.txt")
	if !isSafePath(tmp) {
		t.Errorf("isSafePath(%q) = false, want true (system temp dir must be safe)", tmp)
	}
	escaped := filepath.Join(os.TempDir(), "..", "etc", "passwd")
	if isSafePath(escaped) {
		t.Errorf("isSafePath(%q) = true, want false (traversal must be rejected)", escaped)
	}
}

// TestVerifySafePathTemp verifies that a real file under the system temp
// directory passes verification, while a real file outside cwd/home/temp
// is still rejected.
func TestVerifySafePathTemp(t *testing.T) {
	tmp := filepath.Join(os.TempDir(), "askexpert-tmp-verify-test.txt")
	if err := os.WriteFile(tmp, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(tmp) })
	if err := verifySafePath(tmp); err != nil {
		t.Errorf("verifySafePath(%q) = %v, want nil", tmp, err)
	}

	outside := outsideSandboxPath(t)
	if outside == "" {
		t.Skip("no candidate file outside cwd/home/temp found")
	}
	if err := verifySafePath(outside); err == nil {
		t.Errorf("verifySafePath(%q) = nil, want error", outside)
	}
}

// outsideSandboxPath returns an existing absolute path that lies outside the
// current directory, the home directory, and the temp directory, or "" if no
// such candidate is found on this system.
func outsideSandboxPath(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"/etc/passwd", "/etc/hosts", "/etc/resolv.conf"} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if !pathWithinDir(p, cwd) && !pathWithinDir(p, home) && !pathWithinTemp(p) {
			return p
		}
	}
	return ""
}

func TestAskExpertWebChatRetriesOnBusy(t *testing.T) {
	origFunc, origDelays := webChatFunc, askExpertRetryDelays
	t.Cleanup(func() { webChatFunc, askExpertRetryDelays = origFunc, origDelays })

	calls := 0
	webChatFunc = func(_ context.Context, _ string, _ lp.WebChatOptions) (lp.WebChatResult, error) {
		calls++
		return lp.WebChatResult{}, lp.ErrServerBusy
	}
	askExpertRetryDelays = []time.Duration{0, 0, 0}

	_, _, err := askExpertWebChat(context.Background(), "input", "expert", "", "", "", nil)
	if err == nil {
		t.Fatal("expected error for persistent server busy")
	}
	if !errors.Is(err, lp.ErrServerBusy) {
		t.Errorf("err = %v, want ErrServerBusy chain", err)
	}
	// 1 initial attempt + 3 backoff retries.
	if calls != 4 {
		t.Errorf("webChatFunc calls = %d, want 4", calls)
	}
}

func TestAskExpertWebChatRetriesThenSuccess(t *testing.T) {
	origFunc, origDelays := webChatFunc, askExpertRetryDelays
	t.Cleanup(func() { webChatFunc, askExpertRetryDelays = origFunc, origDelays })

	calls := 0
	webChatFunc = func(_ context.Context, _ string, _ lp.WebChatOptions) (lp.WebChatResult, error) {
		calls++
		if calls <= 2 {
			return lp.WebChatResult{}, lp.ErrServerBusy
		}
		return lp.WebChatResult{Text: "expert answer"}, nil
	}
	askExpertRetryDelays = []time.Duration{0, 0, 0}

	reply, _, err := askExpertWebChat(context.Background(), "input", "expert", "", "", "", nil)
	if err != nil {
		t.Fatalf("askExpertWebChat: %v", err)
	}
	if reply != "expert answer" {
		t.Errorf("reply = %q, want expert answer", reply)
	}
	if calls != 3 {
		t.Errorf("webChatFunc calls = %d, want 3", calls)
	}
}

func TestAskExpertWebChatNoRetryOnHardError(t *testing.T) {
	origFunc, origDelays := webChatFunc, askExpertRetryDelays
	t.Cleanup(func() { webChatFunc, askExpertRetryDelays = origFunc, origDelays })

	hardErr := errors.New("login required")
	calls := 0
	webChatFunc = func(_ context.Context, _ string, _ lp.WebChatOptions) (lp.WebChatResult, error) {
		calls++
		return lp.WebChatResult{}, hardErr
	}

	_, _, err := askExpertWebChat(context.Background(), "input", "expert", "", "", "", nil)
	if !errors.Is(err, hardErr) {
		t.Errorf("err = %v, want hard error passthrough", err)
	}
	if calls != 1 {
		t.Errorf("webChatFunc calls = %d, want 1 (no retry on permanent error)", calls)
	}
}

func TestAskExpertWebChatRetriesOnTruncated(t *testing.T) {
	origFunc, origDelays := webChatFunc, askExpertRetryDelays
	t.Cleanup(func() { webChatFunc, askExpertRetryDelays = origFunc, origDelays })

	calls := 0
	webChatFunc = func(_ context.Context, _ string, _ lp.WebChatOptions) (lp.WebChatResult, error) {
		calls++
		return lp.WebChatResult{}, lp.ErrTruncated
	}
	askExpertRetryDelays = []time.Duration{0, 0, 0}

	_, _, err := askExpertWebChat(context.Background(), "input", "expert", "", "", "", nil)
	if err == nil {
		t.Fatal("expected error for persistent truncation")
	}
	if !errors.Is(err, lp.ErrTruncated) {
		t.Errorf("err = %v, want ErrTruncated chain", err)
	}
	// 1 initial attempt + 3 backoff retries.
	if calls != 4 {
		t.Errorf("webChatFunc calls = %d, want 4", calls)
	}
}

func TestAskExpertWebChatTruncatedThenSuccess(t *testing.T) {
	origFunc, origDelays := webChatFunc, askExpertRetryDelays
	t.Cleanup(func() { webChatFunc, askExpertRetryDelays = origFunc, origDelays })

	calls := 0
	webChatFunc = func(_ context.Context, _ string, _ lp.WebChatOptions) (lp.WebChatResult, error) {
		calls++
		if calls <= 2 {
			return lp.WebChatResult{}, lp.ErrTruncated
		}
		return lp.WebChatResult{Text: "expert answer"}, nil
	}
	askExpertRetryDelays = []time.Duration{0, 0, 0}

	reply, _, err := askExpertWebChat(context.Background(), "input", "expert", "", "", "", nil)
	if err != nil {
		t.Fatalf("askExpertWebChat: %v", err)
	}
	if reply != "expert answer" {
		t.Errorf("reply = %q, want expert answer", reply)
	}
	if calls != 3 {
		t.Errorf("webChatFunc calls = %d, want 3", calls)
	}
}

func TestAskExpertWebChatRetryAbortsOnCancel(t *testing.T) {
	origFunc, origDelays := webChatFunc, askExpertRetryDelays
	t.Cleanup(func() { webChatFunc, askExpertRetryDelays = origFunc, origDelays })

	calls := 0
	webChatFunc = func(_ context.Context, _ string, _ lp.WebChatOptions) (lp.WebChatResult, error) {
		calls++
		return lp.WebChatResult{}, lp.ErrServerBusy
	}
	// Non-zero delay guarantees ctx.Done wins the select.
	askExpertRetryDelays = []time.Duration{time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := askExpertWebChat(ctx, "input", "expert", "", "", "", nil)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("webChatFunc calls = %d, want 1 (backoff aborted by cancel)", calls)
	}
}
