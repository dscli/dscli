### How to Use a Skill

1. Call `skill_by_name(skill_name="...")` to fetch the full skill content
2. Read the SKILL.md content for instructions. Scripts are listed under `## Scripts` with paths.
3. To read a resource, use the path shown (e.g. `~/.dscli/skills/<name>/scripts/foo.sh`)
4. Execute scripts via `shell` tool:

   - Shell script: `bash <skill_path>/scripts/foo.sh <args>`
   - Python script: `python3 <skill_path>/scripts/bar.py <args>`
5. Reference documents and templates can be read with `read_file` or `cat` via shell

**Example** — user says "run tests", you see `test-skill` in the list:

```
Step 1: skill_by_name(skill_name="test-skill")
Step 2: Find the script path in the returned content (e.g. ~/.dscli/skills/test-skill/scripts/test.sh)
Step 3: Execute via shell tool: bash ~/.dscli/skills/test-skill/scripts/test.sh
```

**IMPORTANT**: Never guess or fabricate scripts. Always fetch the skill first.