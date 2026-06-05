# AGENTS.md

This is **dscli**, an AI-enhanced CLI tool for developers - DeepSeek API chat client with tool calling, project management, and a pluggable skills system. Module path: `github.com/dscli/dscli`.

## Build, Test, and Lint

```bash
make build                        # Build - outputs build/dscli
make install                      # Install to $GOPATH/bin
go test ./...                     # All unit tests
go test -v -run '^TestX$' ./...   # Single test (anchor with ^ and $ to avoid partial matches)
make dev-test                     # Fast test - skips formatting, use during development
make gofmt                        # Format with goimports + gofumpt
make fmt-check                    # Check formatting without modifying
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

## Architecture

Entry point: `main.go` → `RootExecute()` → `root.go` (Cobra root command with persistent flags).

Top-level `*_cmd.go` files are CLI command implementations registered via `AddRootCommand()` in their `init()` functions.

Packages use `init()` + `sqlite.Register*Schema` for declarative dependency wiring — `sqlite.OpenDB()` executes all registered DDL on first open.

### Key Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/prompt/` | System prompts (dev/expert/review templates), history, notes, recall |
| `internal/toolcall/` | Tool registration, execution, JSON fix, result truncation |
| `internal/toolcall/alltools/` | All tool definitions registered for AI use |
| `internal/config/` | Config file parsing (`~/.dscli/config.dscli`) |
| `internal/session/` | Session management with per-project SQLite isolation |
| `internal/skills/` | Skill lifecycle: search, load, validate, auto-inject |
| `internal/context/` | Extends stdlib `context` with typed KV keys, project root, param bus |
| `internal/dsc/` | DeepSeek API client (chat, balance, models) |
| `internal/price/` | Token usage tracking & cost calculation |
| `internal/flycheck/` | Static analysis (Go, Python, Emacs) |
| `internal/outfmt/` | Output formatting (markdown/org), color, timestamp |
| `internal/sqlite/` | SQLite connection, WAL mode, schema migration |
| `internal/mail/` | Inter-AI mail system |
| `internal/ainame/` | 32 scientist personality assignment |
| `internal/roles/` | Role configuration (tools, skills, prompt overrides) |
| `internal/chimein/` | Concurrent chat message injection |
| `internal/lockfile/` | Per-project process lock for chat sessions |
| `internal/editor/` | External editor integration |
| `internal/shell/` | Safe shell execution via mvdan/sh |
| `internal/lp/` | Language parser (C, Python, JSON) |
| `internal/parse/` | Code structure parsing (Go/Python), symbol extraction |
| `internal/memories/` | Persistent cross-session memory with FTS5 |
| `internal/gse/` | Chinese text segmentation (Go jieba) |
| `internal/tokenizer/` | Token counting for context window management |
| `internal/userservice/` | systemd --user dscli-<name>.service |
| `internal/wechat/` | WeChat integration |

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
4. `injectChimein` - check for pending chimein/unread mail between rounds

### System Prompt Pipeline

`LoadPrompts()` assembles the final system prompt:
```
embedded template (dev.md) → project override (.dscli/prompt/) → global override (~/.dscli/prompt/)
    ↓
+ skill prompt (BuildSkillPrompt, role-dependent)
    ↓
+ note prompt (BuildNotePrompt, recent conversation clues)
    ↓
+ unread mail notification
```

## Testing

### Patterns

- Table-driven tests with `t.Run()` for multiple scenarios
- Use `t.Context()` for context (Go 1.24+)
- Use `t.TempDir()` for temporary directories (Go 1.24+)
- Use standard `testing` package: `t.Fatal` for setup errors, `t.Error`/`t.Errorf` for assertions

### Test Files

Tests live alongside their code:
- `chat.go` → `chat_test.go`
- `internal/prompt/prompt.go` → `internal/prompt/prompt_test.go`
- `internal/toolcall/tool.go` → `internal/toolcall/tool_test.go`

## Code Style

- **Godoc comments** on all exported functions, types, and constants
- **gofumpt -extra** before commit (`make gofmt`)
- **Prefer simplicity** - avoid unnecessary abstraction
- **Modern Go** - use features from Go 1.22+ (see `use-modern-go` skill)
- **No em dashes** - use regular dashes in code and comments
- **Comment the *why***, not the *what* - don't restate obvious code

## Error Handling

- Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain
- Use `errors.Is`/`errors.As` for sentinel error checks (not `==` comparison)
- Always check `rows.Err()` after database iteration
- Use `require.NoError(t, err)` in tests for immediate halt on failures

## Skills System

Skills are reusable recipes in `.dscli/skills/<name>/SKILL.md`. They are:
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

## AI Assistant Context

AI assistants: your tool set and behavior contract are defined in `internal/prompt/` templates
(dev/expert/review). This AGENTS.md is the **project-specific supplement** — read it before
writing code to understand build commands, architecture, and conventions unique to dscli.
