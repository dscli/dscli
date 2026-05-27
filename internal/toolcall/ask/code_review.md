# code_review

Code review via expert.

Review recent commit(s) with expert-level improvement
suggestions.  Checks for uncommitted changes first; optionally runs
tests before review.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed.

**Parameters**: `summary` (required), `test_command` (optional), `since` (optional, default `-1` — review last commit; use `-2` for last 2, `-3` for last 3, etc., equivalent to `HEAD~N`), `timeout` (optional).

Timeout: default 300s. Set `timeout` (seconds) to override — set longer (e.g. 600) for large projects with many tests.

Use before pushing code or to learn better practices.
