# code_review

Code review via expert.

Review recent commit(s) with expert-level improvement
suggestions.  Checks for uncommitted changes first; optionally runs
tests before review.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed.

**Parameters**: `summary` (required), `test_command` (optional), `since` (optional, default `-1` — review last commit; use `-2` for last 2, `-3` for last 3, etc., equivalent to `HEAD~N`), `timeout` (optional).

Timeout: default 300s. Set `timeout` (seconds) to override — set longer (e.g. 600) for large projects with many tests.

Use before pushing code or to learn better practices.

**Context**: includes the repo's AGENTS.md when present, plus full contents of
changed files (files over 100KB are diff-only). Oversized inputs are condensed
to hunk-context excerpts with complete enclosing function definitions; any
files dropped from the review are listed in the tool warning.
