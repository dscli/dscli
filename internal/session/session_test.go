package session

import (
	"testing"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

func init() {
	// Register tables normally owned by ainame — session cannot import
	// ainame due to circular dependency, so we register them here for tests.
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS ai_names (
			id              INTEGER PRIMARY KEY,
			name_cn         TEXT NOT NULL,
			name_en         TEXT NOT NULL,
			bird_frog       TEXT NOT NULL DEFAULT 'frog',
			personality_cn  TEXT NOT NULL,
			personality_en  TEXT NOT NULL,
			desc_cn         TEXT NOT NULL,
			desc_en         TEXT NOT NULL,
			email           TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS session_names (
			id          INTEGER PRIMARY KEY,
			session_id  INTEGER NOT NULL,
			name_id     INTEGER NOT NULL DEFAULT 0,
			assigned_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id),
			FOREIGN KEY (name_id) REFERENCES ai_names(id),
			UNIQUE(session_id)
		)`,
	)
}

func TestGetSessionID(t *testing.T) {
	ctx := t.Context()
	sessionID, err := CreateOrGetSessionID(ctx)
	if err != nil || sessionID == 0 {
		t.Fatal(err, sessionID)
	}
}

func TestListProjects(t *testing.T) {
	// Ensure at least one session exists in the test DB.
	ctx := t.Context()
	id, err := CreateOrGetSessionID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	projects, err := ListProjects()
	if err != nil {
		t.Fatal(err)
	}

	if len(projects) == 0 {
		t.Fatal("expected at least one project")
	}

	found := false
	for _, p := range projects {
		if p.ID == id {
			found = true
			if p.ProjectPath == "" {
				t.Error("project_path should not be empty")
			}
			if p.CreatedAt == "" {
				t.Error("created_at should not be empty")
			}
			// Maintainer may be empty for sessions without an assigned name;
			// that's valid — we just verify the field exists in the row.
			break
		}
	}
	if !found {
		t.Errorf("session %d not found in project list", id)
	}
}
func TestAssignMaintainer(t *testing.T) {
	ctx := t.Context()
	sessionID, err := CreateOrGetSessionID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Insert a test name into ai_names (normally seeded by ainame).
	db, err := sqlite.OpenDB()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	const testNameID int64 = 99
	db.Exec(`INSERT OR IGNORE INTO ai_names (id, name_cn, name_en, bird_frog, personality_cn, personality_en, desc_cn, desc_en)
		VALUES (?, '测试', 'Test', 'frog', '', '', '', '')`, testNameID)

	// Assign maintainer.
	if err := AssignMaintainer(sessionID, testNameID); err != nil {
		t.Fatalf("AssignMaintainer: %v", err)
	}

	// Verify via ListProjects.
	projects, err := ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range projects {
		if p.ID == sessionID {
			found = true
			if p.MaintainerID != testNameID {
				t.Errorf("MaintainerID = %d, want %d", p.MaintainerID, testNameID)
			}
			if p.MaintainerCN != "测试" {
				t.Errorf("MaintainerCN = %q, want %q", p.MaintainerCN, "测试")
			}
			if p.MaintainerEN != "Test" {
				t.Errorf("MaintainerEN = %q, want %q", p.MaintainerEN, "Test")
			}
			break
		}
	}
	if !found {
		t.Errorf("session %d not found", sessionID)
	}

	// Re-assign (UPSERT) — should succeed.
	const newNameID int64 = 100
	db.Exec(`INSERT OR IGNORE INTO ai_names (id, name_cn, name_en, bird_frog, personality_cn, personality_en, desc_cn, desc_en)
		VALUES (?, '测试2', 'Test2', 'frog', '', '', '', '')`, newNameID)
	if err := AssignMaintainer(sessionID, newNameID); err != nil {
		t.Fatalf("AssignMaintainer (reassign): %v", err)
	}

	// Error: non-existent session.
	if err := AssignMaintainer(99999, testNameID); err == nil {
		t.Error("expected error for non-existent session")
	}

	// Error: non-existent name.
	if err := AssignMaintainer(sessionID, 99999); err == nil {
		t.Error("expected error for non-existent name")
	}
}

func TestUpdateProjectPath(t *testing.T) {
	ctx := t.Context()
	sessionID, err := CreateOrGetSessionID(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Happy path: update to a valid path.
	newPath := "/tmp/test-updated-project"
	if err := UpdateProjectPath(sessionID, newPath); err != nil {
		t.Fatalf("UpdateProjectPath: %v", err)
	}

	// Verify via ListProjects.
	projects, err := ListProjects()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, p := range projects {
		if p.ID == sessionID {
			found = true
			if p.ProjectPath != newPath {
				t.Errorf("ProjectPath = %q, want %q", p.ProjectPath, newPath)
			}
			break
		}
	}
	if !found {
		t.Errorf("session %d not found", sessionID)
	}

	// Error: non-existent session.
	if err := UpdateProjectPath(99999, "/tmp/foo"); err == nil {
		t.Error("expected error for non-existent session")
	}

	// Error: empty path.
	if err := UpdateProjectPath(sessionID, ""); err == nil {
		t.Error("expected error for empty path")
	}
}
