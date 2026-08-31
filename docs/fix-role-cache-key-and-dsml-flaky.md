# Fix: role config cache key + dsml shuffle flaky test

Date: 2026-08
Status: approved by user

Two independent, low-risk fixes in the dscli repo (branch: main, all commits in English).

---

## Part 1 — dsml usage-count test is order-dependent (`internal/dsml`)

### Problem

`go test -shuffle=on ./...` makes `TestExecuteDSMLToolCallsToolResultFormat`
fail intermittently. QA verified the failure rate is comparable on the
pre-existing baseline commit (`e61fcf1`) and on HEAD: ~6/8 vs ~5/8 shuffle
runs. It is a pre-existing test isolation defect, not a product bug.

### Root cause

- `tools.usage_count` is a **global per-tool counter** (`internal/toolcall/tool.go:719`:
  `UPDATE tools SET usage_count = usage_count + 1 WHERE id = ?`), keyed by the
  tool row (`name TEXT NOT NULL UNIQUE`).
- All tests in package `internal/dsml` share one process and one test DB
  (`context.IsTesting()` → `/tmp/dscli-test-<binary>-<pid>.db`).
- Several tests in `dsml_test.go` register the **same name** `"shell"` and
  actually execute it:
  - `TestExecuteDSMLToolCallsToolResultFormat` (line ~692) — executes shell + read_file
  - `TestExecuteDSMLToolCallsRoleGate` (line ~757) — executes shell (and read_file)
  - `TestExecuteDSMLToolCallsLegacySpellingSkipped` (line ~804) — executes shell
- The test asserts **absolute** counts:
  ```go
  if got := countFor("shell"); got != 1 { ... }        // line 742
  if got := countFor("read_file"); got != 1 { ... }    // line 745
  if got := countFor("write_file"); got != 0 { ... }   // line 749
  ```
  Under default (alphabetic-ish) ordering this happens to pass; under
  `-shuffle` a test that executed `"shell"` first bumps the shared counter and
  the `== 1` assertion fails. There is no `t.Parallel` in these tests
  (verified), so tests in the package run serially.

### Fix (test-only, no product-code change)

In `TestExecuteDSMLToolCallsToolResultFormat` only:

- **Snapshot before**: before executing the DSML calls, read the baseline
  stats with `toolcall.GetToolUsageStats(ctx, 0)` and build
  `map[string]int` name → count (helper `usageBaseline(t, ctx)` or an inline
  closure).
- **Assert deltas after**: after `ExecuteDSMLToolCalls`, re-read stats and
  assert `delta("shell") == 1`, `delta("read_file") == 1`,
  `delta("write_file") == 0`.
