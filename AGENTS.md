# AGENTS.md

This is **dscli**, an AI-enhanced CLI tool for developers - DeepSeek API chat client with tool calling, project management, and a pluggable skills system. Module path: `github.com/dscli/dscli` (requires Go 1.26+, see `go.mod`).

## Build, Test, and Lint

```bash
make build                        # Build - outputs build/dscli
make install                      # Install to $GOPATH/bin; verifies embedded commit vs HEAD
make release                      # Cross-compile + gzip (linux/darwin amd64+arm64, windows amd64)
go test ./...                     # All unit tests
go test -v -run '^TestX$' ./...   # Single test (anchor with ^ and $ to avoid partial matches)
make dev-test                     # Fast test - skips formatting, use during development
make gofmt                        # Format with goimports + gofumpt
make fmt-check                    # Check formatting without modifying
make test-coverage                # Coverage: coverage.out / coverage.html / test-output.txt
```

**Which test command to use:**
- `make dev-test` — during development: runs `go test -v ./...`, skips formatting
- `go test ./...` — before committing: CI-equivalent, no verbose output
- `go test -v -run '^TestX$' ./...` — single test: use `^` and `$` to avoid matching `TestXyz`

**Before committing, ensure tests pass:**
```bash
go test ./...
make fmt-check
```

**Before pushing, run code review:**
```bash
# code_review is token-free - use it before every push
# Recommended: code_review(summary="<describe the change>")
```
- `code_review` before `git push` — fix issues before they reach remote
- No need to do it during development; only before push

**⚠️ Embedded assets are compile-time snapshots**: scripts and templates are
embedded via `go:embed` (see below). Editing them does NOT affect an installed
binary — you must rebuild and reinstall. `make install` checks the installed
binary's embedded commit against HEAD and warns on mismatch.

## Architecture

Entry point: `main.go` → `RootExecute()` → `root.go` (Cobra root command with
persistent flags: `--org`, `--no-color`, `--no-timestamp`, `--verbose`).

Top-level `*_cmd.go` files are CLI command implementations registered via
`AddRootCommand()` in their `init()` functions.

Packages use `init()` + `sqlite.Register*Schema` for declarative dependency
wiring — `sqlite.OpenDB()` executes all registered DDL on first open (sync.Once).
Four registrars, executed in this order:

1. `RegisterTableSchema` — CREATE TABLE / CREATE VIRTUAL TABLE (fatal on error)
2. `RegisterIndexSchema` — CREATE INDEX (fatal on error)
3. `RegisterUpgradeSchema` — ALTER TABLE / migration SQL (best-effort)
4. `RegisterPostInitHook` — `func(*DB) error` callbacks (best-effort)

Tests get an isolated database: `context.IsTesting()` → `/tmp/dscli-test-<binary>-<pid>.db`.

