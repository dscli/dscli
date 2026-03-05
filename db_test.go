package main

import (
	"strings"
	"testing"
)

func TestGetSessionID(t *testing.T) {
	sessionID, err := CreateOrGetSessionID()
	if err != nil || sessionID == 0 {
		t.Fatal(err, sessionID)
	}
}

// TestCreateSkillPlaceholderCount verifies that CreateSkill's SQL has matching
// column count and placeholder count.
func TestCreateSkillPlaceholderCount(t *testing.T) {
	_, err := CreateSkill("test-skill", "desc", "{}", "test", 50, false)
	if err != nil {
		// Accept "UNIQUE constraint" if the skill already exists — that proves
		// the SQL was valid enough to reach the DB engine.
		if strings.Contains(err.Error(), "UNIQUE") {
			return
		}
		// Any bind/placeholder error means the bug is back.
		if strings.Contains(err.Error(), "bind") ||
			strings.Contains(err.Error(), "column") {
			t.Fatalf("CreateSkill SQL placeholder mismatch regression: %v", err)
		}
		t.Logf("CreateSkill returned non-placeholder error (acceptable): %v", err)
	}
}
