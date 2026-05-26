# code_review

Code review via expert.

Review the most recent commit (HEAD) with expert-level improvement
suggestions.  Checks for uncommitted changes first; optionally runs
tests before review.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed.

Timeout: default 300s. Set `timeout` (seconds) to override — set longer (e.g. 600) for large projects with many tests.

Use before pushing code or to learn better practices.
