# dscli — AI-Powered Developer Toolbox

```text
     o
    /|\
     |   +---------------+
    / \  | dscli tools   |
 ~~~~~~~~~| AI assistant  |
dscli    +---------------+
```

## 🎯 What is dscli?

**dscli** is an intelligent CLI tool powered by the DeepSeek API, combining an AI coding assistant, code analysis, and project management in one.

1. **AI Coding Assistant** — Deep integration with DeepSeek, supporting tool-calling multi-turn conversations
2. **Developer Toolbox** — File operations, code search, Git management, static analysis, Shell execution
3. **Session Memory** — Project-level conversation history, note system, cross-session recall
4. **Customizable** — Custom system prompts, skill system, multi-format output

Simply put: **dscli = AI assistant + dev tools + session memory + CLI efficiency**

## 📦 Version Information

### Version History

- v0.9.2 (2026-08-27) — **Web Chat tool calling & QA**: webchat executes DSML tool calls from DeepSeek Web replies (`--role` dev/expert/review/test, `--mode` renamed to `--model`); over-long inputs truncated by runes before send; one shared Chrome for all sends; auto-resend on failed sends with reload-based answer extraction; `keep` resume for interrupted sessions; `apply_patch` tool (git apply semantics, symlink-cwd escape blocked, repo-root scoped) wired into the webchat DSML loop; DSML parser hardened against LLM markup artifacts (fences, escapes, badge junk, slash-less/cut close tags); toolcall executor refactored to reuse `HandleToolCalls` core with opaque `<tool_result>`; new `quality_assurance` tool for release-gate checks (keep-resume + honored `timeout`); code_review sends commit message + diff only (expert reads files via tools), 30-min default timeout; SSE parsing hardening; ChatResponse ID persisted as conversation ID
- v0.9.1 (2026-08-23) — Vision & Files: DeepSeek Files API client with local upload cache (`dscli file upload|list|info|delete`), vision file tools (vision_file_read/list/info/delete) with dual-message image injection, `chat --attach`/`--model` for image input; time-aware pricing (2026-08-17 peak/off-peak, weekends off-peak, daily cache), `dscli models` shows current token prices; ask_expert `raw` mode + overload/truncation retries + temp-dir sandbox; file tools `insert_before_line` + length-mismatch warning + large-file size hint; interrupted tool calls marked on SIGINT/SIGTERM instead of replayed; macOS test-build regression fixes
- v0.9.0 (2026-08-05) — `web_fetch` one-shot tool replacing MCP web reading (meta-refresh following, output save/insert); webchat flash/vision modes + file uploads + conversation registry (`keep=<id|last|url|list>`); ask_expert `@file` input, role/system customization, function definitions kept when truncating; drop LightPanda MCP support and `mcp_client` tool; tool re-categorization (`check`/`ai`); wakeup/ainap/aistatus consolidation into `internal/toolcall/ai`
- v0.8.9 (2026-08-02) — Emacs integration (emacsclient-aware editor & wakeup), wakeup shell injection fix, MCP lenient schema support, Unicode-aware mail recipient lookup, history keyset pagination (`--before-id`), `project list --json`, site-zine skill, CI workflow
- v0.8.8 (2026-07-04) — Distributed tracing (Jaeger/clogs), cross-project AI communication (wakeup), project management commands, history move, session cleanup with active session protection, MCP integration framework (mcphub), gzip release archives, SQL lock leak fix, write_file CAS safety
- v0.8.7 (2026-06-13) — LightPanda Cloud MCP support, test role (QA Engineer), code fence language preservation, cloud token validation
- v0.8.0 (2026-05-17) — AI personality system (32 scientists), skill author auto-fill, unified output format, `git author` style user display
- v0.7.6 (2026-05-03) — P0 nil panic fix, type alias cleanup, recall limits, 11 new tests
- v0.7.5 (2026-05-03) — Toolcall result truncation threshold raised to 1M context
- v0.7.4 (2026-04-29) — Package restructuring, integrated prompt/note/session
- v0.7.3 (2026-04-15) — Recall tool supports keyword search in history
- v0.7.2 (2026-04-10) — Note tool supports cross-session memory
- v0.7.1 (2026-03-16) — Test refactoring, performance improved from 27s to 6s (4.2x)
- v0.7.0 (2026-03-16) — Integrated auto code formatting toolchain, refactored shell command logic, added timeout control
- v0.6.0 (2026-03-13) — Merged vimscript branch, added vimscript language support, optimized web reader
- v0.5.5 (2026-03-12) — Fixed issues from modernize tool, code structure optimization
- v0.5.4 (2026-03-09) — Added AskExpert function, improved AI assistant interaction
- v0.5.2 (2026-03-08) — Code restructuring, separation of concerns
- v0.5.0 (2026-02-28) — Feature-complete release, 43 iterations
- v0.4.0 — Format system refactoring, multiple output modes
- v0.3.0 — Git issue management
- v0.2.0 — Enhanced AI tool calling
- v0.1.0 — Initial release

