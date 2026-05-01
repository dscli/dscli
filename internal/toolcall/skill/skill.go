package skill

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/skills"
	"gitcode.com/dscli/dscli/internal/toolcall"

	pctx "gitcode.com/dscli/dscli/internal/context"
)

var (
	RegisterTool = toolcall.RegisterTool
)

type (
	ToolArgs  = toolcall.ToolArgs
	ToolDef   = toolcall.ToolDef
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}

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

// handleSkillCreate creates a new local skill (in .dscli/skills/) with proper frontmatter.
// It overwrites if a skill with the same name already exists locally.
func handleSkillCreate(ctx context.Context, args ToolArgs) (content string, user string, err error) {
	name := ToolArgsValue(args, "name", "")
	description := ToolArgsValue(args, "description", "")
	bodyContent := ToolArgsValue(args, "content", "")
	keywordsStr := ToolArgsValue(args, "keywords", "")
	autoInject := ToolArgsValue(args, "auto_inject", false)

	// Validate required params
	if name == "" {
		err = fmt.Errorf("skill name cannot be empty")
		return
	}
	if description == "" {
		err = fmt.Errorf("skill description cannot be empty")
		return
	}
	if bodyContent == "" {
		err = fmt.Errorf("skill content cannot be empty")
		return
	}

	outfmt.Printf("Creating skill [%s]\n", name)

	// Parse keywords from comma-separated string
	var keywords []string
	if keywordsStr != "" {
		for _, kw := range strings.Split(keywordsStr, ",") {
			kw = strings.TrimSpace(kw)
			if kw != "" {
				keywords = append(keywords, kw)
			}
		}
	}

	// Build skill struct
	skill := skills.Skill{
		Name:        name,
		Description: description,
		Content:     bodyContent,
		Keywords:    keywords,
		AutoInject:  autoInject,
	}

	// Generate SKILL.md content with frontmatter
	skillMD, err := skills.FormatSkillMD(&skill)
	if err != nil {
		err = fmt.Errorf("failed to format SKILL.md: %w", err)
		return
	}

	// Create local skill directory: .dscli/skills/<name>/
	localDir := filepath.Join(pctx.ProjectRoot, ".dscli", "skills", name)
	if err = os.MkdirAll(localDir, 0o755); err != nil {
		err = fmt.Errorf("failed to create skill directory: %w", err)
		return
	}

	// Write SKILL.md
	skillFile := filepath.Join(localDir, "SKILL.md")
	if err = os.WriteFile(skillFile, []byte(skillMD), 0o644); err != nil {
		err = fmt.Errorf("failed to write SKILL.md: %w", err)
		return
	}

	// Register in local store so it's immediately usable via skill_by_name / skill_search
	localStore, storeErr := skills.LocalStore()
	if storeErr != nil {
		// Non-fatal: skill file is on disk, will be picked up on next load
		outfmt.Printf("Warning: could not update local store cache: %v\n", storeErr)
		content = fmt.Sprintf("Skill %q created at %s (store cache update skipped).", name, localDir)
		return
	}

	// Parse the newly created SKILL.md to get the full Skill (with resources, etc.)
	var parsedSkill skills.Skill
	if parseErr := skills.ParseSkill(skillFile, &parsedSkill); parseErr != nil {
		err = fmt.Errorf("failed to parse created skill: %w", parseErr)
		return
	}

	// Preserve auto_inject if set
	if autoInject {
		parsedSkill.AutoInject = true
	}

	// Add to in-memory store
	localStore.Skills[name] = parsedSkill

	// Update keywords index: ensure no duplicates
	for _, kw := range parsedSkill.Keywords {
		names := localStore.Keywords[kw]
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			localStore.Keywords[kw] = append(names, name)
		}
	}

	// Persist skills.yaml
	if saveErr := localStore.Save(); saveErr != nil {
		outfmt.Printf("Warning: failed to save skills.yaml: %v\n", saveErr)
	}

	content = fmt.Sprintf("Local skill %q created successfully.\n\nPath: %s\nKeywords: %s",
		name, localDir, strings.Join(parsedSkill.Keywords, ", "))
	return
}

// init registers the skill tools with the global registry.
func init() {
	// Register skill_by_name — fetch a skill's full content by exact name.
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
- Scripts are listed with paths; execute via shell (bash &lt;path&gt;)
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
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Category: "skill",
		Handler:  handleSkillSearch,
	})

	// Register skill_create — create a new local skill with YAML frontmatter.
	RegisterTool(ToolDef{
		Name:        "skill_create",
		DisplayName: "Create Skill",
		Description: `Create a new local skill in .dscli/skills/ with proper YAML frontmatter.

Creates SKILL.md with name, description, keywords, and auto_inject frontmatter fields,
making it discoverable via skill_search and loadable via skill_by_name.

If a skill with the same name already exists locally, it will be overwritten.

Usage:
  skill_create(name="go-fix", description="Go code modernization helper", content="# go-fix\n\n...", keywords="go, fix, modernize")

Parameters:
- name: skill name (required, e.g. "go-fix")
- description: skill description for display and search (required)
- content: skill body in Markdown, without frontmatter (required)
- keywords: comma-separated search keywords (optional, e.g. "go, modernize, refactor")
- auto_inject: auto-inject into conversation context (optional, default false)

After creation, the skill is immediately usable via skill_by_name and skill_search.`,
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
		Handler:  handleSkillCreate,
	})
}
