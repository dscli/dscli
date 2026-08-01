package roles

import (
	"path/filepath"
	"testing"

	"github.com/dscli/dscli/internal/sqlite"
)

// newTestDB points the sqlite DB at a fresh temp file and restores the
// original path afterwards.
//
// OpenDB caches dbPath at package init, so changing config dirs alone would
// still hit the real ~/.dscli/sqlite.db. SetDBPath resets that state and is
// the only reliable isolation for package tests.
func newTestDB(t *testing.T) {
	t.Helper()
	origDBPath := sqlite.GetDBPath()
	sqlite.SetDBPath(filepath.Join(t.TempDir(), "roles-test.db"))
	t.Cleanup(func() { sqlite.SetDBPath(origDBPath) })
}

// findConfig returns the config for role in sessionID, or nil if absent.
func findConfig(t *testing.T, sessionID int64, role string) *RoleConfig {
	t.Helper()
	configs, err := ListRoleConfigs(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("ListRoleConfigs: %v", err)
	}
	for i := range configs {
		if configs[i].Role == role {
			return &configs[i]
		}
	}
	return nil
}

// TestUpsertInsertUnspecifiedDefaultsToAll is the regression test for the
// 2026-07-31 incident: `role update dev --prompt editor` on a role with no
// row used to INSERT skills/tools as empty strings, and an empty tools value
// made GetAllTools return nothing, so the model lost all tool calling.
// INSERT must fall back to "all" for unspecified skills/tools (schema
// DEFAULT, docs promise).
func TestUpsertInsertUnspecifiedDefaultsToAll(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "dev", sessionID, "", "", "editor"); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "dev")
	if cfg == nil {
		t.Fatal("dev config not found after insert")
	}
	if cfg.Skills != "all" {
		t.Errorf("Skills = %q, want %q", cfg.Skills, "all")
	}
	if cfg.Tools != "all" {
		t.Errorf("Tools = %q, want %q", cfg.Tools, "all")
	}
	if cfg.Prompt != "editor" {
		t.Errorf("Prompt = %q, want %q", cfg.Prompt, "editor")
	}
}

// TestUpsertInsertExplicitValues stores explicitly provided values verbatim.
func TestUpsertInsertExplicitValues(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "review", sessionID, "go-fix,shell", "shell,file_read", "editor"); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "review")
	if cfg == nil {
		t.Fatal("review config not found after insert")
	}
	if cfg.Skills != "go-fix,shell" {
		t.Errorf("Skills = %q, want %q", cfg.Skills, "go-fix,shell")
	}
	if cfg.Tools != "shell,file_read" {
		t.Errorf("Tools = %q, want %q", cfg.Tools, "shell,file_read")
	}
	if cfg.Prompt != "editor" {
		t.Errorf("Prompt = %q, want %q", cfg.Prompt, "editor")
	}
}

// TestUpsertInsertPromptEmptyKeepsEmpty documents that an empty prompt is a
// legal final value on INSERT (means "use the role-named default template") —
// only skills/tools fall back to "all".
func TestUpsertInsertPromptEmptyKeepsEmpty(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "dev", sessionID, "go-fix", "", ""); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "dev")
	if cfg == nil {
		t.Fatal("dev config not found after insert")
	}
	if cfg.Skills != "go-fix" {
		t.Errorf("Skills = %q, want %q", cfg.Skills, "go-fix")
	}
	if cfg.Tools != "all" {
		t.Errorf("Tools = %q, want %q", cfg.Tools, "all")
	}
	if cfg.Prompt != "" {
		t.Errorf("Prompt = %q, want empty", cfg.Prompt)
	}
}

// TestUpsertUpdatePreservesUnspecified guards the UPDATE branch: changing
// only prompt must keep existing skills/tools untouched.
func TestUpsertUpdatePreservesUnspecified(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	ctx := t.Context()
	if err := UpsertRoleConfig(ctx, "dev", sessionID, "go-fix", "shell", "p1"); err != nil {
		t.Fatalf("initial UpsertRoleConfig: %v", err)
	}
	if err := UpsertRoleConfig(ctx, "dev", sessionID, "", "", "p2"); err != nil {
		t.Fatalf("update UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "dev")
	if cfg == nil {
		t.Fatal("dev config not found after update")
	}
	if cfg.Skills != "go-fix" {
		t.Errorf("Skills = %q, want preserved %q", cfg.Skills, "go-fix")
	}
	if cfg.Tools != "shell" {
		t.Errorf("Tools = %q, want preserved %q", cfg.Tools, "shell")
	}
	if cfg.Prompt != "p2" {
		t.Errorf("Prompt = %q, want %q", cfg.Prompt, "p2")
	}
}

// TestUpsertUpdateOverwritesExplicit guards the UPDATE branch: explicitly
// provided fields must overwrite, unspecified ones must stay.
func TestUpsertUpdateOverwritesExplicit(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	ctx := t.Context()
	if err := UpsertRoleConfig(ctx, "dev", sessionID, "go-fix", "shell", "p1"); err != nil {
		t.Fatalf("initial UpsertRoleConfig: %v", err)
	}
	if err := UpsertRoleConfig(ctx, "dev", sessionID, "all", "", ""); err != nil {
		t.Fatalf("update UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "dev")
	if cfg == nil {
		t.Fatal("dev config not found after update")
	}
	if cfg.Skills != "all" {
		t.Errorf("Skills = %q, want %q", cfg.Skills, "all")
	}
	if cfg.Tools != "shell" {
		t.Errorf("Tools = %q, want preserved %q", cfg.Tools, "shell")
	}
	if cfg.Prompt != "p1" {
		t.Errorf("Prompt = %q, want preserved %q", cfg.Prompt, "p1")
	}
}

// TestParseListSemantics documents the API conventions from the package
// header: "all" → nil (include everything), "" → empty slice (nothing),
// "a,b" → filtered names.
func TestParseListSemantics(t *testing.T) {
	if got := parseList("all"); got != nil {
		t.Errorf("parseList(%q) = %v, want nil", "all", got)
	}
	if got := parseList(""); got == nil || len(got) != 0 {
		t.Errorf("parseList(%q) = %v, want empty slice", "", got)
	}
	got := parseList("go-fix, shell, ,gofumpt")
	want := []string{"go-fix", "shell", "gofumpt"}
	if len(got) != len(want) {
		t.Fatalf("parseList names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("parseList names = %v, want %v", got, want)
			break
		}
	}
}