## ✨ Core Features

### 🤖 AI Chat

- **`dscli chat`** — Multi-turn conversation with DeepSeek AI, supports tool calls (file I/O, code search, Git operations, etc.)
- **`dscli fim`** — Code completion (Fill-in-the-Middle), boost coding efficiency
- **`dscli models`** — List AI models with current token prices
- **`dscli balance`** — Check API balance and usage
- **`dscli chat --attach <img>`** — Image input with vision models (e.g. `deepseek-v4-flash-vision-exp`), uploaded via the DeepSeek Files API
- **`dscli webchat`** — Free chat through Chrome with chat.deepseek.com: `--model` pro/flash/vision, `--role` dev/expert/review/test/architect personas, `--keep` resumes saved conversations; DSML tool calls inside replies are executed locally (file ops, shell, code_review…) with per-round output

### 🖼️ Vision & Files

- **`dscli file`** — Manage DeepSeek Files API files (upload / list / info / delete) with a local content cache (`~/.dscli/files.json`): identical content reuses the same `file_id` with zero network requests
- **Vision model support** — `dscli chat --model deepseek-v4-flash-vision-exp --attach screenshot.png "图中有什么？"`
- **Vision file tools** — Models can read/upload images themselves: `vision_file_read` (injects the image into the conversation in the same round), `vision_file_list` / `vision_file_info` / `vision_file_delete`

### 📝 Session Management

- **`dscli history`** — Conversation history management (list / load / show / edit / update / move)
- **`dscli history move <project_id>`** — Transfer messages between projects
- **`dscli history recall <keywords>`** — Search conversation history, recall past discussions
- **`dscli project`** — Project management (list / assign / update / remove)

### 🛠️ Developer Tools

- **`dscli flycheck <path>`** — Static code analysis (Go with staticcheck, Python with ruff)
- **`dscli skill`** — Skill management (list / show / add / remove / query / validate / set-auto-inject / save; with YAML frontmatter author auto-fill)
- **`dscli prompt`** — System prompt management (show / edit, supports project-level and global)
- **`dscli completion`** — Generate shell completion scripts (bash / zsh / fish / powershell)
- **`dscli config edit`** — Edit configuration file

### 🎨 General Features

- **Multi-format output** — Markdown by default, `--org` for Org mode output
- **Database support** — SQLite for conversation history, configuration, notes, etc.
- **Project awareness** — Automatically detects Git repository root, isolates conversation history per project
- **Session statistics** — Shows elapsed time, cost, and balance after each conversation
- **`dscli version`** — Display version and runtime information

### 🎭 AI Personas

32 scientist personalities assigned randomly, each with unique character traits and email.

- **Random assignment** — Randomly drawn on first use, persistently bound
- **Persona injection** — Character descriptions automatically injected into system prompts
### 🔍 Distributed Tracing

