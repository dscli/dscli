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
	// roleCache is package-level and survives the DB swap; without clearing
	// it a previous test's configs would leak into this one's fresh DB.
	invalidateRoleCache()
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

// strPtr returns a pointer to v — an explicitly specified field value
// (the tri-state counterpart of nil = not specified).
func strPtr(v string) *string { return &v }

// TestUpsertInsertUnspecifiedFallsBackToRoleDefaults is the regression test
// for the 2026-07-31 incident: `role update dev --prompt editor` on a role
// with no row used to INSERT skills/tools as empty strings, and an empty
// tools value made GetAllTools return nothing, so the model lost all tool
// calling. Unspecified fields must fall back to the ROLE DEFAULTS
// (DefaultFor), never to a bare empty string.
func TestUpsertInsertUnspecifiedFallsBackToRoleDefaults(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "dev", sessionID, nil, nil, strPtr("editor")); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "dev")
	if cfg == nil {
		t.Fatal("dev config not found after insert")
	}
	if cfg.Skills != "all" {
		t.Errorf("Skills = %q, want %q", cfg.Skills, "all")
	}
	if cfg.Tools != DevDefaultTools {
		t.Errorf("Tools = %q, want %q", cfg.Tools, DevDefaultTools)
	}
	if cfg.Prompt != "editor" {
		t.Errorf("Prompt = %q, want %q", cfg.Prompt, "editor")
	}
}

// TestUpsertInsertPartialUsesRoleDefault guards the partial-update footgun:
// `role update review --tools shell,read_file` on a fresh project must NOT
// grant the review role all skills — the unspecified skills field falls back
// to DefaultFor("review") which is none, matching what role list displays.
func TestUpsertInsertPartialUsesRoleDefault(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "review", sessionID, nil, strPtr("shell,read_file"), nil); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "review")
	if cfg == nil {
		t.Fatal("review config not found after insert")
	}
	if cfg.Skills != "" {
		t.Errorf("Skills = %q, want empty (role default none)", cfg.Skills)
	}
	if cfg.Tools != "shell,read_file" {
		t.Errorf("Tools = %q, want %q", cfg.Tools, "shell,read_file")
	}
	if cfg.Prompt != "" {
		t.Errorf("Prompt = %q, want empty (role-named template)", cfg.Prompt)
	}
}

// TestUpsertInsertExplicitValues stores explicitly provided values verbatim.
func TestUpsertInsertExplicitValues(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "review", sessionID, strPtr("go-fix,shell"), strPtr("shell,file_read"), strPtr("editor")); err != nil {
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

// TestUpsertInsertExplicitNone stores "none" as an empty string: the CLI's
// `--tools none` must result in a row whose tools field is "", which
// ParseToolsList resolves to an empty allowlist.
func TestUpsertInsertExplicitNone(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "expert", sessionID, strPtr("none"), strPtr(""), nil); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "expert")
	if cfg == nil {
		t.Fatal("expert config not found after insert")
	}
	if cfg.Skills != "" {
		t.Errorf("Skills = %q, want empty", cfg.Skills)
	}
	if cfg.Tools != "" {
		t.Errorf("Tools = %q, want empty", cfg.Tools)
	}
	if got := ParseSkillsList(cfg.Skills); got == nil || len(got) != 0 {
		t.Errorf("ParseSkillsList(%q) = %v, want empty slice", cfg.Skills, got)
	}
}

