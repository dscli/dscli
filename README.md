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
- **`dscli models`** — List available AI models
- **`dscli balance`** — Check API balance and usage

### 📝 Session Management

- **`dscli history`** — Conversation history management (list / load / show / edit / update)
- **`dscli history recall <keywords>`** — Search conversation history, recall past discussions

### 🛠️ Developer Tools

- **`dscli flycheck <path>`** — Static code analysis (Go with staticcheck, Python with ruff)
- **`dscli skill`** — Skill management (list / show / add / remove / query / validate / set-auto-inject / save; with YAML frontmatter author auto-fill)
- **`dscli prompt`** — System prompt management (show / edit, supports project-level and global)
- **`dscli completion`** — Generate shell completion scripts (bash / zsh / fish / powershell)
- **`dscli config edit`** — Edit configuration file

### 💬 WeChat Integration

- **`dscli wechat`** — WeChat AI tool interface (login, message send/receive, contact/group management)

### 🎨 General Features

- **Multi-format output** — Supports `--mode markdown` (default) and `--mode org`
- **Database support** — SQLite for conversation history, configuration, notes, etc.
- **Project awareness** — Automatically detects Git repository root, isolates conversation history per project
- **Session statistics** — Shows elapsed time, cost, and balance after each conversation
- **`dscli version`** — Display version and runtime information

### 🎭 AI Personas

32 scientist personalities assigned randomly, each with unique character traits and email.

- **Random assignment** — Randomly drawn on first use, persistently bound
- **Persona injection** — Character descriptions automatically injected into system prompts

## 🚀 Quick Start

### Installation

```bash
# Option 1: go install (recommended)
go install github.com/dscli/dscli@latest

# Option 2: Build from source
git clone https://github.com/dscli/dscli.git
cd dscli
git checkout v0.8.0
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
echo "Explain the time complexity of this algorithm" | dscli chat --mode org

# Code completion
echo "def fibonacci(n):" | dscli fim
```

### 2. Session Management

```bash
# List conversation history
dscli history list

# Search history messages
dscli history recall "Go error handling"

# View message details
dscli history show 42

# Edit message content
dscli history edit 42
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

### 5. Role Customization

dscli has three built-in AI roles: **dev** (development assistant, full tools/skills),
**expert** (domain expert, no tools/skills), **review** (code review,
shell+file_read/no skills). Each role has independently configurable system prompts,
available tools, and skill lists.

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

### 6. Developer Tools

```bash
# Static code analysis
dscli flycheck internal/...

# Emacs flycheck (supports 119+ languages)
dscli flycheck --emacs internal/

# Parse file structure (for LLM editing)
dscli parse main.go
dscli parse main.go -l python
```

### 7. View Models and Balance

```bash
# List available models
dscli models

# Check account balance
dscli balance

# JSON format output
dscli models --format json
dscli balance --format json
```

### 8. Configuration File

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