### Key Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/prompt/` | System prompts (roles: dev/expert/review/test), message persistence, history, recall/note, content blocks (image input), interrupt marking |
| `internal/toolcall/` | Tool registration (aliases), execution, JSON fix, result truncation, dual-message protocol, interrupt handling |
| `internal/toolcall/alltools/` | Blank-imports all tool packages; `GetAllTools(ctx)` |
| `internal/config/` | Config parsing (`~/.dscli/config.dscli` / `~/.dscli/dscli.env`) |
| `internal/session/` | Session management with per-project SQLite isolation |
| `internal/skills/` | Skill lifecycle: search, load, validate, auto-inject |
| `internal/context/` | Extends stdlib `context` with typed KV keys, project root, param bus |
| `internal/dsc/` | DeepSeek API client (chat, balance, models) + Files API (upload/list/info/delete with local content cache) |
| `internal/price/` | Token usage tracking & cost calculation; time-aware pricing (peak/off-peak after 2026-08-17, daily cache in ~/.dscli/price.json) |
| `internal/flycheck/` | Static analysis (Go, Python, Emacs) via embedded `dscli-flycheck.sh` |
| `internal/toolcall/vision/` | Files API vision tools: vision_file_read/list/info/delete (category: vision) |
| `internal/toolcall/ai/` | AI-conversation tools: wakeup, ainap, aistatus (category: ai) |
| `internal/emacsutil/` | Emacs client/server detection (emacsclient probe) |
| `internal/outfmt/` | Output formatting (markdown/org), color, timestamp |
| `internal/sqlite/` | Declarative, lazy SQLite connection, WAL mode, migration |
| `internal/mail/` | Inter-AI mail system (SQLite + FTS5) |
| `internal/ainame/` | 32 scientist persona assignment (bird/frog, from Dyson's essay) |
| `internal/roles/` | Role → skills/tools/prompt mapping (`role_configs` table) |
| `internal/chimein/` | Concurrent chat message injection |
| `internal/lockfile/` | Per-project process lock for chat sessions |
| `internal/editor/` | External editor integration (emacsclient-aware) |
| `internal/shell/` | Safe shell execution via mvdan/sh |
| `internal/lp/` | Web page reading via `lightpanda fetch` CLI, DeepSeek web login/chat (chromedp) with overload/truncation detection and conversation registry; `HandleWebChat` is the high-level entry point (retry + DSML tool loop) shared by ask_expert and the webchat CLI |
| `internal/mcphub/` | Multi-MCP-server connections; dispatches unknown tools |
| `internal/memories/` | Persistent cross-session memory with FTS5 |
| `internal/tokenizer/` | Chinese+English segmentation for FTS5 (gse) |
| `internal/gse/` | Chinese text segmentation (embedded dictionary) |
| `internal/userservice/` | OS user services (systemd --user / launchctl / pidfile) |
| `internal/processutil/` | Cross-platform process utilities (Windows support) |
| `internal/version/` | Single source of truth for version string (ldflags) |

### Tool Framework

Tools register via `toolcall.RegisterTool(ToolDef{...})` in package `init()`s;
`internal/toolcall/alltools` blank-imports every tool package. Chat loads them
through `alltools.GetAllTools(ctx)` (role-filtered - non-dev roles return fewer
tools). Tool categories: `file_ops`, `system` (cwd/shell/sql),
`communication` (ask), `check` (code_review/flycheck), `history`, `mail`,
`memory`, `skill`, `ai` (wakeup/ainap/aistatus), `vision` (Files API), `web`.
`ToolDef.Aliases` maps legacy names (e.g. `vision_file_upload` ->
`vision_file_read`) without exposing them in the tool list. Tools not in the
registry dispatch to MCP servers via `toolcall.DispatchMCP` (set by
`mcphub`).

Handler results may be a `DualMessage` (internal/toolcall/dual.go):
`HandleToolCalls` splits it into a tool message plus an extra user message
appended after ALL tool messages - the only path that can inject an image
block right after a tool call (OpenAI-compatible APIs allow images only in
user messages).

DSML tool calls from WebChat: chat.deepseek.com replies (role-driven
consultations like `review` via code_review, and plain chat alike) may embed
DSML markup (`<invoke name="exec_command">` with `<parameter>` children) - it
is the web model's native tool protocol. `lp.HandleWebChat` judges each reply
with `toolcall.IsDSMLToolCallEnd` (the reply ENDS with a `</tool_calls>`
close tag; whatever prose precedes it is discarded with the round; a lone
close tag without the opening wrapper still qualifies, and a wrapper with no
parseable `<invoke>` executes nothing - quoted code and prose references that
merely cite an `<invoke>` example never end with the wrapper close tag and
never qualify), parses them (internal/toolcall/dsml.go), maps `exec_command`
-> `shell` (cmd->script, justification->summary, timeout ms->s) and feeds
results back into the SAME conversation (handleWebChatToolLoop). Which tools
are executable is decided by the role's tools config (role_configs /
roles.DefaultFor) - the SAME source that gates GetAllTools, so `dscli role
update --tools` is the single place that decides it; there is NO separate
DSML whitelist. Registered tools without a hand-written DSML entry are
documented straight from their ToolDef (dsmlGeneratedDocEntry); only
exec_command keeps DSML-layer naming (cmd/justification/timeout-ms). The
loop prints every round it receives (reasoning + content via
outfmt.PrintContent, with the output token count derived from the site's
IndexedDB accumulated_token_usage) and marks the final result `Printed` so
callers do not re-print it. The `webchat` CLI defaults to `--role ""` (plain
chat: no role injection; DSML tool-call replies are still executed - default
dev profile, i.e. all tools); a stderr warning fires whenever the tool loop
actually runs (any mode), and role sessions additionally get a role-specific
warning up front, since the remote model's DSML tool calls will run
locally. Role templates (internal/prompt/*.md) render a DSML tool section
for WebChat via a `{{if .DSMLToolDoc.Intro}}` block: the section content
(`toolcall.BuildDSMLToolDoc`, see internal/toolcall/dsml_doc.go) is derived
from the role's tool config (roles.DefaultFor + role_configs), formatting
aligned with DeepSeek V4's tool template (string= attribute rules,
`### Available Tool Schemas`). `HandleWebChat` injects it via
`prompt.RenderPromptForRoleWithTools`; `RenderPromptForRole` and
`GetSystemPrompt` (dscli chat path, which registers tools through the API
`tools` parameter instead) leave it out entirely. A role without executable
tools (expert/review/test by default) gets no DSML section at all.

code_review sends ONLY commit message + diff on its first message: the review
expert reads AGENTS.md and full changed-file contents on demand via the DSML
tool loop (`read_file` / `exec_command`). Do not re-inject file contents or
AGENTS.md into the request — the web-chat input budget (140k runes, see code_review.go) is better spent on
the diff, and the expert can deep-read any file it needs (see
internal/prompt/review.md).

### Embedded Assets (`go:embed`)

| Asset | Location |
|-------|----------|
| `dscli-flycheck.sh` | `internal/flycheck/` — Emacs syntax check runner |
| Prompt templates | `internal/prompt/{dev,expert,review,test}.md` |
| Tool docs | `internal/toolcall/*/*.md` (one per tool) |
| Skill docs | `internal/skills/*.md` |
| Dictionaries | `internal/tokenizer/stopwords/*.txt`, `internal/gse/data/` |

**Editing any of these requires rebuild + reinstall.**

## Command Structure

Every CLI command follows the same pattern in a `*_cmd.go` file at the project root:

```go
func init() {
	cmd := AddRootCommand(&cobra.Command{
		Use:   "subcommand <required> [optional]",
		Short: "brief description",
		RunE:  subcommandRunE,
	})
	cmd.Flags().String("flag", "default", "description")
}
```

### Cobra `Use` Convention (see `cobra-use-convention` skill)

|Writing         |Meaning              |
|----------------|---------------------|
|`arg` or `<arg>`|**Required** argument|
|`[arg]`         |**Optional** argument|

**Key rule**: match the `Use` field with your `Args` validator (`cobra.ExactArgs`, `MinimumNArgs`, etc.). Don't blindly copy patterns from existing commands - they may be wrong.

### The Chat Command

The `chat` command (`chat.go`) is the core of dscli. Its flow:

1. `ChatPreRunE` - validate model (`--model` overrides the default), load role, set context values
2. `ChatRunE` - acquire project lock; if primary, start chat loop; if secondary, inject as chimein. `--attach` uploads images to the Files API (vision models only, hard error otherwise)
3. `ChatRound` - assemble messages (prompts → history → inputs), call DeepSeek API, handle tool calls recursively
4. `readChimein` - check for pending chimein/unread mail between rounds (after tool calls, before recursion)
5. Image handling - user attachments become `ContentBlock`s (text + file blocks, serialized to `content_blocks` column); `cleanMessagesForModel` strips image blocks for non-vision models to avoid API 400
6. Interrupt safety - on SIGINT/SIGTERM `MarkInterruptedToolCalls` inserts placeholder tool messages for unexecuted calls, so the next start does not replay user-cancelled operations

### System Prompt Pipeline

`LoadPrompts()` assembles the final system prompt:
```
embedded template ({role}.md) → project override (.dscli/prompt/) → global override (~/.dscli/prompt/)
    ↓
+ skill prompt (BuildSkillPrompt, role-dependent)
    ↓
+ note prompt (BuildNotePrompt, recent conversation clues)
    ↓
+ unread mail notification
    ↓
+ persona (ainame: NameEN / PersonalityEN / DescEN)
```
Role name == template file name: `dev` (default), `expert`, `review`, `test`.

## Testing

### Patterns
- Table-driven tests with `t.Run()` for multiple scenarios
- Use `t.Context()` for context (Go 1.24+, project requires 1.26)
- Use `t.TempDir()` for temporary directories
- Standard `testing` package: `t.Fatal` for setup errors, `t.Error`/`t.Errorf` for assertions
- See `go-test` skill: scripts `run.sh`, `lint.sh`, config isolation scaffold
- **Isolate ambient state** - tests must not touch `~/.dscli/files.json`,
  `~/.dscli/price.json`, or real `DEEPSEEK_*` env vars: inject
  `files-cache-path` (or `WithCachePath`), override `price.cachePath`/
  `fetchPage`, and call `sanitizeDeepSeekEnv` before building a Config.

### Test Files
Tests live alongside their code:
- `chat.go` → `chat_test.go`
- `history.go` → `history_test.go`
- `prompt.go` → `prompt_test.go`
- `project_cmd.go` → `project_cmd_test.go`
- `role_cmd.go` → `role_cmd_test.go`
- `internal/prompt/prompt.go` → `internal/prompt/prompt_test.go`
- `internal/toolcall/tool.go` → `internal/toolcall/tool_test.go`

## Code Style

- **Godoc comments** on all exported functions, types, and constants
- **gofumpt -extra** before commit (`make gofmt`)
- **Prefer simplicity** - avoid unnecessary abstraction
- **Modern Go** - use features from Go 1.22+ (see `use-modern-go` skill; go.mod requires 1.26.4)
- **No em dashes** - use regular dashes in code and comments
- **Comment the *why***, not the *what* - don't restate obvious code

## Commit Convention

- **English only** — commit messages must be in English. This project lives at `github.com/dscli/dscli`; developers worldwide should understand the history. Never use Chinese or other languages in commit messages.
- **Conventional Commits** preferred: `type(scope): description` (e.g. `feat(chat): add streaming`, `fix(lp): handle nil context`)
- **Imperative mood**, first line ≤72 chars

## Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain
- Use `errors.Is`/`errors.As` for sentinel error checks (not `==` comparison)
- Always check `rows.Err()` after database iteration
- Use `require.NoError(t, err)` in tests for immediate halt on failures

## Shell Scripts

- `internal/flycheck/dscli-flycheck.sh` — embedded Emacs flycheck runner:
  1. `emacsclient --eval '(server-running-p)'` when an Emacs server is running (probe is the proof — a failed connect exits 1)
  2. Fallback: `emacs --batch -q` + `dscli-flycheck.el` (found via `DSCLI_EL_ROOT` or an upward directory walk)
  - **Never pass `-a ""` to emacsclient**: it auto-starts a daemon, turning the probe into a side effect that always succeeds
- Skill scripts live in `.dscli/skills/<name>/scripts/` (e.g. `go-test/scripts/run.sh`)

## Skills System

Skills are reusable recipes in `.dscli/skills/<name>/SKILL.md`, registered in `.dscli/skills/skills.yaml`:
- Discoverable via `skill_search`/`dscli skill query`
- Loadable on demand via `skill_by_name`
- Auto-injectable per-role via `skill_set_auto_inject`

Key skills for development:
- `cobra-use-convention` - Cobra Use field conventions
- `use-modern-go` - Modern Go syntax (1.22–1.26)
- `go-test` - Go testing best practices + scripts
- `gofumpt` - Strict Go formatter rules
- `go-fix` - Go code modernization (analyzer-based)
- `go-doc-comments` - Go doc comment conventions
- `version-bump` - Version bump + git tag automation
- `fix-context-import` - Fix dual context import issues
- `fix-dup-comments` - Remove duplicate comment lines
- `pkgsite-api` - Query pkg.go.dev API
- `gh` - GitHub CLI patterns
- `issue-pr-response` - Issue/PR response best practices
- `emacs-client` - Query a running Emacs via emacsclient
- `jaeger-query` - Query Jaeger traces (via slingshot)
- `incus` - Incus container lifecycle for test environments
- `dscli` - dscli core concepts (prompt, history, skills, memory, mail)

## Key Invariants

- **Tool-call pairing** - `internal/prompt/history.go` pairs assistant `tool_calls`
  with `tool` messages by count and ID (`CleanupReverse`); on any mismatch the
  whole block is dropped on history reload. When adding placeholder `tool`
  messages (e.g. interrupt handling), never trim `tool_calls` - keep the full
  list and matching `ToolCallID`s.
- **History changes** - verify through the reload path (`LoadHistory`), not just
  raw DB rows: `CleanupReverse` runs at load time and can silently drop blocks
  that look correct in the table.
- **Image blocks are user-message-only** - the OpenAI-compatible protocol
  rejects images in assistant/system messages. The dual-message protocol
  (`internal/toolcall/dual.go`) is the only post-tool-call injection path;
  never re-inject file blocks at the chat layer (`BuildUploadInjection` was
  removed because it double-injected).
- **Red CI != your change** - when CI fails on a branch, first check whether the
  failure pre-exists on `main` before debugging your branch.

## Development Workflow

- Work on a branch (fork for external PRs); rebase onto latest `main` before
  pushing - keep history linear, no merge commits.
- Address every review comment; reply with a per-point summary and request
  re-review in the PR thread.
- Never modify or delete `sqlite.db` or `dscli.env` - they hold local state and
  secrets.

## AI Assistant Context

AI assistants: your tool set and behavior contract are defined in `internal/prompt/`
templates (dev/expert/review/test). This AGENTS.md is the **project-specific
supplement**: read it before writing code to understand build commands,
architecture, and conventions unique to dscli.

Behavioral rules:
- **Check unread mail first** - at session start, reviews and decisions may be
  waiting; reply before starting new work.
- **Ask instead of guessing** - when a requirement is ambiguous, ask the user or
  an expert; never fabricate answers.
- **Verify before asserting** - check the actual code or run the test before
  claiming behavior in docs, comments, or review replies.
- **Record lessons** - when a review or test catches a subtle issue, `mem_save`
  the lesson (searchable), and if it must apply to every future session, add it
  to this file. This is how project rules grow.
