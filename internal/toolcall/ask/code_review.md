# code_review

Code review via expert.

Review recent commit(s) with expert-level improvement
suggestions.  Checks for uncommitted changes first; optionally runs
tests before review.
Uses DeepSeek Web (free) via Chrome browser — no API key needed.

**Parameters**: `summary` (required), `test_command` (optional), `since` (optional, default `-1` — review the last commit; `-2` for the last 2 commits, `-3` for the last 3, etc., i.e. the last N commits), `timeout` (optional).

Timeout: tool-level budget 30 min; `timeout` (seconds) optionally lowers it
for the expert phase. The review runs in one browser session; large commits
take longer to generate, so prefer a longer budget for large projects.

Use before pushing code or to learn better practices.

**Context**: every review input is uploaded as an ATTACHMENT: the rendered
review guide (review-guide.md), the complete diff (changes.patch), the full
content of each changed file (attachment names encode repo paths:
`internal__lp__x.go` = `internal/lp/x.go`; a name whose extension the upload
site rejects is uploaded with `.txt` appended, content unchanged - for example
`.gitignore` arrives as `.gitignore.txt`), AGENTS.md when present, and the
gocyclo report (gocyclo.txt, threshold 20) for the changed Go files. The first
message carries the commit background, the full commit message(s) and a
coverage note listing anything NOT attached. The expert has no execution
tools; its review is limited to the request and the attachments.
