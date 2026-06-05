---
name: fix-dup-comments
description: Find and fix duplicate consecutive comment lines in Go source files. Detects single-line duplicates, multi-line block duplicates (even blank-separated), and double blank lines often left after removal.
author: Bohr <bohr@dscli.io>
keywords:
- duplicate
- comment
- dup
- clean
- awk
- go
- block
- blank-line
---

# fix-dup-comments

Find and fix duplicate consecutive comment lines in Go source files,
plus double blank lines often left behind after removing duplicates.

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

A secondary issue: **double blank lines**. When a duplicate comment block is
removed, the blank line that separated it from the original often remains,
creating two consecutive blank lines:

```go
// foo does something.
// foo does something.    ← deleted

func foo() { ... }
```

After deleting the duplicate, a stray blank line is left above `func`:

```go
// foo does something.

                         ← stray blank line
func foo() { ... }
```

## Detection

Run `scripts/find.sh`:

```bash
bash scripts/find.sh [dir]
```

It detects **three** categories:

1. **Single-line duplicate**: same comment on the immediately following line
   (within a comment block). E.g., `// foo` then `// foo` on the next line.

2. **Multi-line block duplicate**: an entire adjacent comment block repeated
   (possibly separated by blank lines, not by code). E.g., a two-line doc
   comment appears twice in a row.

3. **Double blank** (output suffix `:double-blank`): two consecutive blank
   lines. Often a side-effect of removing a duplicate block.

Code-separated identical comments (in different functions) are intentionally
**not** reported — those are legitimate documentation.

## Fix

### Duplicate comments

For each duplicate at `file:line`:

1. Delete one occurrence via `write_file_with_line_range`:
   - `start_line=N, end_line=N, content=""` — deletes line N
   - For multi-line blocks, delete each line of the duplicate block.

2. **Critical**: after each deletion, line numbers shift. Process from
   **bottom to top** (highest line number first). Better yet, **re-run
   `find.sh` after each fix** to get fresh line numbers — this eliminates
   the risk of targeting the wrong line after a shift.

### Double blank lines

For each `file:line:double-blank`:

1. Delete one blank line: `write_file_with_line_range(start_line=N, end_line=N, content="")`
2. Process bottom-to-top, re-running `find.sh` between fixes.

**Prefer fixing duplicates first, then re-run `find.sh` to catch any double
blanks left behind.** This two-pass approach is safer than trying to predict
which blanks will become doubles.

## Caveats

- **False positives in raw string literals**: `find.sh` works line-by-line and
  cannot distinguish code-level blank lines from those inside `` `raw strings` ``.
  Double-blank findings inside test expected-value strings are usually
  intentional and should be skipped. Always check context before deleting.

## Verification

After all fixes, run the full gauntlet:

```bash
go build ./... && go test ./... && gofumpt -l . && bash scripts/find.sh
```

The last command should produce no output (or only known false positives in
raw string literals — skip those).
If it reports `:double-blank` lines outside string literals, fix those too
(they're the easiest to miss — often a direct consequence of the duplicate
removal).
