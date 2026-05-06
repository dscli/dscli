Toggle auto-inject for a skill.

Enable or disable automatic injection of a skill's full
content into each conversation context. When enabled, the
skill is automatically loaded without the LLM needing to
explicitly call skill_by_name.

Usage:
  skill_set_auto_inject(skill_name="use-modern-go", auto_inject=true)

Parameters:
- skill_name: exact skill name (required, case-sensitive)
- auto_inject: whether to auto-inject (required, boolean)
