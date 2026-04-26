package misc

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
)

// handleSkillByName fetches a skill's full content by exact name.
func handleSkillByName(ctx context.Context, args ToolArgs) (content string, user string, err error) {
	skillName := ToolArgsValue(args, "skill_name", "")
	if skillName == "" {
		err = fmt.Errorf("skill name can not be empty")
		return
	}
	outfmt.Printf("Fetching skill [%s]\n", skillName)

	// Use markdown-based skill system
	skillContent, err := skills.Use(skillName)
	if err != nil {
		err = fmt.Errorf("failed to fetch skill %s: %w", skillName, err)
		return
	}

	if skillContent == "" {
		content = fmt.Sprintf("Skill %q exists but has no content.", skillName)
		return
	}

	content = skillContent
	return
}

func init() {
	// Register skill_by_name tool
	RegisterTool(ToolDef{
		Name:        "skill_by_name",
		DisplayName: "Get Skill",
		Description: `Fetch a skill's full content by name. Skills contain best practices, tips, conventions, and executable scripts.

Returns:
- SKILL.md content (instructions)
- Resource listings (scripts, references, templates, examples) with absolute paths

Usage:
  skill_by_name(skill_name="test-skill")

Notes:
- skill_name is case-sensitive, max 128 chars
- Scripts are listed with paths; execute via shell (bash <path>) or python (python3 <path>) tools
- Reference documents can be read via read_file or shell (cat <path>)`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"skill_name": map[string]any{
					"type":        "string",
					"description": "Exact skill name (case-sensitive)",
					"pattern":     TitleLikePattern(128),
				},
			},
			"required":             []string{"skill_name"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillByName,
	})
}

// handleSkillSearch searches skills by keyword query.
func handleSkillSearch(ctx context.Context, args ToolArgs) (content string, user string, err error) {
	query := ToolArgsValue(args, "query", "")
	if query == "" {
		err = fmt.Errorf("search query cannot be empty")
		return
	}
	outfmt.Printf("Searching skills [%s]\n", query)

	result, err := skills.Query(query)
	if err != nil {
		err = fmt.Errorf("skill search failed: %w", err)
		return
	}

	content = result
	return
}

func init() {
	// Register skill_search tool
	RegisterTool(ToolDef{
		Name:        "skill_search",
		DisplayName: "Search Skills",
		Description: `Search available skills by keyword. Use this when unsure which skill to use or to discover relevant skills for a task.

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
					"pattern":     TitleLikePattern(128),
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillSearch,
	})
}
