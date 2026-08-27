# quality_assurance

Quality assurance via the QA engineer.

Assess recent commit(s) for release readiness: the QA engineer establishes a
test baseline (go vet, go test), inspects the diff for edge cases and
regressions, and produces a structured quality report. Checks for uncommitted
changes first.

Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed.

**Parameters**: `summary` (required), `since` (optional, default `-1` —
assess last commit; use `-2` for last 2, `-3` for last 3, etc., equivalent to
`HEAD~N`), `timeout` (optional), `keep` (optional — resume a saved QA
conversation: pass the `conversation_id` from a previous result, e.g. when a
round was interrupted mid tool-call; the pending tool calls are executed
locally and their results fed back to the expert until it produces the final
report; `summary`/`since` are ignored in this mode).

Timeout: default 1200s (20 min). The QA engineer may run several tool-call
rounds (exec_command: go vet, go test, git inspection) before producing the
final report, so set `timeout` (seconds) longer for large projects with many
tests. If the input is too large for the DeepSeek chat box, the QA engineer
inspects the repo via tools — results are fed back automatically in the same
conversation.

Use before releasing code or when a quality gate is required.

**Context**: the first message carries only the release background, commit
message, and diff. The QA engineer has tool access (read_file, exec_command,
apply_patch) in the same conversation and reads AGENTS.md plus full file
contents on demand, keeping the request under the chat-box input limit. If a
diff still exceeds the limit, per-file sections are dropped smallest-first and
listed in the tool warning — the QA engineer can read those files via
read_file.