dscli integrates with [Jaeger](https://www.jaegertracing.io/) (via [clogs](https://github.com/nanjj/clog)) for distributed tracing across all layers — top-level commands, toolcall handlers, shell execution, LightPanda web interactions, and DeepSeek API calls.

- **Automatic span injection** — Every command and tool call creates a trace span with parent-child relationships
- **W3C tracecontext propagation** — Trace context (`traceparent`) is propagated across process boundaries (MCP, subprocesses) for end-to-end tracing
- **Environment variables:**
  - `JAEGER_DISABLED=true` — Disable tracing entirely
  - Default Jaeger agent endpoint: `localhost:6831` (UDP)
- **No additional configuration needed** — Traces are sent automatically when a Jaeger agent is available

## 🚀 Quick Start

### Installation


```bash
# Option 1: go install (recommended)
go install github.com/dscli/dscli@latest

# Option 2: Build from source
git clone https://github.com/dscli/dscli.git
cd dscli
git checkout v0.9.2
make install    # installs to $GOPATH/bin

# Option 3: Download pre-built binary
# Check the Releases page for the latest version
```

### Configuration

1. Get a DeepSeek API key: [DeepSeek Platform](https://platform.deepseek.com/)
2. Set the environment variable:

```bash
export DEEPSEEK_API_KEY="your-api-key-here"
```

## 📖 Usage Examples

### 1. AI Coding Assistant

```bash
# Basic conversation (Markdown output)
echo "How to implement an HTTP server in Go?" | dscli chat

# Org mode output
echo "Explain the time complexity of this algorithm" | dscli chat --org

# Code completion
echo "def fibonacci(n):" | dscli fim

# Image input with a vision model (uploads via Files API)
dscli chat --model deepseek-v4-flash-vision-exp --attach screenshot.png "图中有什么？"
```

### 2. Session Management

```bash
# List conversation history
dscli history list

# Search history messages
dscli history recall "Go error handling"

# Edit message content
dscli history edit 42

# Move messages to another project
dscli history move 7

```

### 3. Skill Management

```bash
# List all skills
dscli skill list

# Search skills
dscli skill query "go fix"

# View skill details
dscli skill show go-fix

# Validate a skill
dscli skill validate go-fix

# Install skills
dscli skill add ~/src/agent-skills/skills/go-fix
dscli skill add ~/src/agent-skills/skills/go-fix --target=global

# Remove a skill
dscli skill remove go-fix

# Set auto-inject
dscli skill set-auto-inject go-fix true

# Create/update a skill (author auto-filled from git config)
dscli skill save --name my-skill --content "..." --desc "description"
```

### 4. Memory Management

```bash
# List memories for the current project
dscli memory list

# Search memories
dscli memory search "flycheck timeout"

# View full memory content
dscli memory show 1

# Memory statistics
dscli memory stats
```

### 5. Project Management

```bash
# List all projects (current project marked with →)
dscli project list

# Remove a project by ID or path
dscli project remove 6
dscli project remove /home/user/tmp

# Assign a maintainer to a project
dscli project assign 7 30

# Update project path
dscli project update 2 /new/path/to/project
```


### 6. Role Customization

dscli has five built-in AI roles: **dev** (development assistant, full tools/skills),
**expert** (domain expert, no tools/skills), **review** (code review,
shell+file_read/no skills), **test** (QA engineer), **architect** (software
architect: clarifies requirements, designs architecture, and orchestrates the
pipeline via `code_dev` / `code_review` / `quality_assurance`). Each role has
independently configurable system prompts, available tools, and skill lists.

**Browse tools:**

```bash
# List all available tools (categorized)
dscli tool list

# Filter by category
dscli tool list --category file
```

**Manage prompts:**

```bash
# List all prompts
dscli prompt list

# View prompt content
dscli prompt show review

# Add a new prompt based on review
dscli prompt show review | dscli prompt add editor

# Edit a prompt
dscli prompt edit editor
```

**Configure roles:**

```bash
# View current role configuration
dscli role list
dscli role show dev

dscli role update review --skills "go-fix,gofumpt" \
    --tools "shell,file_read" --prompt editor

# Reset to defaults
dscli role reset review
```

### 7. Developer Tools

```bash
# Static code analysis
dscli flycheck internal/...

# Emacs flycheck (supports 119+ languages)
dscli flycheck --emacs internal/

# Parse file structure (for LLM editing)
dscli parse main.go
dscli parse main.go -l python
```

### 8. View Models and Balance

```bash
# List available models (with current token prices)
dscli models

# Check account balance
dscli balance

# JSON format output
dscli models --format json
dscli balance --format json
```

### 9. Vision & Files

```bash
# Upload a local file (prints file_id; content is cached by SHA-256 + size)
dscli file upload screenshot.png

# List / inspect / delete uploaded files
dscli file list
dscli file info file-api-xxxxxxxxxxxxxxxx
dscli file delete file-api-xxxxxxxxxxxxxxxx

# Ask a vision model about a local image
dscli chat --model deepseek-v4-flash-vision-exp --attach screenshot.png "图中有什么？"
```

### 10. Configuration File

The configuration file defaults to `~/.dscli/config.dscli`, auto-generated on first run via environment variables:

```bash
# Line-start comment
deepseek-api-key = sk-xxx          # Line-end comment
deepseek-base-url = https://api.deepseek.com
```

Format rules:

- One `key = value` per line
- `#` supports both line-start and line-end comments

Common configuration options:

| Key | Default | Description |
|-----|---------|-------------|
| `deepseek-api-key` | | API key |
| `context-window` | `1000000` | Context window size (tokens) |
| `max-tokens` | `393216` | Max output tokens per request |
| `user-balance` | `true` | Show balance consumption after chat |
| `deepseek-v4` | `true` | Enable V4 model |
| `files-cache-path` | `~/.dscli/files.json` | Local cache for Files API uploads (content-hash keyed) |
| `read-file-large-threshold` | `200` | Size (KB) above which a full read of a file appends a size hint |

## 🔄 Workflow

1. **Project awareness** — Automatically detects Git repository root, establishes project context
2. **System prompts** — Loads project/global/default three-tier prompts, injects skills and notes
3. **Context isolation** — Each project has independent sessions and conversation history
4. **Tool integration** — AI can directly manipulate files, search code, execute Git/Shell commands
5. **Session statistics** — Displays elapsed time and balance consumption after each conversation

## 🤝 Contributing

Contributions, bug reports, and feature requests are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for detailed guidelines.

Apache License 2.0

## 📞 Support

- Repository: [github.com/dscli/dscli](https://github.com/dscli/dscli)
- Issues: [Create an Issue](https://github.com/dscli/dscli/issues)

---

**dscli** — Smarter, more efficient CLI development!
