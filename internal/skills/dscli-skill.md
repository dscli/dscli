---
name: dscli
description: dscli built-in skill. Documents dscli-unique tools: memory, skill management, session notes, role config.
keywords: [dscli, built-in]
auto_inject: true
---

# dscli-unique Tools

These tools are dscli-specific. Use them directly; their names and parameters are as listed.

## Memory (persistent, cross-session)

| Tool | Parameters |
|------|-----------|
| `mem_save` | `title` (required), `content` (required), `type` (optional: decision/bugfix/pattern/config/discovery/learning) |
| `mem_search` | `query` (required), `type` (optional), `limit` (optional, default 10) |
| `mem_update` | `id` (required), `title` (optional), `content` (optional), `type` (optional) |
| `mem_delete` | `id` (required) — irreversible |
| `mem_get_observation` | `id` (required) |
| `mem_stats` | none |

Before solving a problem, search memories with `mem_search` to avoid repeating past mistakes.

## Skill Management

| Tool | Parameters |
|------|-----------|
| `skill_save` | `name` (required), `description` (required for new), `content` (required for new), `keywords` (optional, comma-separated), `auto_inject` (optional, default false) |
| `skill_set_auto_inject` | `skill_name` (required), `auto_inject` (required, boolean) |
| `skill_search` | `query` (required) |
| `skill_by_name` | `skill_name` (required) |

CLI validation: `dscli skill validate <skill_name>` validates a skill by name or path.

## Session

| Tool | Parameters |
|------|-----------|
| `note` | `content` (required, ≤40 chars) — summary injected into next session |
| `recall` | `keywords` (required), `days` (optional, default 30), `limit` (optional, default 5) |

## Role Configuration

Each role controls its own tools, skills, and prompt via the `role_configs` table (SQLite). Changes take effect immediately. Use `dscli role` to manage.
