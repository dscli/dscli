package lp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeUploadName(t *testing.T) {
	cases := map[string]struct {
		want    string
		renamed bool
	}{
		// Site-rejected or silently dropped names get the ".txt" suffix.
		".gitignore":     {".gitignore.txt", true},
		"Makefile":       {"Makefile.txt", true},
		"go.sum":         {"go.sum.txt", true},
		"go.work":        {"go.work.txt", true},
		".gitattributes": {".gitattributes.txt", true},
		".env":           {".env.txt", true},
		"LICENSE":        {"LICENSE.txt", true},
		// Unprobed spellings are renamed too: matching is exact and
		// case-sensitive, so only the lower-case forms the probe exercised
		// pass through untouched.
		"README.MD": {"README.MD.txt", true},
		"Icon.PNG":  {"Icon.PNG.txt", true},
		"Doc.PDF":   {"Doc.PDF.txt", true},
		// A trailing dot names no format: it is dropped, not doubled, and
		// the extension is re-derived, so a verified extension underneath is
		// kept without stacking another ".txt".
		"foo.":     {"foo.txt", true},
		"foo.txt.": {"foo.txt", true},
		"foo.md.":  {"foo.md", true},
		// A hidden file whose whole name is a verified extension passes
		// through (known and intentional: the policy keys on the extension).
		".txt": {".txt", false},
		".md":  {".md", false},
		// An SVG is accepted as an image but yields no text: not verified.
		"icon.svg": {"icon.svg.txt", true},
		// Verified names stay untouched.
		"Makefile.txt":   {"Makefile.txt", false},
		"main.go":        {"main.go", false},
		"a.txt":          {"a.txt", false},
		".github__x.yml": {".github__x.yml", false},
		"shot.png":       {"shot.png", false},
		"doc.pdf":        {"doc.pdf", false},
	}
	for in, want := range cases {
		got, renamed := SafeUploadName(in)
		if got != want.want || renamed != want.renamed {
			t.Errorf("SafeUploadName(%q) = (%q, %v), want (%q, %v)", in, got, renamed, want.want, want.renamed)
		}
	}
}

func TestUniqueUploadName(t *testing.T) {
	used := map[string]bool{}
	for i, want := range []string{"x.go", "x_2.go", "x_3.go"} {
		if got := UniqueUploadName(used, "x.go"); got != want {
			t.Errorf("collision %d: got %q, want %q", i, got, want)
		}
	}
	for i, want := range []string{"x.txt", "x_2.txt", "x_3.txt"} {
		if got := UniqueUploadName(used, "x.txt"); got != want {
			t.Errorf("txt collision %d: got %q, want %q", i, got, want)
		}
	}
	// A distinct stem is unaffected, and the suffix lands before the extension
	// (the part the upload site inspects).
	if got := UniqueUploadName(used, "Makefile.txt"); got != "Makefile.txt" {
		t.Errorf("distinct name: got %q, want Makefile.txt", got)
	}
	if got := UniqueUploadName(used, "Makefile.txt"); got != "Makefile_2.txt" {
		t.Errorf("suffix must precede the extension: got %q, want Makefile_2.txt", got)
	}
	// A name the caller already reserved is not handed out again.
	seeded := map[string]bool{"a.go": true}
	if got := UniqueUploadName(seeded, "a.go"); got != "a_2.go" {
		t.Errorf("seeded name: got %q, want a_2.go", got)
	}
}

