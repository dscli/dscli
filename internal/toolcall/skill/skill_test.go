package skill

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gitcode.com/dscli/dscli/internal/skills"
	"gitcode.com/dscli/dscli/internal/toolcall"

	pctx "gitcode.com/dscli/dscli/internal/context"
)

// TestRegistration verifies all skill tools are registered via init().
func TestRegistration(t *testing.T) {
	ctx := context.Background()

	for _, name := range []string{"skill_by_name", "skill_search", "skill_create"} {
		t.Run(name, func(t *testing.T) {
			tool, ok := toolcall.GetToolDef(ctx, name)
			if !ok {
				t.Fatalf("tool %q has not been registered; init() may not have been called", name)
			}
			if tool.Name != name {
				t.Errorf("Name = %q, want %q", tool.Name, name)
			}
			if tool.Handler == nil {
				t.Error("Handler is nil")
			}
			if tool.DisplayName == "" {
				t.Error("DisplayName is empty")
			}
			if tool.Description == "" {
				t.Error("Description is empty")
			}
			if tool.Category == "" {
				t.Error("Category is empty")
			}
		})
	}
}

// TestHandleSkillCreate verifies that handleSkillCreate creates a valid SKILL.md
// with frontmatter, and that the resulting skill can be loaded and queried.
func TestHandleSkillCreate(t *testing.T) {
	// Save and restore original ProjectRoot
	origRoot := pctx.ProjectRoot
	defer func() { pctx.ProjectRoot = origRoot }()

	// Use a temp dir as project root
	tmpDir := t.TempDir()
	pctx.ProjectRoot = tmpDir

	// Ensure .dscli/skills dir exists
	skillsDir := filepath.Join(tmpDir, ".dscli", "skills")
	if err := os.MkdirAll(skillsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	args := toolcall.ToolArgs{
		"name":        "test-skill",
		"description": "A test skill for unit testing",
		"content":     "# Test Skill\n\nThis is the body.",
		"keywords":    "test, unit-test, go",
		"auto_inject": true,
	}

	content, _, err := handleSkillCreate(ctx, args)
	if err != nil {
		t.Fatalf("handleSkillCreate failed: %v", err)
	}
	t.Logf("result: %s", content)

	// Verify SKILL.md exists
	skillFile := filepath.Join(skillsDir, "test-skill", "SKILL.md")
	if _, statErr := os.Stat(skillFile); statErr != nil {
		t.Fatalf("SKILL.md not created: %v", statErr)
	}

	// Verify it parses correctly
	var skill skills.Skill
	if err := skills.ParseSkill(skillFile, &skill); err != nil {
		t.Fatalf("ParseSkill failed: %v", err)
	}
	if skill.Name != "test-skill" {
		t.Errorf("Name = %q, want %q", skill.Name, "test-skill")
	}
	if skill.Description != "A test skill for unit testing" {
		t.Errorf("Description = %q", skill.Description)
	}
	if len(skill.Keywords) != 3 {
		t.Errorf("Keywords len = %d, want 3", len(skill.Keywords))
	}
	if !skill.AutoInject {
		t.Error("AutoInject should be true")
	}

	// Verify immediate availability via skill_by_name / skill_search
	// (local store should be updated)
	used, useErr := skills.Use("test-skill")
	if useErr != nil {
		t.Fatalf("skills.Use failed: %v", useErr)
	}
	if used == "" {
		t.Error("Use returned empty content")
	}
	t.Logf("Use result preview: %.80s...", used)

	// Verify search by keyword
	searchResult, searchErr := skills.Query("unit-test")
	if searchErr != nil {
		t.Fatalf("skills.Query failed: %v", searchErr)
	}
	if searchResult == "" {
		t.Error("Query returned empty result")
	}
	t.Logf("Query result: %s", searchResult)

	// Verify overwrite
	args["content"] = "# Updated\n\nNew body."
	content2, _, err2 := handleSkillCreate(ctx, args)
	if err2 != nil {
		t.Fatalf("handleSkillCreate (overwrite) failed: %v", err2)
	}
	t.Logf("overwrite result: %s", content2)

	// Verify updated content
	used2, useErr2 := skills.Use("test-skill")
	if useErr2 != nil {
		t.Fatalf("skills.Use after overwrite failed: %v", useErr2)
	}
	if used2 == "" {
		t.Error("Use returned empty content after overwrite")
	}
}

// TestHandleSkillCreateValidation tests input validation.
func TestHandleSkillCreateValidation(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name string
		args toolcall.ToolArgs
	}{
		{"missing name", toolcall.ToolArgs{"description": "d", "content": "c"}},
		{"missing description", toolcall.ToolArgs{"name": "n", "content": "c"}},
		{"missing content", toolcall.ToolArgs{"name": "n", "description": "d"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := handleSkillCreate(ctx, tt.args)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}