// TestUpsertInsertPromptEmptyKeepsEmpty documents that an empty prompt is a
// legal final value on INSERT (means "use the role-named default template") —
// skills/tools fall back to the role default (all for dev), not empty.
func TestUpsertInsertPromptEmptyKeepsEmpty(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	if err := UpsertRoleConfig(t.Context(), "dev", sessionID, strPtr("go-fix"), nil, strPtr("")); err != nil {
		t.Fatalf("UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "dev")
	if cfg == nil {
		t.Fatal("dev config not found after insert")
	}
	if cfg.Skills != "go-fix" {
		t.Errorf("Skills = %q, want %q", cfg.Skills, "go-fix")
	}
	if cfg.Tools != DevDefaultTools {
		t.Errorf("Tools = %q, want %q", cfg.Tools, DevDefaultTools)
	}
	if cfg.Prompt != "" {
		t.Errorf("Prompt = %q, want empty", cfg.Prompt)
	}
}

// TestUpsertUpdatePreservesUnspecified guards the UPDATE branch: changing
// only prompt must keep existing skills/tools untouched (nil = not passed).
func TestUpsertUpdatePreservesUnspecified(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	ctx := t.Context()
	if err := UpsertRoleConfig(ctx, "dev", sessionID, strPtr("go-fix"), strPtr("shell"), strPtr("p1")); err != nil {
		t.Fatalf("initial UpsertRoleConfig: %v", err)
	}
	if err := UpsertRoleConfig(ctx, "dev", sessionID, nil, nil, strPtr("p2")); err != nil {
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
	if err := UpsertRoleConfig(ctx, "dev", sessionID, strPtr("go-fix"), strPtr("shell"), strPtr("p1")); err != nil {
		t.Fatalf("initial UpsertRoleConfig: %v", err)
	}
	if err := UpsertRoleConfig(ctx, "dev", sessionID, strPtr("all"), nil, nil); err != nil {
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

// TestUpsertUpdateExplicitNone guards clearing a field to none on UPDATE:
// `role update test --tools none` must store "", not keep the old value.
func TestUpsertUpdateExplicitNone(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	ctx := t.Context()
	if err := UpsertRoleConfig(ctx, "test", sessionID, strPtr("all"), strPtr("shell,read_file"), strPtr("p1")); err != nil {
		t.Fatalf("initial UpsertRoleConfig: %v", err)
	}
	if err := UpsertRoleConfig(ctx, "test", sessionID, nil, strPtr(""), nil); err != nil {
		t.Fatalf("update UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "test")
	if cfg == nil {
		t.Fatal("test config not found after update")
	}
	if cfg.Skills != "all" {
		t.Errorf("Skills = %q, want preserved %q", cfg.Skills, "all")
	}
	if cfg.Tools != "" {
		t.Errorf("Tools = %q, want empty (explicit none)", cfg.Tools)
	}
	if cfg.Prompt != "p1" {
		t.Errorf("Prompt = %q, want preserved %q", cfg.Prompt, "p1")
	}
}

// TestUpsertUpdatePromptExplicitEmpty guards clearing a prompt back to the
// role-named default template: `role update test --prompt ""` must store "".
func TestUpsertUpdatePromptExplicitEmpty(t *testing.T) {
	newTestDB(t)

	const sessionID = int64(1)
	ctx := t.Context()
	if err := UpsertRoleConfig(ctx, "test", sessionID, strPtr("all"), strPtr("all"), strPtr("my-qa")); err != nil {
		t.Fatalf("initial UpsertRoleConfig: %v", err)
	}
	if err := UpsertRoleConfig(ctx, "test", sessionID, nil, nil, strPtr("")); err != nil {
		t.Fatalf("update UpsertRoleConfig: %v", err)
	}

	cfg := findConfig(t, sessionID, "test")
	if cfg == nil {
		t.Fatal("test config not found after update")
	}
	if cfg.Prompt != "" {
		t.Errorf("Prompt = %q, want empty (back to role-named template)", cfg.Prompt)
	}
}

// TestDefaultFor documents the built-in defaults — the single source of truth
// shared by role list/show, GetAllTools, LoadPrompts and the WebChat DSML
// section.
func TestDefaultFor(t *testing.T) {
	cases := []struct {
		role       string
		wantSkills string
		wantTools  string
		wantPrompt string
	}{
		{"dev", "all", DevDefaultTools, "dev"},
		{"expert", "", "", "expert"},
		{"review", "", "", "review"},
		{"test", "", "", "test"},
		// The architect ships with the full toolset (like dev): it must be
		// able to design, persist the architecture doc, delegate via
		// code_dev/code_review/quality_assurance, and verify delivery.
		{"architect", "all", "all", "architect"},
		{"", "all", DevDefaultTools, "dev"}, // empty role = dev behavior
		{"unknown", "all", DevDefaultTools, "dev"},
	}
	for _, c := range cases {
		got := DefaultFor(c.role)
		if got.Skills != c.wantSkills || got.Tools != c.wantTools || got.Prompt != c.wantPrompt {
			t.Errorf("DefaultFor(%q) = %+v, want skills=%q tools=%q prompt=%q",
				c.role, got, c.wantSkills, c.wantTools, c.wantPrompt)
		}
	}
}

// TestDevDefaultToolsExcludesCommunication locks the dev tool-set shape:
// DevDefaultTools must be a non-empty, duplicate-free explicit list that
// excludes the communication/collaboration categories owned by architect,
// while still including the core development tools.
func TestDevDefaultToolsExcludesCommunication(t *testing.T) {
	names := ParseToolsList(DevDefaultTools)
	if names == nil || len(names) == 0 {
		t.Fatal("ParseToolsList(DevDefaultTools) must return a non-empty list")
	}
	seen := make(map[string]bool, len(names))
	for _, n := range names {
		if seen[n] {
			t.Errorf("DevDefaultTools has duplicate name %q", n)
		}
		seen[n] = true
	}
	excluded := []string{
		"readmail", "replymail", "sendmail", "listmail", "deletemail",
		"contacts", "mail_search",
		"ask_expert", "ask_user",
		"ainap", "wakeup", "aistatus",
		"flycheck", "code_review", "code_dev", "quality_assurance",
	}
	for _, ex := range excluded {
		if seen[ex] {
			t.Errorf("DevDefaultTools must not include %q", ex)
		}
	}
	included := []string{"read_file", "shell", "code_edit", "mem_save", "skill_by_name", "web_fetch"}
	for _, in := range included {
		if !seen[in] {
			t.Errorf("DevDefaultTools must include %q", in)
		}
	}
}

// TestParseListSemantics documents the API conventions from the package
// header: "all" → nil (include everything), "" and "none" → empty slice
// (nothing), "a,b" → filtered names.
func TestParseListSemantics(t *testing.T) {
	if got := parseList("all"); got != nil {
		t.Errorf("parseList(%q) = %v, want nil", "all", got)
	}
	if got := parseList(""); got == nil || len(got) != 0 {
		t.Errorf("parseList(%q) = %v, want empty slice", "", got)
	}
	if got := parseList("none"); got == nil || len(got) != 0 {
		t.Errorf("parseList(%q) = %v, want empty slice", "none", got)
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

// TestListRoleConfigsByPrompt verifies the dangling-reference scan used by
// `prompt remove`: rows are found by the prompt field they reference, with
// an optional single-session filter (0 = all sessions).
func TestListRoleConfigsByPrompt(t *testing.T) {
	newTestDB(t)

	ctx := t.Context()
	const (
		sidA = int64(1)
		sidB = int64(2)
	)
	if err := UpsertRoleConfig(ctx, "dev", sidA, strPtr("all"), strPtr("all"), strPtr("editor")); err != nil {
		t.Fatalf("UpsertRoleConfig(dev/sidA): %v", err)
	}
	if err := UpsertRoleConfig(ctx, "expert", sidA, nil, nil, strPtr("dev")); err != nil {
		t.Fatalf("UpsertRoleConfig(expert/sidA): %v", err)
	}
	if err := UpsertRoleConfig(ctx, "review", sidB, nil, nil, strPtr("editor")); err != nil {
		t.Fatalf("UpsertRoleConfig(review/sidB): %v", err)
	}

	// All sessions referencing "editor".
	refs, err := ListRoleConfigsByPrompt(ctx, "editor", 0)
	if err != nil {
		t.Fatalf("ListRoleConfigsByPrompt(all): %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("ListRoleConfigsByPrompt(all) = %d rows, want 2", len(refs))
	}
	gotRoles := map[string]bool{}
	for _, c := range refs {
		gotRoles[c.Role] = true
	}
	if !gotRoles["dev"] || !gotRoles["review"] {
		t.Errorf("ListRoleConfigsByPrompt(all) roles = %v, want dev and review", gotRoles)
	}

	// Single session only.
	refs, err = ListRoleConfigsByPrompt(ctx, "editor", sidA)
	if err != nil {
		t.Fatalf("ListRoleConfigsByPrompt(sidA): %v", err)
	}
	if len(refs) != 1 || refs[0].Role != "dev" {
		t.Errorf("ListRoleConfigsByPrompt(sidA) = %+v, want only dev", refs)
	}

	// Unknown prompt name.
	refs, err = ListRoleConfigsByPrompt(ctx, "missing", 0)
	if err != nil {
		t.Fatalf("ListRoleConfigsByPrompt(missing): %v", err)
	}
	if len(refs) != 0 {
		t.Errorf("ListRoleConfigsByPrompt(missing) = %d rows, want 0", len(refs))
	}
}

// TestGetRoleConfigSessionIsolation guards the per-session cache buckets:
// role_configs is keyed by UNIQUE(role, session_id), so a lookup for session
// B must never return session A's row (or A's cached nil). Both LOOKUP
// orders are exercised because the pre-bucket cache had two failure modes:
// populated-first served the wrong config (false positive), empty-first
// cached nil and then hid the populated session's config (false negative).
func TestGetRoleConfigSessionIsolation(t *testing.T) {
	const (
		role        = "dev"
		populatedID = int64(10) // has a config row
		emptyID     = int64(20) // has no rows
	)
	scenarios := []struct {
		name     string
		firstID  int64 // session looked up first
		secondID int64 // session looked up second
	}{
		{"populated first", populatedID, emptyID},
		{"empty first", emptyID, populatedID},
	}

	for _, sc := range scenarios {
		t.Run(sc.name, func(t *testing.T) {
			newTestDB(t) // fresh DB per subtest; also clears the role cache
			ctx := t.Context()

			if err := UpsertRoleConfig(ctx, role, populatedID, nil, strPtr("shell"), nil); err != nil {
				t.Fatalf("UpsertRoleConfig(populated): %v", err)
			}

			// First lookup: the populated session must return its config,
			// the empty session nil. A repeat lookup must agree (positive
			// and negative results are cached within their bucket).
			first, err := GetRoleConfig(ctx, role, sc.firstID)
			if err != nil {
				t.Fatalf("GetRoleConfig(first): %v", err)
			}
			assertSessionConfig(t, "first", first, sc.firstID == populatedID)
			first, err = GetRoleConfig(ctx, role, sc.firstID)
			if err != nil {
				t.Fatalf("GetRoleConfig(first) second call: %v", err)
			}
			assertSessionConfig(t, "first repeat", first, sc.firstID == populatedID)

			// Second lookup must not be affected by the first: buckets do
			// not overwrite each other.
			second, err := GetRoleConfig(ctx, role, sc.secondID)
			if err != nil {
				t.Fatalf("GetRoleConfig(second): %v", err)
			}
			assertSessionConfig(t, "second", second, sc.secondID == populatedID)
			second, err = GetRoleConfig(ctx, role, sc.secondID)
			if err != nil {
				t.Fatalf("GetRoleConfig(second) second call: %v", err)
			}
			assertSessionConfig(t, "second repeat", second, sc.secondID == populatedID)
		})
	}
}

// assertSessionConfig checks a lookup result against the expected session
// state: the populated session must return Tools=="shell", the empty one nil.
func assertSessionConfig(t *testing.T, label string, cfg *RoleConfig, populated bool) {
	t.Helper()
	if populated {
		if cfg == nil || cfg.Tools != "shell" {
			t.Errorf("GetRoleConfig(%s) = %+v, want Tools=shell", label, cfg)
		}
		return
	}
	if cfg != nil {
		t.Errorf("GetRoleConfig(%s) = %+v, want nil", label, cfg)
	}
}
