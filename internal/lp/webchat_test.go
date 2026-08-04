package lp

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractResponse(t *testing.T) {
	tests := []struct {
		name     string
		baseline string
		current  string
		want     string
	}{
		{name: "appended", baseline: "abc", current: "abcd", want: "d"},
		{name: "unchanged", baseline: "abc", current: "abc", want: ""},
		{name: "shrunk", baseline: "abcd", current: "abc", want: ""},
		// The U+FFFD bug: current no longer starts with baseline (textarea
		// cleared after send), so a suffix slice would return garbage.
		{name: "prefix mismatch", baseline: "xyz", current: "abcd", want: ""},
		{name: "empty", baseline: "", current: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractResponse(tt.baseline, tt.current); got != tt.want {
				t.Errorf("extractResponse(%q, %q) = %q, want %q", tt.baseline, tt.current, got, tt.want)
			}
		})
	}
}

func TestIsCompleteResponse(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{name: "empty", s: "", want: false},
		{name: "replacement char", s: "\uFFFD", want: false},
		// The model pauses after emitting a simulated tool call that the
		// web UI cannot execute; the fragment must not be returned.
		{name: "tool call fragment", s: "<read_file path=\"AGENTS.md\" />", want: false},
		{name: "quoted tool call fragment", s: "> <read_file path=\"AGENTS.md\" />", want: false},
		// A full answer that includes the simulated call plus body is fine.
		{name: "full review with tool call", s: "> <read_file path=\"AGENTS.md\" />\n\n> <tool_result>\n# AGENTS.md\n\n## Overall Assessment\nSolid.", want: true},
		{name: "short with tool result", s: "<read_file />\n<tool_result>ok</tool_result>", want: true},
		// A genuine short answer with an XML-like tag must not be rejected
		// as a tool-call fragment (regression guard for the whitelist).
		{name: "short answer with html tag", s: "<b>bold</b> is fine", want: true},
		{name: "plain answer", s: "The change looks correct.", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCompleteResponse(tt.s); got != tt.want {
				t.Errorf("isCompleteResponse(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStripBaselinePrefix(t *testing.T) {
	tests := []struct {
		name string
		resp string
		base string
		want string
	}{
		{name: "prefix stripped", resp: "history\nnew", base: "history", want: "new"},
		{name: "no prefix", resp: "history\nnew", base: "other", want: "history\nnew"},
		{name: "empty baseline", resp: "new", base: "", want: "new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripBaselinePrefix(tt.resp, tt.base); got != tt.want {
				t.Errorf("stripBaselinePrefix(%q, %q) = %q, want %q", tt.resp, tt.base, got, tt.want)
			}
		})
	}
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"photo.png", true},
		{"photo.PNG", true}, // case-insensitive
		{"photo.jpg", true},
		{"photo.jpeg", true},
		{"photo.gif", true},
		{"photo.webp", true},
		{"photo.bmp", true},
		{"doc.txt", false},
		{"archive.tar.gz", false},
		{"noext", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsImageFile(tt.path); got != tt.want {
				t.Errorf("IsImageFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizeWebChatOptions(t *testing.T) {
	tests := []struct {
		name string
		opts WebChatOptions
		want Mode
	}{
		{"new conversation defaults to pro", WebChatOptions{}, ModePro},
		{"attachments imply vision", WebChatOptions{Attachments: []string{"a.png"}}, ModeVision},
		{"continued conversation preserves mode", WebChatOptions{Keep: "last"}, ""},
		{"explicit mode wins", WebChatOptions{Mode: ModeFlash, Attachments: []string{"a.png"}}, ModeFlash},
		{"explicit pro with keep", WebChatOptions{Mode: ModePro, Keep: "last"}, ModePro},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeWebChatOptions(tt.opts).Mode; got != tt.want {
				t.Errorf("normalizeWebChatOptions(%+v).Mode = %q, want %q", tt.opts, got, tt.want)
			}
		})
	}
}

func TestValidateWebChatOptions(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	if err := os.WriteFile(img, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := []WebChatOptions{
		{Mode: ModePro},
		{Mode: ModeFlash, Attachments: []string{img}},
		{Mode: ModeVision, Attachments: []string{img}},
		{Mode: ""}, // auto
	}
	for _, opts := range valid {
		if err := validateWebChatOptions(opts); err != nil {
			t.Errorf("validateWebChatOptions(%+v) = %v, want nil", opts, err)
		}
	}
	if err := validateWebChatOptions(WebChatOptions{Mode: "turbo"}); err == nil {
		t.Error("unknown mode must fail")
	}
	if err := validateWebChatOptions(WebChatOptions{Mode: ModePro, Attachments: []string{img}}); err == nil {
		t.Error("pro with attachments must fail")
	}
}

func TestResolveWebAttachments(t *testing.T) {
	// Empty input stays untouched (nil stays nil, empty stays empty).
	if got := mustResolve(t, nil); got != nil {
		t.Errorf("resolveWebAttachments(nil) = %v, want nil", got)
	}
	if got := mustResolve(t, []string{}); len(got) != 0 {
		t.Errorf("resolveWebAttachments([]) = %v, want empty", got)
	}

	// Absolute paths are kept as-is.
	abs := filepath.Join(t.TempDir(), "shot.png")
	if got := mustResolve(t, []string{abs}); got[0] != abs {
		t.Errorf("resolveWebAttachments(%q) = %q, want unchanged", abs, got[0])
	}

	// Relative paths are resolved against the process cwd — Chrome's CDP
	// upload reads files with Chrome's working directory, not dscli's.
	rel := "relative-shot.png"
	got := mustResolve(t, []string{rel})
	want, _ := filepath.Abs(rel)
	if got[0] != want {
		t.Errorf("resolveWebAttachments(%q) = %q, want %q", rel, got[0], want)
	}
	if !filepath.IsAbs(got[0]) {
		t.Errorf("resolveWebAttachments(%q) = %q, want absolute path", rel, got[0])
	}
}

func mustResolve(t *testing.T, files []string) []string {
	t.Helper()
	resolved, err := resolveWebAttachments(files)
	if err != nil {
		t.Fatalf("resolveWebAttachments(%v): %v", files, err)
	}
	return resolved
}

func TestValidateWebAttachments(t *testing.T) {
	// Too many files (count check runs before the stat loop).
	files := make([]string, webUploadMaxFiles+1)
	for i := range files {
		files[i] = fmt.Sprintf("f%d.png", i)
	}
	if err := validateWebAttachments(files); err == nil {
		t.Error("too many attachments must fail")
	}

	// Missing file.
	if err := validateWebAttachments([]string{"no-such-file.png"}); err == nil {
		t.Error("missing attachment must fail")
	}

	dir := t.TempDir()
	// Total size limit: a sparse file reports its logical size without
	// using disk space.
	big := filepath.Join(dir, "big.png")
	if err := os.WriteFile(big, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, webUploadMaxTotal+1); err != nil {
		t.Fatal(err)
	}
	if err := validateWebAttachments([]string{big}); err == nil {
		t.Error("oversized attachments must fail")
	}

	// Valid single small file.
	small := filepath.Join(dir, "small.png")
	if err := os.WriteFile(small, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWebAttachments([]string{small}); err != nil {
		t.Errorf("small attachment must pass: %v", err)
	}
}

func TestConversationIDFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://chat.deepseek.com/a/chat/s/abc123", "abc123"},
		{"https://chat.deepseek.com/a/chat/s/a1b2-c3d4_e5", "a1b2-c3d4_e5"},
		{"https://chat.deepseek.com/a/chat/s/abc123?extra=1", "abc123"},
		{"https://chat.deepseek.com/", ""},
		{"", ""},
		{"not a url", ""},
	}
	for _, tt := range tests {
		if got := ConversationIDFromURL(tt.url); got != tt.want {
			t.Errorf("ConversationIDFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// testRegistry builds a registry with the given id → url entries.
func testRegistry(entries map[string]string) *conversationRegistry {
	reg := &conversationRegistry{Sessions: map[string]conversationEntry{}}
	for id, url := range entries {
		reg.Sessions[id] = conversationEntry{URL: url}
	}
	return reg
}

func TestRegistryResolve(t *testing.T) {
	reg := testRegistry(map[string]string{
		"aaa": "https://chat.deepseek.com/a/chat/s/aaa",
		"bbb": "https://chat.deepseek.com/a/chat/s/bbb",
	})
	reg.Sessions["aaa"] = conversationEntry{
		URL:       "https://chat.deepseek.com/a/chat/s/aaa",
		UpdatedAt: "2026-08-01T00:00:00Z",
	}
	reg.Sessions["bbb"] = conversationEntry{
		URL:       "https://chat.deepseek.com/a/chat/s/bbb",
		UpdatedAt: "2026-08-02T00:00:00Z",
	}

	tests := []struct {
		name string
		keep string
		want string
	}{
		{"empty = new conversation", "", ""},
		{"last picks most recent", "last", "https://chat.deepseek.com/a/chat/s/bbb"},
		{"exact id", "aaa", "https://chat.deepseek.com/a/chat/s/aaa"},
		{"full url passthrough", "https://chat.deepseek.com/a/chat/s/zzz", "https://chat.deepseek.com/a/chat/s/zzz"},
		{"url suffix match", "s/bbb", "https://chat.deepseek.com/a/chat/s/bbb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.resolve(tt.keep)
			if err != nil {
				t.Fatalf("resolve(%q): %v", tt.keep, err)
			}
			if got != tt.want {
				t.Errorf("resolve(%q) = %q, want %q", tt.keep, got, tt.want)
			}
		})
	}

	if _, err := reg.resolve("nope"); err == nil {
		t.Error("unknown id must fail")
	}
}

func TestRegistryResolveEmptyRegistry(t *testing.T) {
	reg := testRegistry(nil)
	if _, err := reg.resolve("last"); err == nil {
		t.Error("last on empty registry must fail")
	}
	if got, err := reg.resolve(""); err != nil || got != "" {
		t.Errorf("resolve(\"\") = %q, %v; want \"\", nil", got, err)
	}
}

func TestRegistryTrim(t *testing.T) {
	reg := &conversationRegistry{Sessions: map[string]conversationEntry{}}
	// Fill beyond the cap; later timestamps are newer and must survive.
	for i := 0; i < maxSavedConversations+10; i++ {
		id := fmt.Sprintf("id%03d", i)
		reg.Sessions[id] = conversationEntry{
			URL:       "https://chat.deepseek.com/a/chat/s/" + id,
			UpdatedAt: fmt.Sprintf("2026-08-01T%02d:%02d:00Z", i/60, i%60),
		}
	}
	reg.trim()
	if len(reg.Sessions) != maxSavedConversations {
		t.Fatalf("trim: %d entries, want %d", len(reg.Sessions), maxSavedConversations)
	}
	// The 10 oldest (id000..id009) must be gone.
	if _, ok := reg.Sessions["id000"]; ok {
		t.Error("oldest entry id000 survived trim")
	}
	if _, ok := reg.Sessions["id010"]; !ok {
		t.Error("newest entry id010 was trimmed away")
	}
}
