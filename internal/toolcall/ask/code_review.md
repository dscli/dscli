# code_review

Code review via expert.

Review recent commit(s) with expert-level improvement
suggestions.  Checks for uncommitted changes first; optionally runs
tests before review.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed.

**Parameters**: `summary` (required), `test_command` (optional), `since` (optional, default `-1` — review last commit; use `-2` for last 2, `-3` for last 3, etc., equivalent to `HEAD~N`), `timeout` (optional).

Timeout: default 900s (15 min). The expert may run several tool-call rounds
(exec_command) before producing the final review, so set `timeout` (seconds)
longer for large projects with many tests. If the input is too large for the
DeepSeek chat box, the expert inspects the repo via tools — results are fed
back automatically in the same conversation.

Use before pushing code or to learn better practices.

**Context**: the first message carries only the commit message and the diff.
The expert has tool access (read_file, exec_command) in the same conversation
and reads AGENTS.md plus full file contents on demand, keeping the request
under the chat-box input limit. If a diff still exceeds the limit, per-file
sections are dropped smallest-first and listed in the tool warning — the
expert can read those files via read_file.
