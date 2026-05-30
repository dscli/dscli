# AGENTS.md

This is **dscli**, an AI-enhanced CLI tool for developers - DeepSeek API chat client with tool calling, project management, and a pluggable skills system. Module path: `gitcode.com/dscli/dscli`.

## Build, Test, and Lint

```bash
make build                   # Build - outputs build/dscli
make install                 # Install to $GOPATH/bin
go test ./...                # All unit tests
go test -v -run TestX ./...  # Single test
make dev-test                # Fast test (skip formatting)
make gofmt                   # Format with goimports + gofumpt
make fmt-check               # Check formatting without modifying
```

**Before committing, ensure tests pass:**
```bash
go test ./...
make fmt-check
```

## Architecture

Entry point: `main.go` → `RootExecute()` → `root.go` (Cobra root command with persistent flags).

Top-level `*_cmd.go` files are CLI command implementations registered via `AddRootCommand()` in their `init()` functions.

### Key Internal Packages

| Package | Purpose |
|---------|---------|
| `internal/prompt/` | System prompts (dev/expert/review templates), history, notes, recall |
| `internal/toolcall/` | Tool registration, execution, JSON fix, result truncation |
| `internal/toolcall/alltools/` | All tool definitions registered for AI use |
| `internal/config/` | Config file parsing (`~/.dscli/config.dscli`) |
| `internal/session/` | Session management with per-project SQLite isolation |
| `internal/skills/` | Skill lifecycle: search, load, validate, auto-inject |
| `internal/context/` | Context key-value store, project root detection |
| `internal/dsc/` | DeepSeek API client (chat, balance, models) |
| `internal/flycheck/` | Static analysis (Go, Python, Emacs) |
| `internal/outfmt/` | Output formatting (markdown/org), color, timestamp |
| `internal/sqlite/` | SQLite database access with FTS5 |
| `internal/mail/` | Inter-AI mail system |
| `internal/ainame/` | 32 scientist personality assignment |
| `internal/roles/` | Role configuration (tools, skills, prompt overrides) |
| `internal/chimein/` | Concurrent chat message injection |
| `internal/lockfile/` | Per-project process lock for chat sessions |
| `internal/editor/` | External editor integration |
| `internal/lp/` | Language parser (C, Python, JSON) |
| `internal/memories/` | Persistent cross-session memory store |
| `internal/tokenizer/` | Token counting for context window management |
| `internal/userservice/` | User identity service |
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

| Writing | Meaning |
|---------|---------|
| `arg` or `<arg>` | **Required** argument |
| `[arg]` | **Optional** argument |

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

## AI Assistant Tools

AI assistants working on this project have access to standard file operations
(`read_file`, `write_file`, `search_content`), shell execution, code analysis,
and git management. See `internal/prompt/` for the full system prompt templates
that define the AI's tool set and behavior contract.