- Keep the existing comment about "shell executed exactly once / unregistered
  write_file never executes" — update wording to reflect the delta semantics
  (this test measures *this test's* executions, not global totals).
- Do NOT weaken to `>= 1`; the delta basis is exact and keeps the strong
  "rejected call was never executed" guarantee.
- Do NOT change the tool names; the deep semantics (native registered name ==
  execution name) stay intact.

Note: `GetToolUsageStats(ctx, 0)` (days=0) returns all tools with their
`usage_count` — that is the data source already used by the test; just compute
before/after deltas.

---

## Part 2 — role config cache ignores session (`internal/roles`)

### Problem

`roleCache` is keyed only by role name:
```go
roleCache   map[string]*RoleConfig // role name → config, nil until loaded
```
but the DB table `role_configs` is keyed by `UNIQUE(role, session_id)`.
`GetRoleConfig(ctx, role, sessionID)` fast-path returns `roleCache[role]`
**without consulting `sessionID`**. Consequences:

1. In a multi-session process (webchat sessions, tests with isolated
   sessions), session B's lookup can return session A's row for the same role,
   or nil when A had no row — i.e. wrong or missing config.
2. This matters now more than ever: `prompt.GetSystemPrompt`, `chat.go`
   unread-mail notification, `prompt.ToolsSpec` and the DSML role gate all
   resolve through `roles.GetRoleConfig` (see `internal/prompt/prompt.go:362,
   492, 683`). A wrong session's config directly gates whether a role can
   read mail or which tools are offered.
3. Reproducible in tests: e.g. `internal/prompt` `TestRoleCanReadMailRowOverride`
   runs in an isolated session; a stale cache entry from another session
   would flip the result (this was a real shuffle failure during review).

### Fix (internal structure only; API unchanged)

In `internal/roles/role.go`:

- Change the cache type to session-bucketed:
  ```go
  // roleCache: sessionID → role name → config.
  // A bucket present means that session was loaded (negative results cached too).
  roleCache map[int64]map[string]*RoleConfig
  ```
- Fast path (RLock): if `roleCache != nil`, look up the bucket
  `roleCache[sessionID]`; if the bucket exists, return `bucket[role]` (nil is a
  valid cached negative) and unlock. If the bucket is missing, fall through to
  the slow path (do NOT return nil).
- Slow path (Lock + double-check): re-check bucket; then
  `configs, err := ListRoleConfigs(ctx, sessionID)`; on success build the
  bucket `m := make(map[string]*RoleConfig, len(configs))` with the existing
  `m[configs[i].Role] = &configs[i]` pattern, store it under
  `roleCache[sessionID]` (creating the outer map if nil), return `m[role]`
  (nil when the session has no row for that role).
- On `ListRoleConfigs` error keep the existing fallback: direct DB query
  (`SELECT ... WHERE role = ? AND session_id = ?`), no cache write —
  unchanged behavior.
- `invalidateRoleCache()` stays a full clear (set outer map to nil); call
  sites in Upsert/Delete are unchanged.
- Update the doc comment on `GetRoleConfig` and the var comment: the cache is
  loaded lazily per (session, role-bucket) and cleared by writes; remove any
  wording implying "loaded once per process lifetime" if it is now inaccurate
  (per-session buckets are loaded on demand).

### Tests (`internal/roles/role_test.go`)

1. `newTestDB`: add `invalidateRoleCache()` right after `sqlite.SetDBPath(...)`
   (same package, the helper is unexported). Without it, a previous test's
   cache would serve configs from the OLD db path — the cache is package-level
   and survives DB swap. Add a comment explaining why.

2. New `TestGetRoleConfigSessionIsolation` (table-driven, both orderings):
   - sessionA: `UpsertRoleConfig(ctx, "dev", sidA, nil, strPtr("shell"), nil)`
     → `GetRoleConfig(ctx, "dev", sidA)` returns config with Tools=="shell".
   - sessionB: no rows at all → `GetRoleConfig(ctx, "dev", sidB)` returns
     nil (and stays nil on a second call — negative caching within bucket).
   - ordering A-then-B and B-then-A both asserted (use two `t.Run` subtests or
     reset cache between subtests via `invalidateRoleCache()` so each subtest
     starts clean; simpler: run both orders as separate subtests each starting
     with `invalidateRoleCache()`).
   - Also assert sessionA's config is still returned after sessionB lookups
     (buckets don't overwrite each other).
   - Use `FindConfig`/`UpsertRoleConfig`/`DeleteRoleConfig` helpers already in
     the test file; keep the tri-state pointer style (`strPtr`).

---

## Acceptance criteria

- `go build ./...`, `go vet ./...`, `go test ./...` all green (40 packages).
- `go test -shuffle=on -count=3 ./internal/dsml/` green (repeat until confident;
  at least 3 shuffle runs).
- `go test -shuffle=on ./internal/roles/ ./internal/prompt/` green.
- `make fmt-check` clean; `make gofmt` applied (gofumpt strict).
- No API changes; no behavior change for single-session usage.
- Two commits (one per part) or one combined commit — prefer two logical
  commits:
  1. `fix(test): make dsml usage-count assertions shuffle-safe` (or similar)
  2. `fix(roles): scope role config cache per session` (or similar)
- Working tree CLEAN when done; summary must include test outputs and hashes.

## Constraints

- English-only commit messages (Conventional Commits).
- Do not touch product behavior of `RecordToolUsage`/`GetToolUsageStats`.
- Do not add new exported symbols to `internal/roles` (keep `GetRoleConfig`
  signature as-is).
- Comment the *why* in code (per AGENTS.md); no em dashes in comments.
- Prefer simplicity: no unnecessary abstractions, no new dependencies.
