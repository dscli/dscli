# code_dev

Implement a feature or fix via the developer role.

Hand a complete implementation task to the built-in `dev` assistant
(DeepSeek Web, free V4 Pro via Chrome — no API key needed), which works in
the project repo with the developer's full toolset (shell, read_file,
write_file, git, ...) through the DSML tool loop. It implements, runs the
project's tests, and commits the result.

**Parameters**: `task` (required — the implementation task; a value starting
with `@` reads the task from a file, e.g. `@docs/architecture.md`, safe
paths only, max 1MB), `keep` (optional — continue a previous developer
conversation: pass the `conversation_id` from a previous `code_dev` result
to send follow-up fix instructions to the SAME session, which keeps the
full project context), `timeout` (optional).

Timeout: tool-level budget 60 min; `timeout` (seconds) optionally lowers it
for the developer phase. Implementation plus several test rounds plus a
commit can take a while — budget generously.

**Developer contract** (enforced by the dev role prompt): implement →
run the project's test suite → commit all changes in English with a
descriptive message → report what was done, test outcomes, and the commit
hash. The working tree must be CLEAN when it returns, because `code_review`
requires a clean tree before it reviews the commit.

**Pipeline**: architect designs → `code_dev` implements → `code_review`
inspects → fixes flow back via `keep=<conversation_id>` → optionally
`quality_assurance` for release readiness.

**Note**: the developer session is synchronous — the tool returns only
after the developer finishes (multiple web-chat rounds, potentially tens of
minutes).
