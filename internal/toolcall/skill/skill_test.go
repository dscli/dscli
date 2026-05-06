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

	for _, name := range []string{"skill_by_name", "skill_search", "skill_save", "skill_set_auto_inject"} {
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

// TestHandleSkillSave verifies that handleSkillSave creates a valid SKILL.md
// with frontmatter, and that the resulting skill can be loaded and queried.
func TestHandleSkillSave(t *testing.T) {
	// Save and restore original ProjectRoot
	origRoot := pctx.ProjectRoot
	defer func() { pctx.ProjectRoot = origRoot }()

	// Use a temp dir as project root
	tmpDir := t.TempDir()
	pctx.ProjectRoot = tmpDir

	// Reset cached local store so it re-initializes with the new ProjectRoot
	skills.ResetLocalStore()

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

	content, _, err := handleSkillSave(ctx, args)
	if err != nil {
		t.Fatalf("handleSkillSave failed: %v", err)
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

	// Verify overwrite (full update — all fields provided)
	args["content"] = "# Updated\n\nNew body."
	content2, _, err2 := handleSkillSave(ctx, args)
	if err2 != nil {
		t.Fatalf("handleSkillSave (overwrite) failed: %v", err2)
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

// TestHandleSkillSavePartialUpdate verifies partial update behavior:
// when updating an existing skill with only some fields, the omitted
// fields retain their existing values.
func TestHandleSkillSavePartialUpdate(t *testing.T) {
	origRoot := pctx.ProjectRoot
	defer func() { pctx.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	pctx.ProjectRoot = tmpDir

	// Reset cached local store so it re-initializes with the new ProjectRoot
	skills.ResetLocalStore()

	ctx := context.Background()

	// Step 1: Create a skill with all fields
	createArgs := toolcall.ToolArgs{
		"name":        "partial-test",
		"description": "Original description",
		"content":     "# Original\n\nBody.",
		"keywords":    "go, test",
		"auto_inject": true,
	}
	_, _, err := handleSkillSave(ctx, createArgs)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Step 2: Update only keywords — description, content, auto_inject should be preserved
	updateArgs := toolcall.ToolArgs{
		"name":     "partial-test",
		"keywords": "go, test, updated",
	}
	_, _, err = handleSkillSave(ctx, updateArgs)
	if err != nil {
		t.Fatalf("partial update failed: %v", err)
	}

	// Verify: description and content preserved, keywords updated
	skillFile := filepath.Join(tmpDir, ".dscli", "skills", "partial-test", "SKILL.md")
	var skill skills.Skill
	if err := skills.ParseSkill(skillFile, &skill); err != nil {
		t.Fatalf("ParseSkill failed: %v", err)
	}
	if skill.Description != "Original description" {
		t.Errorf("Description = %q, want %q (should be preserved)", skill.Description, "Original description")
	}
	// ParseSkill appends \n to content
	if skill.Content != "# Original\n\nBody.\n" {
		t.Errorf("Content = %q, want %q (should be preserved)", skill.Content, "# Original\n\nBody.\n")
	}
	if !skill.AutoInject {
		t.Error("AutoInject should still be true (preserved)")
	}
	if len(skill.Keywords) != 3 {
		t.Errorf("Keywords len = %d, want 3: %v", len(skill.Keywords), skill.Keywords)
	}

	// Step 3: Update only auto_inject to false — other fields preserved
	updateArgs2 := toolcall.ToolArgs{
		"name":        "partial-test",
		"auto_inject": false,
	}
	_, _, err = handleSkillSave(ctx, updateArgs2)
	if err != nil {
		t.Fatalf("auto_inject update failed: %v", err)
	}

	var skill2 skills.Skill
	if err := skills.ParseSkill(skillFile, &skill2); err != nil {
		t.Fatalf("ParseSkill after auto_inject update failed: %v", err)
	}
	if skill2.AutoInject {
		t.Error("AutoInject should be false after update")
	}
	if skill2.Description != "Original description" {
		t.Error("Description should still be preserved")
	}
}

// TestHandleSkillSetAutoInject verifies the skill_set_auto_inject tool.
func TestHandleSkillSetAutoInject(t *testing.T) {
	origRoot := pctx.ProjectRoot
	defer func() { pctx.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	pctx.ProjectRoot = tmpDir

	// Reset cached local store so it re-initializes with the new ProjectRoot
	skills.ResetLocalStore()

	ctx := context.Background()

	// Create a skill first (auto_inject = false)
	createArgs := toolcall.ToolArgs{
		"name":        "auto-inject-test",
		"description": "Test auto inject toggle",
		"content":     "# Auto Inject Test\n\nBody.",
	}
	_, _, err := handleSkillSave(ctx, createArgs)
	if err != nil {
		t.Fatalf("create failed: %v", err)
	}

	// Enable auto-inject
	enableArgs := toolcall.ToolArgs{
		"skill_name":  "auto-inject-test",
		"auto_inject": true,
	}
	result, _, err := handleSkillSetAutoInject(ctx, enableArgs)
	if err != nil {
		t.Fatalf("enable auto_inject failed: %v", err)
	}
	t.Logf("enable result: %s", result)

	// Verify via store
	_, useErr := skills.Use("auto-inject-test")
	if useErr != nil {
		t.Fatalf("skills.Use failed: %v", useErr)
	}

	// Disable auto-inject
	disableArgs := toolcall.ToolArgs{
		"skill_name":  "auto-inject-test",
		"auto_inject": false,
	}
	result2, _, err := handleSkillSetAutoInject(ctx, disableArgs)
	if err != nil {
		t.Fatalf("disable auto_inject failed: %v", err)
	}
	t.Logf("disable result: %s", result2)
}

// TestHandleSkillSaveValidation tests input validation.
func TestHandleSkillSaveValidation(t *testing.T) {
	origRoot := pctx.ProjectRoot
	defer func() { pctx.ProjectRoot = origRoot }()

	tmpDir := t.TempDir()
	pctx.ProjectRoot = tmpDir

	// Reset cached local store so it re-initializes with the new ProjectRoot
	skills.ResetLocalStore()

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
			_, _, err := handleSkillSave(ctx, tt.args)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tt.name)
			}
		})
	}
}
