---
name: fix-dup-comments
description: Find and fix duplicate consecutive comment lines in Go source files. Detects both single-line duplicates and multi-line block duplicates, even when separated by blank lines.
keywords:
- duplicate
- comment
- dup
- clean
- awk
- go
- block
---

# fix-dup-comments

Find and fix duplicate consecutive comment lines in Go source files.

## Problem

Go source files accumulate duplicate consecutive comment lines over time:

```go
// NewContext creates a new context.
// NewContext creates a new context.
func NewContext() { ... }
```

Or even multi-line blocks, possibly separated by blank lines:

```go
// restoreFuncVars saves and restores all function variables used by
// ensureLocalLightpanda. Use with defer.

// restoreFuncVars saves and restores all function variables used by
// ensureLocalLightpanda. Use with defer.
func restoreFuncVars(...) { ... }
```

These are noise — from careless copy-paste or merge. They pass compilation
and tests, so they persist.

## Detection

Run `scripts/find.sh`:

```bash
bash scripts/find.sh [dir]
```

It detects **two** categories:

1. **Single-line duplicate**: same comment on the immediately following line
   (within a comment block). E.g., `// foo` then `// foo` on the next line.

2. **Multi-line block duplicate**: an entire adjacent comment block repeated
   (possibly separated by blank lines, not by code). E.g., a two-line doc
   comment appears twice in a row.

Code-separated identical comments (in different functions) are intentionally
**not** reported — those are legitimate documentation.

## Fix

For each duplicate at `file:line`:

1. Delete one occurrence via `write_file_with_line_range`:
   - `start_line=N, end_line=N, content=""` — deletes line N
   - For multi-line blocks, delete each line of the duplicate block.

2. **Important**: after each deletion, line numbers shift. Process from
   **bottom to top** (highest line number first) to keep remaining line
   numbers stable.

## Verification

After all fixes:

```bash
go build ./... && go test ./... && gofumpt -l . && bash scripts/find.sh
```

Last command should produce no output (clean).

## Scripts

- `scripts/find.sh` — detect single-line and multi-line block duplicates
