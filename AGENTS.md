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
persistent flags: `--mode`, `--no-color`, `--no-timestamp`, `--verbose`).

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
| `internal/prompt/` | System prompts (roles: dev/expert/review/test), message persistence, history, recall/note |
| `internal/toolcall/` | Tool registration, execution, JSON fix, result truncation |
| `internal/toolcall/alltools/` | Blank-imports all tool packages; `GetAllTools(ctx)` |
| `internal/config/` | Config parsing (`~/.dscli/config.dscli` / `~/.dscli/dscli.env`) |
| `internal/session/` | Session management with per-project SQLite isolation |
| `internal/skills/` | Skill lifecycle: search, load, validate, auto-inject |
| `internal/context/` | Extends stdlib `context` with typed KV keys, project root, param bus |
| `internal/dsc/` | DeepSeek API client (chat, balance, models) |
| `internal/price/` | Token usage tracking & cost calculation |
| `internal/flycheck/` | Static analysis (Go, Python, Emacs) via embedded `dscli-flycheck.sh` |
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
| `internal/lp/` | Web page reading via LightPanda MCP (local stdio / cloud SSE) |
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
through `alltools.GetAllTools(ctx)` (role-filtered — non-dev roles return fewer
tools). Tool categories: `file`, `shell`, `cwd`, `ask`, `history`, `mail`,
`memory`, `skill`, `sql`, `web`, `flycheck`, `wakeup`, `ainap`, `aistatus`,
`system`. Tools not in the registry dispatch to MCP servers via
`toolcall.DispatchMCP` (set by `mcphub`).

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

1. `ChatPreRunE` - validate model, load role, set context values
2. `ChatRunE` - acquire project lock; if primary, start chat loop; if secondary, inject as chimein
3. `ChatRound` - assemble messages (prompts → history → inputs), call DeepSeek API, handle tool calls recursively
4. `readChimein` - check for pending chimein/unread mail between rounds (after tool calls, before recursion)

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

## AI Assistant Context

AI assistants: your tool set and behavior contract are defined in `internal/prompt/`
templates (dev/expert/review/test). This AGENTS.md is the **project-specific
supplement** — read it before writing code to understand build commands,
architecture, and conventions unique to dscli.