func TestPrepareUploadAttachmentsRenamesRejectedNames(t *testing.T) {
	dir := t.TempDir()
	orig := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(orig, []byte("*.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prepared, err := prepareUploadAttachments([]string{orig})
	if err != nil {
		t.Fatalf("prepareUploadAttachments: %v", err)
	}
	if prepared.cleanup == nil {
		t.Fatal("a renamed batch must carry a cleanup")
	}
	if len(prepared.notes) != 1 || !strings.Contains(prepared.notes[0], ".gitignore.txt") {
		t.Errorf("notes = %q, want one line naming .gitignore.txt", prepared.notes)
	}
	if len(prepared.notes) == 1 && !strings.Contains(prepared.notes[0], "扩展名网站不支持") {
		t.Errorf("note = %q, want the extension reason", prepared.notes[0])
	}
	if got := filepath.Base(prepared.files[0]); got != ".gitignore.txt" {
		t.Fatalf("upload name = %q, want .gitignore.txt", got)
	}
	info, err := os.Stat(prepared.files[0])
	if err != nil {
		t.Fatalf("stat copy: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("copy mode = %o, want 600", perm)
	}
	if b, err := os.ReadFile(prepared.files[0]); err != nil || string(b) != "*.txt\n" {
		t.Errorf("copy content = %q (err %v), want the original bytes", b, err)
	}
	// The original file is never modified or moved.
	if b, err := os.ReadFile(orig); err != nil || string(b) != "*.txt\n" {
		t.Errorf("original content = %q (err %v), want unchanged", b, err)
	}

	copyPath := prepared.files[0]
	prepared.cleanup()
	if _, err := os.Stat(copyPath); !os.IsNotExist(err) {
		t.Errorf("copy survived cleanup (stat err = %v)", err)
	}
}

func TestPrepareUploadAttachmentsKeepsVerifiedNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "main.go")
	if err := os.WriteFile(path, []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prepared, err := prepareUploadAttachments([]string{path})
	if err != nil {
		t.Fatalf("prepareUploadAttachments: %v", err)
	}
	if prepared.cleanup != nil {
		t.Error("a clean batch must not create a temp dir")
	}
	if len(prepared.notes) != 0 {
		t.Errorf("notes = %q, want none", prepared.notes)
	}
	if prepared.files[0] != path {
		t.Errorf("upload path = %q, want the original %q", prepared.files[0], path)
	}
}

// TestPrepareUploadAttachmentsDeduplicates covers the collision branch: two
// files in different directories share the base name "x.txt", so the second is
// uploaded as "x_2.txt" and the note names the collision (not the extension).
func TestPrepareUploadAttachmentsDeduplicates(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	first := filepath.Join(firstDir, "x.txt")
	second := filepath.Join(secondDir, "x.txt")
	if err := os.WriteFile(first, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prepared, err := prepareUploadAttachments([]string{first, second})
	if err != nil {
		t.Fatalf("prepareUploadAttachments: %v", err)
	}
	if prepared.cleanup == nil {
		t.Fatal("the deduplicated file needs a copy")
	}
	defer prepared.cleanup()

	if prepared.files[0] != first {
		t.Errorf("first upload path = %q, want the original %q (no rename)", prepared.files[0], first)
	}
	got := filepath.Base(prepared.files[1])
	if got != "x_2.txt" {
		t.Fatalf("second upload name = %q, want x_2.txt (suffix before the extension)", got)
	}
	if b, err := os.ReadFile(prepared.files[1]); err != nil || string(b) != "second\n" {
		t.Errorf("deduplicated copy = %q (err %v), want the second file's bytes", b, err)
	}
	if len(prepared.notes) != 1 {
		t.Fatalf("notes = %q, want exactly one line for the colliding file", prepared.notes)
	}
	if !strings.Contains(prepared.notes[0], "与已有附件重名") || !strings.Contains(prepared.notes[0], "x_2.txt") {
		t.Errorf("note = %q, want the collision reason and the new name", prepared.notes[0])
	}
}

// TestPrepareUploadAttachmentsDedupAfterRename: a name with no extension
// renames to "x.txt" and then collides with an existing "x.txt"; the final
// name keeps the ".txt" ending so it is never renamed a second time.
func TestPrepareUploadAttachmentsDedupAfterRename(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "x.txt")
	second := filepath.Join(dir, "x") // renames to x.txt, colliding with first
	if err := os.WriteFile(first, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	prepared, err := prepareUploadAttachments([]string{first, second})
	if err != nil {
		t.Fatalf("prepareUploadAttachments: %v", err)
	}
	if prepared.cleanup == nil {
		t.Fatal("the colliding file needs a copy")
	}
	defer prepared.cleanup()

	if got := filepath.Base(prepared.files[1]); got != "x_2.txt" {
		t.Fatalf("upload name = %q, want x_2.txt", got)
	}
	if safe, renamed := SafeUploadName("x_2.txt"); renamed || safe != "x_2.txt" {
		t.Errorf("SafeUploadName(x_2.txt) = (%q, %v), want it stable", safe, renamed)
	}
}

// TestUploadTempDirUsesPrefix pins the production default: the seam must keep
// creating "dscli-upload-*" directories, which is what the cleanup contract
// and the isolated tests below rely on.
func TestUploadTempDirUsesPrefix(t *testing.T) {
	dir, err := uploadTempDir()
	if err != nil {
		t.Fatalf("uploadTempDir: %v", err)
	}
	defer os.RemoveAll(dir)
	if !strings.HasPrefix(filepath.Base(dir), "dscli-upload-") {
		t.Errorf("temp dir = %q, want the dscli-upload- prefix", filepath.Base(dir))
	}
}

func TestPrepareUploadAttachmentsMissingSource(t *testing.T) {
	count := isolateUploadTempRoot(t)
	_, err := prepareUploadAttachments([]string{filepath.Join(t.TempDir(), "Makefile")})
	if err == nil {
		t.Fatal("a missing source must fail")
	}
	if n := count(); n != 0 {
		t.Errorf("temp dirs leaked on error: %d", n)
	}
}

// TestPrepareUploadAttachmentsCopyFailureCleansUp forces the io.Copy failure
// branch by naming a directory "Makefile" (open succeeds, the copy does not)
// and checks that the temp dir is removed before the error is returned.
func TestPrepareUploadAttachmentsCopyFailureCleansUp(t *testing.T) {
	count := isolateUploadTempRoot(t)
	src := filepath.Join(t.TempDir(), "Makefile")
	if err := os.Mkdir(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareUploadAttachments([]string{src}); err == nil {
		t.Fatal("copying a directory must fail")
	}
	if n := count(); n != 0 {
		t.Errorf("temp dirs leaked on error: %d", n)
	}
}

// isolateUploadTempRoot confines uploadTempDir to a test-owned root and
// returns a counter of the upload dirs still present there, so the error-path
// tests assert the cleanup contract without scanning the system temp
// directory (other processes share it).
func isolateUploadTempRoot(t *testing.T) func() int {
	t.Helper()
	root := t.TempDir()
	orig := uploadTempDir
	t.Cleanup(func() { uploadTempDir = orig })
	uploadTempDir = func() (string, error) {
		return os.MkdirTemp(root, "dscli-upload-")
	}
	return func() int {
		t.Helper()
		entries, err := os.ReadDir(root)
		if err != nil {
			t.Fatalf("read temp root: %v", err)
		}
		return len(entries)
	}
}
