package session

import (
	"testing"
)

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
			break
		}
	}
	if !found {
		t.Errorf("session %d not found in project list", id)
	}
}
