---
name: go-doc-comments
description: Go doc comment conventions from https://go.dev/doc/comment — headings, paragraphs, links, lists, code blocks, and common pitfalls.
keywords:
- go
- doc
- comment
- godoc
- heading
- link
- list
- code-block
- format
- gofmt
---

# Go Doc Comments

Reference: https://go.dev/doc/comment

## Core Rules

### What is a Doc Comment?
Comments immediately before top-level package, const, func, type, var declarations
with no intervening newlines. Every exported name should have one.

### Package Comments
- Start with "// Package <name> " (first sentence).
- Only ONE source file per package should have a package comment.
- Use complete sentences. Use semantic linefeeds (one sentence per line).

### Headings
- Use `# ` prefix (Go 1.19+): `// # Architecture`
- Must be unindented, separated by blank lines, single line.
- Before Go 1.19: implicit headings detected by single-line paragraphs without
  terminating punctuation. Gofmt may reformat these to `#` headings.
- **To avoid heading detection**: add period/colon, or break into two lines.

### Links
- Doc links: `[Name]` or `[pkg.Name]` to exported identifiers.
- URL links: `[Text]: URL` in a separate block at the end.
- Doc links must be surrounded by punctuation, spaces, or line boundaries.

### Lists
- Indented lines starting with `- `, `* `, `+ `, or `• ` (bullet).
- Indented lines starting with number + `.` or `)` (numbered).
- Gofmt reformats: dash bullets, 2-space indent before dash, 4-space continuation.

### Code Blocks
- Any span of indented non-list lines → preformatted text.
- Gofmt indents all lines by a single tab, inserts blank lines before/after.

### Paragraphs
- Unindented non-blank lines.
- Double backticks → left quote, double single quotes → right quote.
- Gofmt preserves line breaks (supports semantic linefeeds).

### Deprecation
- Paragraph starting with `Deprecated: ` triggers deprecation warnings.

### Notes
- Form: `MARKER(uid): body` where MARKER is 2+ uppercase letters.
- e.g. `TODO(user): ...`, `BUG(user): ...`

## Common Mistakes

1. **Unindented lists** — indented continuation lines make the last line a code block.
   Fix: indent all lines of the list.

2. **Indented paragraph text** — accidental code block.
   Fix: unindent.

3. **Nested lists** — not supported; gofmt flattens them.
   Fix: restructure text.

4. **Implicit headings** — single-line paragraphs without punctuation get reformatted
   to `#` headings by gofmt. If unintended, add a period or colon.

5. **Multiple package comments** — they get concatenated. Keep only one.

## Plain ASCII Principle

- Use ASCII for doc comments. Avoid Unicode box-drawing (`━`, `─`, `┌`, `┐`, etc.).
  They render inconsistently, break `grep`/`rg`, and cause implicit heading issues.
- Code-body section separators: use `===` instead of Unicode `───`.
- Package doc section titles: use plain `// Section Title:` paragraphs, not
  decorative box-drawing lines.

## Checking

```bash
go doc <package>    # Preview rendered doc
go vet ./...        # Catch issues
```