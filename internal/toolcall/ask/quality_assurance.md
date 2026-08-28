# quality_assurance

Quality assurance via the QA engineer.

Assess recent commit(s) for release readiness: the QA engineer establishes a
test baseline (go vet, go test), inspects the diff for edge cases and
regressions, and produces a structured quality report. Checks for uncommitted
changes first.

**Parameters**: `summary` (required), `since` (optional, default `-1` —
assess the last commit; `-2` for the last 2 commits, `-3` for the last 3,
etc., i.e. the last N commits), `timeout` (optional), `keep` (optional —
resume a saved QA conversation: pass the `conversation_id` from a previous
result, e.g. when a round was interrupted mid tool-call; the pending tool
calls are executed locally and their results fed back to the expert until it
produces the final report; `summary`/`since` are ignored in this mode).

Timeout: tool-level budget 30 min; `timeout` (seconds) optionally lowers it
for the expert phase. The QA engineer may run several tool-call
rounds (shell: go vet, go test, git inspection) before producing the
final report, so set `timeout` (seconds) longer for large projects with many
tests. If the input is too large for the DeepSeek chat box, the QA engineer
inspects the repo via tools — results are fed back automatically in the same
conversation.

Use before releasing code or when a quality gate is required.

**Context**: the first message carries only the release background, commit
message, and diff. The QA engineer reads AGENTS.md plus full file contents on
demand and runs go vet / go test via the DSML tool loop, provided the test
role has tools configured (role_configs / roles.DefaultFor: none by default;
enable with `dscli role update test --tools shell,read_file,apply_patch`).
Without them the report is limited to the diff itself. If a diff still
exceeds the limit, per-file sections are dropped smallest-first and listed
in the tool warning.
