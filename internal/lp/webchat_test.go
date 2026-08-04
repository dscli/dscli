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
		{"continued conversation preserves mode", WebChatOptions{Keep: true}, ""},
		{"explicit mode wins", WebChatOptions{Mode: ModeFlash, Attachments: []string{"a.png"}}, ModeFlash},
		{"explicit pro with keep", WebChatOptions{Mode: ModePro, Keep: true}, ModePro},
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
