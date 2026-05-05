package skill

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

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
// It overwrites if a skill with the same name already exists locally.
func handleSkillSave(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	name := ToolArgsValue(args, "name", "")
	description := ToolArgsValue(args, "description", "")
	bodyContent := ToolArgsValue(args, "content", "")
	keywordsStr := ToolArgsValue(args, "keywords", "")
	autoInject := ToolArgsValue(args, "auto_inject", false)

	// Validate required params
	if name == "" {
		err = fmt.Errorf("skill name cannot be empty")
		return result, warning, err
	}
	if description == "" {
		err = fmt.Errorf("skill description cannot be empty")
		return result, warning, err
	}
	if bodyContent == "" {
		err = fmt.Errorf("skill content cannot be empty")
		return result, warning, err
	}

	outfmt.Printf("Saving skill [%s]\n", name)
	result, warning, err = skills.HandleSkillCreate(ctx, name, description, bodyContent, keywordsStr, autoInject)
	return result, warning, err
}

// init registers the skill tools with the global registry.
func init() {
	// Register skill_by_name — fetch a skill's full content by exact name.
	RegisterTool(ToolDef{
		Name:        "skill_by_name",
		DisplayName: "Get Skill",
		Description: `Fetch skill by name.

Fetch a skill's full content by name. Skills contain best practices, tips, conventions, and executable scripts.

Returns:
- SKILL.md content (instructions)
- Resource listings (scripts, references, templates, examples) with absolute paths

Usage:
  skill_by_name(skill_name="test-skill")

Notes:
- Scripts are listed with paths; execute via shell (bash <path>)
- Reference documents can be read via read_file or shell (cat <path>)`,
		Strict: true,
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
		Description: `Search skills by keyword.

Search available skills by keyword. Use when unsure which skill to use or to discover relevant skills for a task.

Usage:
  skill_search(query="test")
  skill_search(query="build deploy")

Notes:
- query is case-insensitive, max 128 chars
- Returns matching skill names with description summaries
- After finding a skill, use skill_by_name to get full content`,
		Strict: true,
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

	// Register skill_save — create or update a local skill with YAML frontmatter.
	RegisterTool(ToolDef{
		Name:        "skill_save",
		DisplayName: "Save Skill",
		Description: `Save a local skill.

Create or update a local skill in .dscli/skills/ with proper YAML frontmatter.

Creates or overwrites SKILL.md with name, description, keywords, and auto_inject frontmatter fields,
making it discoverable via skill_search and loadable via skill_by_name.

If a skill with the same name already exists locally, it will be overwritten.

Usage:
  skill_save(name="go-fix", description="Go code modernization helper", content="# go-fix\n\n...", keywords="go, fix, modernize")

Parameters:
- name: skill name (required, e.g. "go-fix")
- description: skill description for display and search (required)
- content: skill body in Markdown, without frontmatter (required)
- keywords: comma-separated search keywords (optional, e.g. "go, modernize, refactor")
- auto_inject: auto-inject into conversation context (optional, default false)

After saving, the skill is immediately usable via skill_by_name and skill_search.`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Skill name (e.g. go-fix), used as directory name and identifier",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "Skill description for display in lists and search matching",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Skill body in Markdown format (without YAML frontmatter, it will be added automatically)",
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
			"required":             []string{"name", "description", "content"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillSave,
	})
}
