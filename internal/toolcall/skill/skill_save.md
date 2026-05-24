# skill_save

Save a local skill — create or update with partial merge.

Creates a local skill in .dscli/skills/ with proper YAML
frontmatter, or updates an existing one. When updating,
only provided fields are changed; omitted fields retain
their existing values.

For new skills, name, description, and content are required.
For existing skills, only name is required — provide only
the fields you want to change.

Creates or overwrites SKILL.md with merged frontmatter fields,
making it discoverable via skill_search and loadable via
skill_by_name.

Usage (create):
  skill_save(name="go-fix", description="Go code modernization helper",
    content="# go-fix

...", keywords="go, fix, modernize")

Usage (update keywords only):
  skill_save(name="go-fix", keywords="go, fix, modernize, refactor")

Usage (toggle auto_inject):
  skill_save(name="go-fix", auto_inject=true)

Parameters:

- name: skill name (required, e.g. "go-fix")

- description: skill description (required for new, optional for update)

- content: skill body in Markdown without frontmatter (required for new, optional for update)

- keywords: comma-separated keywords (optional, e.g. "go, modernize")

- auto_inject: auto-inject into conversation context (optional, default false for new, preserved on update)

After saving, the skill is immediately usable via skill_by_name
and skill_search.
