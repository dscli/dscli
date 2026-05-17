---
name: fix-dup-comments
description: Find and fix duplicate consecutive comment lines in Go source files. Detection via awk one-liner, fix via write_file_with_line_range.
keywords:
- duplicate
- comment
- dup
- clean
- awk
- go
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

These are noise — same line repeated back-to-back, usually from careless
copy-paste or merge.  They pass compilation and tests, so they persist.

## Detection

One awk invocation finds them all:

```bash
find . -name '*.go' -not -path './.git/*' \
  -exec awk 'FNR==1{prev="";fn=FILENAME}
    /^[[:space:]]*\/\// && $0==prev {print fn":"FNR":"$0}
    {prev=$0}' {} +
```

**How it works**: for each file, track the previous line.  If current line
starts with `//` (possibly indented) and equals the previous line exactly,
print it.  Blank lines break the chain (prev is resetting on each new file
but blank lines within a file naturally separate logical groups).

## Fix

For each duplicate at `file:line`:

1. Delete one occurrence via `write_file_with_line_range`:
   - `start_line=N, end_line=N, content=""` — deletes line N

2. **Important**: after each deletion, line numbers shift.  Process from
   **bottom to top** (highest line number first) to keep remaining line
   numbers stable.

## Verification

After all fixes:

```bash
go build ./... && go test ./... && gofumpt -l . && \
  find . -name '*.go' -not -path './.git/*' \
    -exec awk 'FNR==1{prev="";fn=FILENAME}
      /^[[:space:]]*\/\//&&$0==prev{print fn":"FNR":"$0}{prev=$0}' {} +
```

Last command should produce no output (clean).

## Scripts

- `scripts/find.sh` — detect duplicate comments
