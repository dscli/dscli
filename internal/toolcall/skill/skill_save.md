Save a local skill

Create or update a local skill in .dscli/skills/ with proper
YAML frontmatter.

Creates or overwrites SKILL.md with name, description,
keywords, and auto_inject frontmatter fields, making it
discoverable via skill_search and loadable via skill_by_name.

If a skill with the same name already exists locally, it will
be overwritten.

Usage:
  skill_save(name="go-fix", description="Go code modernization helper",
    content="# go-fix\n\n...", keywords="go, fix, modernize")

Parameters:
- name: skill name (required, e.g. "go-fix")
- description: skill description for display and search (required)
- content: skill body in Markdown, without frontmatter (required)
- keywords: comma-separated search keywords (optional, e.g. "go, modernize")
- auto_inject: auto-inject into conversation context (optional, default false)

After saving, the skill is immediately usable via skill_by_name
and skill_search.
