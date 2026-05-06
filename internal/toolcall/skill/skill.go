package skill

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	pctx "gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed skill_by_name.md
var skill_by_name_md string

//go:embed skill_search.md
var skill_search_md string

//go:embed skill_save.md
var skill_save_md string

//go:embed skill_set_auto_inject.md
var skill_set_auto_inject_md string

var RegisterTool = toolcall.RegisterTool

type (
	ToolArgs  = toolcall.ToolArgs
	ToolDef   = toolcall.ToolDef
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}

// handleSkillByName fetches a skill's full content by exact name.
func handleSkillByName(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	skillName := ToolArgsValue(args, "skill_name", "")
	if skillName == "" {
		err = fmt.Errorf("skill name can not be empty")
		return result, warning, err
	}
	outfmt.Printf("Fetching skill [%s]\n", skillName)

	// Use markdown-based skill system
	skillContent, err := skills.Use(skillName)
	if err != nil {
		err = fmt.Errorf("failed to fetch skill %s: %w", skillName, err)
		return result, warning, err
	}

	if skillContent == "" {
		result = fmt.Sprintf("Skill %q exists but has no content.", skillName)
		return result, warning, err
	}

	result = skillContent
	return result, warning, err
}

// handleSkillSearch searches skills by keyword query.
func handleSkillSearch(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	query := ToolArgsValue(args, "query", "")
	if query == "" {
		err = fmt.Errorf("search query cannot be empty")
		return result, warning, err
	}
	outfmt.Printf("Searching skills [%s]\n", query)

	result, err = skills.Query(query)
	if err != nil {
		err = fmt.Errorf("skill search failed: %w", err)
		return result, warning, err
	}
	return result, warning, err
}

// handleSkillSave creates or updates a local skill (in .dscli/skills/) with proper frontmatter.
// For new skills: name, description, and content are required.
// For existing skills: only name is required — omitted fields keep existing values.
func handleSkillSave(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	name := ToolArgsValue(args, "name", "")
	description := ToolArgsValue(args, "description", "")
	bodyContent := ToolArgsValue(args, "content", "")
	keywordsStr := ToolArgsValue(args, "keywords", "")
	autoInject := ToolArgsValue(args, "auto_inject", false)

	// name is always required
	if name == "" {
		err = fmt.Errorf("skill name cannot be empty")
		return result, warning, err
	}

	// Detect which fields were explicitly provided (present in args map)
	_, hasDescription := args["description"]
	_, hasContent := args["content"]
	_, hasKeywords := args["keywords"]
	_, hasAutoInject := args["auto_inject"]

	// Try to read existing skill on disk for partial update merge
	skillFile := filepath.Join(pctx.ProjectRoot, ".dscli", "skills", name, "SKILL.md")
	var existing *skills.Skill
	if _, statErr := os.Stat(skillFile); statErr == nil {
		var s skills.Skill
		if parseErr := skills.ParseSkill(skillFile, &s); parseErr == nil {
			existing = &s
		}
	}

	if existing == nil {
		// Creating new skill: description and content are required
		if description == "" {
			err = fmt.Errorf("skill description cannot be empty for new skills")
			return result, warning, err
		}
		if bodyContent == "" {
			err = fmt.Errorf("skill content cannot be empty for new skills")
			return result, warning, err
		}
	} else {
		// Updating existing skill: merge — use new values where provided, keep existing otherwise
		if !hasDescription {
			description = existing.Description
		}
		if !hasContent {
			bodyContent = existing.Content
		}
		if !hasKeywords && keywordsStr == "" {
			keywordsStr = strings.Join(existing.Keywords, ", ")
		}
		if !hasAutoInject {
			autoInject = existing.AutoInject
		}
	}

	outfmt.Printf("Saving skill [%s]\n", name)
	result, warning, err = skills.HandleSkillCreate(ctx, name, description, bodyContent, keywordsStr, autoInject)
	return result, warning, err
}

// handleSkillSetAutoInject toggles auto-inject for an existing skill (local by default).
func handleSkillSetAutoInject(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	skillName := ToolArgsValue(args, "skill_name", "")
	autoInject := ToolArgsValue(args, "auto_inject", false)

	if skillName == "" {
		err = fmt.Errorf("skill_name cannot be empty")
		return result, warning, err
	}

	if err = skills.SetAutoInject(skillName, autoInject, false); err != nil {
		return result, warning, err
	}

	state := "disabled"
	if autoInject {
		state = "enabled"
	}
	result = fmt.Sprintf("Auto-inject %s for skill %q.", state, skillName)
	return result, warning, err
}

// init registers the skill tools with the global registry.
func init() {
	// Register skill_by_name — fetch a skill's full content by exact name.
	RegisterTool(ToolDef{
		Name:        "skill_by_name",
		DisplayName: "Get Skill",
		Description: skill_by_name_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Exact skill name (case-sensitive)",
				},
			},
			"required":             []string{"skill_name"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillByName,
	})

	// Register skill_search — discover skills by keyword query.
	RegisterTool(ToolDef{
		Name:        "skill_search",
		DisplayName: "Search Skills",
		Description: skill_search_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "Search keywords (space-separated)",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillSearch,
	})

	// Register skill_save — create or update a local skill with partial merge support.
	// When updating an existing skill, only name is required; omitted fields preserve existing values.
	RegisterTool(ToolDef{
		Name:        "skill_save",
		DisplayName: "Save Skill",
		Description: skill_save_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (e.g. go-fix), used as directory name and identifier",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Skill description for display and search (required for new, optional for update)",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Skill body in Markdown, without frontmatter (required for new, optional for update)",
				},
				"keywords": map[string]any{
					"type":        "string",
					"description": "Comma-separated keywords for skill_search discovery (optional, e.g. \"go, modernize\")",
				},
				"auto_inject": map[string]any{
					"type":        "boolean",
					"description": "Whether to auto-inject full skill content into each conversation (optional, default false)",
				},
			},
			"required":             []string{"name"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillSave,
	})

	// Register skill_set_auto_inject — toggle auto-inject for an existing skill.
	RegisterTool(ToolDef{
		Name:        "skill_set_auto_inject",
		DisplayName: "Set Auto-Inject",
		Description: skill_set_auto_inject_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Exact skill name (required, case-sensitive)",
				},
				"auto_inject": map[string]any{
					"type":        "boolean",
					"description": "Whether to auto-inject (required)",
				},
			},
			"required":             []string{"skill_name", "auto_inject"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillSetAutoInject,
	})
}
