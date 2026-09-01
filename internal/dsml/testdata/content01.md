## Overall Assessment

The refactor is well-scoped and mostly correct: `BuildDSMLToolDoc` now emits one `## \`name\`` block per tool (description + `Parameters:` line + XML example), the JSON schemas section is gone from code and templates, the `prompt.DSMLToolDoc.Schemas` field is removed at the type level (compile-time enforcement), and the `dev.md` "Ask, don't pretend" move matches the task spec. Tests pass for `internal/dsml` and `internal/prompt`.

Two things stand out. First, the `AGENTS.md` edit introduced a broken English sentence that will be read by every AI session — this is a real documentation defect. Second, although the task spec declares the rendered output "the sole authoritative format" with exact ordering, the tests only do fragment containment, so they cannot catch ordering, blank-line, or unwanted-section regressions.

## Specific Issues

### 1. Broken sentence in `AGENTS.md` (correctness, ships to every session) — high

`AGENTS.md` lines 167-170 now read:

```
section; it now states the strict format requirements
explicitly (complete tool_calls wrapper, string attribute on every
`HandleWebChat` injects it via
```

The sentence never closes. The diff dropped the tail ("parameter, no extra attributes such as justification, every tag closed).") but left the opening dangling, and the next paragraph starts mid-thought with a backtick-quoted `` `HandleWebChat` ``.

Fix suggestion — restore the missing tail so the parenthetical completes, e.g.:

```markdown
section; it now states the strict format requirements
explicitly (complete tool_calls wrapper, string attribute on every
parameter, no extra attributes, every tag closed).

`HandleWebChat` injects it via
```

This file is the project-wide contract; a mangled sentence there is worse than a code comment typo.

### 2. Test coverage is fragment-only despite "exact format" acceptance criteria — medium

`TestBuildDSMLToolDocContent` checks each wanted string appears *somewhere* in `doc.Intro` (`strings.Contains`). It never asserts:

- The per-tool block order: heading → description → `Parameters:` → ```xml example.
- The two trailing strict-format sentences appear after the encoding-rules paragraph (not interleaved).
- The absence of the retired sections. The old assertions for `### Available Tool Schemas` and `"name": "shell"` were deleted but no negative assertion was added. A regression that re-introduces the JSON block would pass today's tests.

The task's own acceptance criteria says the output must match the authoritative format "逐段一致" (segment-by-segment identical). Fragment containment cannot verify that.

Concrete fix: add a negative check plus ordering checks, or better, pin the whole output:

```go
if strings.Contains(doc.Intro, "### Available Tool Schemas") {
    t.Errorf("doc still renders the retired JSON schemas section:\n%s", doc.Intro)
}
if strings.Contains(doc.Intro, "```json") {
    t.Errorf("doc still renders JSON fences:\n%s", doc.Intro)
}
```

For ordering, either assert index relationships:

```go
shellHead := strings.Index(doc.Intro, "## `shell`")
shellDesc := strings.Index(doc.Intro, "Run a shell script.")
shellParams := strings.Index(doc.Intro, "Parameters: `script`")
shellXML := strings.Index(doc.Intro, "<invoke name=\"shell\">")
if !(shellHead < shellDesc && shellDesc < shellParams && shellParams < shellXML) { ... }
```

or maintain a golden/authoritative expected string (the task doc already contains it verbatim) and compare directly. Given the spec's emphasis, a golden comparison is the most faithful test.

### 3. `dsmlXMLClose` workaround: acceptable but fragile and slightly misleading — medium-low

`internal/dsml/doc.go` lines 35-43:

```go
var (
	dsmlParamClose  = "</para" + "meter>"
	dsmlInvokeClose = "</inv" + "oke>"
	dsmlCallsClose  = "</tool_" + "calls>"
)
```

The comment says this keeps "the DSML host parser" from misreading the Go string literals. Two observations:

- **These should be `const`, not `var`.** Go permits constant string concatenation, so `const dsmlParamClose = "</para" + "meter>"` compiles and avoids mutable package-level state that nothing mutates.

- **The comment slightly misattributes the cause.** Per the commit background, the constraint is the *dev session's file-modification host* swallowing literal XML close tags in tool payloads — an environment/tooling limitation, not the dscli runtime's "DSML host parser" (which parses chat replies, not Go source files). A future maintainer reading "keeps the DSML host parser from misreading the Go string literals" will be confused, since the dscli runtime never parses `doc.go`.

Suggested wording:

```go
// Close-tag fragments. They are assembled from pieces so this source file
// contains no literal closing tag; some file-modification hosts swallow
// whole XML close tags in tool payloads, which would corrupt edits to this
// file. The rendered prompt output is unaffected.
const (
	dsmlParamClose  = "</para" + "meter>"
	...
)
```

A cleaner form than concatenation would be `\x3c/para\x3e` escapes, but concatenation is fine and arguably more readable. The key point is the comment should name the actual host, not the dsml parser.

### 4. `TestBuildDSMLToolDocContent` doesn't verify close tags — low

The main content test asserts opening tags only (`<invoke name="shell">`, `<parameter name="script" string="true">`). Full close tags are only covered indirectly in `TestBuildDSMLToolDocGenerated` (lines 207-208). Since this commit is specifically about the rendered XML examples, add close-tag assertions to the content test:

```go
"</parameter>",
"</invoke>",
"</tool_calls>",
```

This also guards the `dsmlXMLClose` concatenation workaround against silent breakage (a typo like `"</para" + "meter"` would still pass today's main content test).

### 5. Documentation authority conflict — low

`docs/architecture-dsml-pkg.md` says the `dsmlGeneratedDocEntry` example is "格式规范的唯一权威" (the sole authoritative format), while the new `docs/task-dsml-tool-doc.md` declares its rendered block "唯一权威格式". Two docs now claim to be the single source of truth. If the task doc is intended to remain as historical record, it should be marked as such; if the architecture doc is the living spec, it should reference the task doc's authoritative block or vice versa. Worth a one-line reconciliation.

## Improvement Suggestions

- **Golden test**: since the task doc contains the full expected `BuildDSMLToolDoc("dev")` output verbatim, embedding it as a `want` string (or a `testdata` file) and comparing against `doc.Intro` directly would satisfy the "逐段一致" acceptance criterion and make future format changes deliberate. At minimum, add the negative assertions and ordering checks from issue 2.

- **`const` instead of `var`** for the three close-tag fragments (issue 3) — trivial, no behavior change.

- **Comment accuracy**: reword the `dsmlXMLClose` comment to describe the file-modification-host constraint rather than the dsml runtime parser.

## Summary

| Priority | Issue | Action |
|----------|-------|--------|
| **Immediate** | `AGENTS.md` L167-170 broken sentence | Restore the missing parenthetical tail; `HandleWebChat` paragraph should stand alone |
| **Should fix now** | Tests only assert fragment containment | Add negative assertions (`### Available Tool Schemas`, ```json) and ordering checks, or pin the golden output |
| **Should fix now** | `dsmlXMLClose` as mutable `var` + misleading comment | Change to `const`, correct the comment's stated cause |
| **Follow-up** | Close tags not asserted in main content test | Add `</parameter>`, `</invoke>`, `</tool_calls>` assertions |
| **Follow-up** | Two docs both claim "sole authoritative format" | Reconcile authority between architecture doc and task doc |

The `dsmlXMLClose` workaround itself is acceptable — the rendered output is unaffected and tests pass — but it should be documented accurately and made `const`. The retire-Schemas sweep is clean: no leftover `.Schemas` references remain outside the intentional historical task spec.
