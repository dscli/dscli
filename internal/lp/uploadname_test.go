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
		// An SVG is accepted as an image but yields no text: not verified.
		"icon.svg": {"icon.svg.txt", true},
		// Verified names stay untouched (case-insensitive extension match).
		"Makefile.txt":   {"Makefile.txt", false},
		"main.go":        {"main.go", false},
		"a.txt":          {"a.txt", false},
		".github__x.yml": {".github__x.yml", false},
		"README.MD":      {"README.MD", false},
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
	for i, want := range []string{"x.txt", "x_2.txt", "x_3.txt"} {
		if got := uniqueUploadName(used, "x.txt"); got != want {
			t.Errorf("collision %d: got %q, want %q", i, got, want)
		}
	}
	// A distinct stem is unaffected, and the suffix lands before the extension.
	if got := uniqueUploadName(used, "Makefile.txt"); got != "Makefile.txt" {
		t.Errorf("distinct name: got %q, want Makefile.txt", got)
	}
	used["Makefile.txt"] = true
	if got := uniqueUploadName(used, "Makefile.txt"); got != "Makefile_2.txt" {
		t.Errorf("suffix must precede the extension: got %q, want Makefile_2.txt", got)
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

func TestPrepareUploadAttachmentsDeduplicates(t *testing.T) {
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
		t.Fatal("the deduplicated file needs a copy")
	}
	defer prepared.cleanup()

	if got := filepath.Base(prepared.files[0]); got != "x.txt" {
		t.Errorf("first upload name = %q, want x.txt", got)
	}
	got := filepath.Base(prepared.files[1])
	if got != "x_2.txt" {
		t.Fatalf("second upload name = %q, want x_2.txt (suffix before the extension)", got)
	}
	if b, err := os.ReadFile(prepared.files[1]); err != nil || string(b) != "second\n" {
		t.Errorf("deduplicated copy = %q (err %v), want the second file's bytes", b, err)
	}
}
