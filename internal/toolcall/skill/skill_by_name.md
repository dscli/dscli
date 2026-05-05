Fetch skill by name.

Fetch a skill's full content by name. Skills contain best
practices, tips, conventions, and executable scripts.

Returns:
- SKILL.md content (instructions)
- Resource listings (scripts, references, templates, examples)
  with absolute paths

Usage:
  skill_by_name(skill_name="test-skill")

Notes:
- Scripts are listed with paths; execute via shell (bash <path>)
- Reference documents can be read via read_file or shell (cat <path>)
