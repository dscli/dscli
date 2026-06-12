// Package roles does one thing: map role names to allowed skills/tools/prompts.
//
// # Why session_id, not project_path
//
// The sessions table maps session_id → project_path. When a user copies a
// project to another directory, they update sessions.project_path and
// everything keyed by session_id follows. If we used project_path directly,
// every table referencing project_path would need updating — doubled work,
// doubled risk.
//
// # Schema
//
//	role_configs (role TEXT, session_id INTEGER, skills TEXT, tools TEXT, prompt TEXT)
//	UNIQUE(role, session_id) — one config per role per session
//
// # Fallback chain
//
//	DB row exists? → use it
//	No row        → hardcoded: dev=all, expert=none, review=none, test=none
//
// # "all" vs ""
//
// In the skills/tools columns:
//   - "all" → ParseXxxList returns nil (no filtering; include everything)
//   - ""    → ParseXxxList returns []string{} (explicitly nothing)
//   - "a,b" → ParseXxxList returns ["a","b"]
//
// This convention lets callers do a single nil-check instead of comparing
// against both "all" and a full list.
package roles
