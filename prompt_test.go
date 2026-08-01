package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/spf13/cobra"
)

// promptTestEnv isolates DB, config dir and project root for prompt command
// tests, restoring all global state afterwards. Returns the temp project root.
//
// NOTE: modifies package-level state (sqlite db path, config dir, project
// root, session cache) and therefore must not run with t.Parallel.
func promptTestEnv(t *testing.T) string {
	t.Helper()

	origDB := sqlite.GetDBPath()
	sqlite.SetDBPath(filepath.Join(t.TempDir(), "test.db"))
	t.Cleanup(func() { sqlite.SetDBPath(origDB) })

	origConfig := config.ConfigDir
	config.ConfigDir = t.TempDir()
	t.Cleanup(func() { config.ConfigDir = origConfig })

	origRoot := context.ProjectRoot
	projectRoot := t.TempDir()
	context.ProjectRoot = projectRoot
	t.Cleanup(func() { context.ProjectRoot = origRoot })

	// Session ID is cached per process; reset it so it re-resolves against
	// the isolated DB/project, and reset again on cleanup for later tests.
	session.ResetSessionID()
	t.Cleanup(session.ResetSessionID)

	return projectRoot
}

// writePromptFile creates <dir>/<name>.md with trivial content.
func writePromptFile(t *testing.T, dir, name string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name+".md")
	if err := os.WriteFile(p, []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// insertRoleConfig writes a raw role_configs row, bypassing UpsertRoleConfig
// so tests can plant stale data (e.g. empty skills/tools from the old INSERT
// bug) exactly as it exists in the wild.
func insertRoleConfig(t *testing.T, role string, sessionID int64, skills, tools, prompt string) {
	t.Helper()
	db, err := sqlite.OpenDB(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(t.Context())
	if _, err := db.Exec(
		`INSERT INTO role_configs (role, skills, tools, prompt, session_id)
		 VALUES (?, ?, ?, ?, ?)`,
		role, skills, tools, prompt, sessionID,
	); err != nil {
		t.Fatal(err)
	}
}

// roleConfigCount returns how many role_configs rows match role+session.
func roleConfigCount(t *testing.T, role string, sessionID int64) int {
	t.Helper()
	db, err := sqlite.OpenDB(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(t.Context())
	var n int
	if err := db.QueryRow(
		`SELECT COUNT(*) FROM role_configs WHERE role = ? AND session_id = ?`,
		role, sessionID,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// runPromptRemove executes `prompt remove <name>` with the given global flag
// and returns captured stdout/stderr.
func runPromptRemove(t *testing.T, name string, global bool) (stdout, stderr string) {
	t.Helper()
	cmd := &cobra.Command{Use: "remove <name>", RunE: promptRemoveRunE}
	cmd.Flags().Bool("global", false, "Remove global prompt")
	cmd.SetArgs([]string{name})

	var out bytes.Buffer
	outfmt.SetOutputWriter(&out)
	defer outfmt.SetOutputWriter(os.Stdout)

	errOut := captureStderr(t, func() {
		if err := cmd.Execute(); err != nil {
			t.Fatalf("prompt remove: %v", err)
		}
	})
	return out.String(), errOut
}

// captureStderr runs fn with os.Stderr redirected to a pipe and returns
// everything written to it.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()
	return string(b)
}

// TestPromptRemoveResetsPollutedRole is the regression test for issue #24:
// a role row with empty skills/tools referencing the removed prompt (the
// stale INSERT-bug shape) must be auto-reset, so one `prompt remove` command
// fully restores the role.
func TestPromptRemoveResetsPollutedRole(t *testing.T) {
	projectRoot := promptTestEnv(t)
	editorPath := writePromptFile(t, filepath.Join(projectRoot, ".dscli", "prompt"), "editor")

	sid := session.GetCurrentSessionID(t.Context())
	insertRoleConfig(t, "dev", sid, "", "", "editor") // stale INSERT-bug row

	stdout, stderr := runPromptRemove(t, "editor", false)

	if _, err := os.Stat(editorPath); !os.IsNotExist(err) {
		t.Errorf("editor.md should be removed, stat err = %v", err)
	}
	if n := roleConfigCount(t, "dev", sid); n != 0 {
		t.Errorf("polluted dev row should be reset, count = %d", n)
	}
	if !strings.Contains(stdout, "已重置角色 dev") {
		t.Errorf("stdout missing reset report, got: %q", stdout)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %q", stderr)
	}
}

// TestPromptRemoveWarnsOnCleanReference verifies that a legitimate config
// referencing the removed prompt is only warned about, never deleted.
func TestPromptRemoveWarnsOnCleanReference(t *testing.T) {
	projectRoot := promptTestEnv(t)
	editorPath := writePromptFile(t, filepath.Join(projectRoot, ".dscli", "prompt"), "editor")

	sid := session.GetCurrentSessionID(t.Context())
	insertRoleConfig(t, "dev", sid, "all", "all", "editor") // deliberate config

	stdout, stderr := runPromptRemove(t, "editor", false)

	if _, err := os.Stat(editorPath); !os.IsNotExist(err) {
		t.Errorf("editor.md should be removed, stat err = %v", err)
	}
	if n := roleConfigCount(t, "dev", sid); n != 1 {
		t.Errorf("clean dev row should be kept, count = %d", n)
	}
	if !strings.Contains(stderr, "警告: 角色 dev 仍引用") {
		t.Errorf("stderr missing dangling-reference warning, got: %q", stderr)
	}
	if strings.Contains(stdout, "已重置角色") {
		t.Errorf("clean row must not be reset, stdout: %q", stdout)
	}
}

// TestPromptRemoveGlobalScansAllSessions verifies that removing a global
// prompt scans every session, auto-resets dangling polluted rows, and keeps
// rows still served by their own project prompt file.
func TestPromptRemoveGlobalScansAllSessions(t *testing.T) {
	promptTestEnv(t) // temp config dir, DB and project root
	globalPath := writePromptFile(t, filepath.Join(config.ConfigDir, "prompt"), "editor")

	ctx := t.Context()
	sidDanglingA, err := session.CreateOrGetSessionIDByPath(ctx, filepath.Join(t.TempDir(), "proj-a"))
	if err != nil {
		t.Fatal(err)
	}
	sidDanglingB, err := session.CreateOrGetSessionIDByPath(ctx, filepath.Join(t.TempDir(), "proj-b"))
	if err != nil {
		t.Fatal(err)
	}
	// A project that keeps its own editor.md: its reference stays valid.
	projC := filepath.Join(t.TempDir(), "proj-c")
	sidStillValid, err := session.CreateOrGetSessionIDByPath(ctx, projC)
	if err != nil {
		t.Fatal(err)
	}
	writePromptFile(t, filepath.Join(projC, ".dscli", "prompt"), "editor")

	insertRoleConfig(t, "dev", sidDanglingA, "", "", "editor")
	insertRoleConfig(t, "review", sidDanglingB, "", "", "editor")
	insertRoleConfig(t, "dev", sidStillValid, "", "", "editor")

	stdout, stderr := runPromptRemove(t, "editor", true)

	if _, err := os.Stat(globalPath); !os.IsNotExist(err) {
		t.Errorf("global editor.md should be removed, stat err = %v", err)
	}
	if n := roleConfigCount(t, "dev", sidDanglingA); n != 0 {
		t.Errorf("dangling polluted row (proj-a) should be reset, count = %d", n)
	}
	if n := roleConfigCount(t, "review", sidDanglingB); n != 0 {
		t.Errorf("dangling polluted row (proj-b) should be reset, count = %d", n)
	}
	if n := roleConfigCount(t, "dev", sidStillValid); n != 1 {
		t.Errorf("row still served by project editor.md should be kept, count = %d", n)
	}
	if !strings.Contains(stdout, "已重置角色 dev") || !strings.Contains(stdout, "已重置角色 review") {
		t.Errorf("stdout missing reset reports, got: %q", stdout)
	}
	if stderr != "" {
		t.Errorf("unexpected stderr: %q", stderr)
	}
}
