---
name: fix-context-import
description: Fix files that import BOTH stdlib "context" AND internal/context — the tricky dual-import case requiring alias cleanup. Fully automated with dry-run.
keywords:
- context
- import
- fix
- dual-import
- alias
- internal/context
---

# fix-context-import

## Purpose

Fix files that import **both** stdlib `"context"` and `internal/context` (aliased).
These are the tricky cases: the alias must be removed and all `alias.XXX` calls
replaced with `context.XXX`.

## When to Use

When a file accidentally imports both stdlib `"context"` and the project's
`internal/context` (usually with an alias like `dsctx` or `pctx`).

## Script

```bash
# Dry-run (report only, no changes)
python3 ~/.dscli/skills/fix-context-import/scripts/fix_context_import.py --dry-run

# Fix all dual-import violations
python3 ~/.dscli/skills/fix-context-import/scripts/fix_context_import.py

# Fix specific files
python3 ~/.dscli/skills/fix-context-import/scripts/fix_context_import.py internal/flycheck/checkpath.go
```

## What It Does

1. Scans Go files for those importing BOTH `"context"` (stdlib) and `internal/context` (aliased)
2. Removes the stdlib `"context"` import line
3. Removes the alias from `internal/context` (e.g. `dsctx "github.com/dscli/dscli"`)
4. Replaces all `alias.XXX` calls with `context.XXX`
5. Runs `gofumpt -w` on modified files

## Scripts

- `scripts/fix_context_import.py`: Main fix script
