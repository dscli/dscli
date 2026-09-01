# Review the whole picture. I've gathered enough. Key issues identified:

    AGENTS.md L169 is broken — "explicitly (complete tool_calls wrapper, string attribute on every" then next line starts "HandleWebChat injects it via" — the sentence never closes. This is a genuine defect from this diff (the diff shows this exact hunk). Confirm.

    dsmlXMLClose workaround: the commit note asks to verify acceptable or suggest cleaner. It's a code smell. The comment says the dev session's file-modification host swallows literal XML close tags in tool payloads. Cleaner: use \x3c/parameter\x3e style or just write the literal and rely on... Actually, the safer approach: build the close tags via string concatenation is hacky but works. But there's a subtle bug: dsmlParamClose etc. are package-level vars. But also — the comment claims "this source file never contains a whole close tag" — yet line 82 b.WriteString("<tool_calls>\n") is an opening tag, fine; but also other source files in repo contain close tags extensively (AGENTS.md, doc_test.go line 207 ...</parameter>). The workaround is localized but fragile and undocumented beyond comment. Also the var block at package level triggers gofmt? It's fine. But the "host swallows" reasoning is about the tool payload of the dev session — i.e., an AI file-modification host, not the production runtime. That's environment-specific and shouldn't be encoded into product source as permanent fragmentation. Cleaner suggestion: use raw string with entity-free but assembled from \x3c escapes? Actually \x3c is still the literal <. Could suggest: write close tags as \x3c/parameter\x3e — but that still contains < visually? No, \x3c is escape sequence, source text is backslash-x-3-c. That would avoid literal < in source. But the current concat approach is equivalent in effect. Alternatively suggest: put literal XML close tag in a raw string and document; or use a helper function closeTag(name). But the cleanest is: document the constraint in AGENTS.md or the file header, and perhaps make it a small const builder. It's acceptable, but should be flagged as tech-debt/misleading if the real reason is only the review harness.

Actually more important: the "DSML host parser" comment is misleading — it says "keeps the DSML host parser from misreading the Go string literals". But this code runs in our local parser? No — the comment conflates: this file is Go source; when we (AI) read or write this file via tool payloads, our tool-call parser might misread literal </...> tags. That's a real concern for the AI tooling environment, not the dscli runtime. The code comment attributes it to "DSML host parser", which is inaccurate-ish. Worth flagging for maintainability: a future dev reading this will be confused.

    Test coverage gaps: no test asserts per-tool ordering (title → description → Parameters → example) nor the two trailing strict sentences relative order, nor heading level distinctness (## per-tool vs ## available tools). The task doc says output format must be exactly authoritative. The tests only check strings.Contains of fragments — they can't catch ordering issues, double spacing, missing blank lines, or the exact placement. Also there's no test that ### Available Tool Schemas is absent. The diff removed assertions for it but should add a negative assertion !strings.Contains(doc.Intro, "### Available Tool Schemas") and !strings.Contains(doc.Intro, "```json") to guard regressions. Also the task's acceptance criteria #3 asks for "逐段一致" (segment-by-segment identical) with authoritative format, but tests only do fragment containment. Improvement: add a golden-string test or ordering assertions.

    paramsLine semicolon join: the task spec says "参数间 ; 分隔,末尾无句号" — code matches. But strings.TrimSpace(e.description) strips trailing whitespace from tool descriptions — fine, but if description has leading/trailing formatting or embedded trailing newline, stripped. Low risk. Also Description might be empty — then you'd get heading + blank line + Parameters; acceptable but untested.

    dsmlName and heading: ## \shell`— diff uses fmt.Fprintf("##%s`") consistent. Fine.

    The else branch dev.md: the new sentence "If information is missing..." — task spec explicitly asks this. Fine. But note the contrast: AGENTS.md "AI Assistant Context" says "Ask instead of guessing"; architect handles ask. The rationale given in task doc seems backwards ("ask user is architect's thing") — not a code issue, just noting the rationale is internal. Not worth flagging strongly.

    b.WriteString("</tool_calls>") in doc.go line 82 — hold on, line 82 is an opening tag <tool_calls>\n, literal in source. The close-tag workaround only applies to closing tags. But look at line 82: \tvar b strings.Builder\n\tb.WriteString("<tool_calls>\n") — opening tag literal is fine for AI tool parser? The issue is closing tags in tool payloads (our file-modification tool would contain literal </invoke> etc.). Opening tags don't terminate. OK.

But this raises inconsistency: doc.go header comment line 14 says "model sees the native tool names and parameter schemas" — good. But no leftover encoding/json — confirmed removed.

    Leftover docs references: the grep shows the only .DSMLToolDoc.Schemas references left are in docs/task-dsml-tool-doc.md (the new task spec — intentional, historical/instructional) — but that file is a committed task doc that will remain in repo. Its acceptance criteria use the old term deliberately. That's fine — historical task doc. No leftover in templates, prompt.go, AGENTS.md (AGENTS.md says "no JSON schemas section" now). Check skills doc: diff updated it. Good.

But wait — internal/dsml/dsml.go L876 comment updated to doc.go — good. But is there any leftover "dsml_doc.go" reference? Diff updated it. Let me not over-check.

    TestBuildDSMLToolDocContent order: The container assertions include both "Run a shell script." and "Read a file or line range." — these could appear anywhere. No per-tool ordering verification. Also assertion "## \shell`"and"## `read_file`"— fine but no check on heading level difference from the## 🛠️ Available Tools. Actually the per-tool heading is also ##` — same level as section title, which the task doc explicitly decided (二级标题). Fine.

    Potential hidden issue: The dsmlXMLClose vars are package-level mutable vars. Since they're used only in one function, they could be const — but you can't concat string consts (constant string concatenation IS allowed: const x = "</para" + "meter>"). Actually Go allows constant string concatenation! const dsmlParamClose = "</para" + "meter>" is valid — strings are constant. So the var block could be const. That's cleaner and gofumpt wouldn't complain. But the existing uses are fine. Minor nitpick: use const instead of var since these never change.

    The comment on dsmlXMLClose references "DSML host parser" but the actual concern, per commit background, is the "dev session's file-modification host swallows literal XML close tags in tool payloads." This is a tooling-level constraint. The fix is legitimate given environment, but should be documented in a way that doesn't mislead: the code comments say "keeps the DSML host parser from misreading the Go string literals; the prompt output is unaffected." Actually this comment is not too bad. But the reviewer should verify: is the workaround actually working in tests? The tests assert <invoke name="shell"> (opening) and <parameter name="script" string="true"> (opening) — they do NOT assert the close tags </parameter>, </invoke>, </tool_calls> are present! TestGenerated does assert full close tags in line 207-208 backtick strings: <parameter name="query" string="true">...</parameter> — yes that includes close tags. So close tags ARE asserted there. But TestBuildDSMLToolDocContent only asserts openings, no closing. So close-tag correctness is only covered via probe_tool test. Weak coverage: add close tag assertions in main content test too.

    Duplicated/redundant trailing sentences: Lines 225-226 append the two strict sentences. The task spec says "Intro 末尾必须保留这两句(原 Schemas 段的)". Good.

    Potential major issue: prompt template injection — dev.md has {{if .DSMLToolDoc.Intro}} then {{.DSMLToolDoc.Intro}} then content then {{else}}. The architect/test/expert/review templates removed {{.DSMLToolDoc.Schemas}} but do they have proper spacing? Diff shows they deleted the line, leaving the preceding text followed by blank line then {{else}}. Need to verify no double blank lines or missing blank lines. It looks fine from diff.

    Test in prompt_test.go: diff shows doc literal constructed with concatenation <invoke name="read_file"> + \n + <parameter name="path" string="true">AGENTS.md — note it's incomplete (no closing tags) but that's fine for a fake doc. No Schemas reference left.

One more important point: The commit message says "docs: add dsml tool doc task spec" — but the task doc includes Chinese text, while AGENTS.md says commit messages English. That's fine, the task doc content is Chinese like other task docs.

Also: docs/task-dsml-tool-doc.md is a new file that says "这是唯一权威格式" — but docs/architecture-dsml-pkg.md also claims "dsmlGeneratedDocEntry 生成的示例逐字节同构(示例就是格式规范的唯一权威)". Two docs now claim "sole authoritative format" — potentially conflicting authorities. The architecture doc was updated to point to doc.go. Minor documentation tension: architecture-dsml-pkg says "示例就是格式规范的唯一权威" (the example is the sole authoritative format), while task doc says the rendered block in the task doc is the only authority. Not a code bug; maybe worth a note.

Now: heading level. The diff uses ## \shell`— same##level as the section header## 🛠️ Available Tools:. The task doc explicitly chose this. But markdown-wise, per-tool headings at the same level as the section heading means each tool block is visually a sibling section — acceptable. The review request specifically asked to verify "per-tool heading level (## vs ###)". Answer: it's ##, matching the task doc requirement and the diff. But note: the OLD format had ### Available Tool Schemas; now there are no ###at all in the intro. Confirm no###generated — code writes only##`. Fine.

Let me check: ## 🛠️ Available Tools: shell, read_file and then per-tool `## `shell. Two H2 headings in a row with no H3 — but that's per spec.

Potential correctness issue: strings.TrimSpace(e.description) then b.WriteString — if description is multi-line, TrimSpace only trims ends; interior preserved. Fine.

    Security: none relevant.

    Performance: fine.

Now I should also verify no other references to .Schemas exist in test code or files beyond task doc and old references. Grep result only shows task doc. Good. But grep for "Schemas" case-insensitively or DSMLToolDoc{ literals? The first grep covered DSMLToolDoc.Schemas|Available Tool Schemas|\.Schemas\b. Let me quickly check .Schemas without word boundary issue? \.Schemas\b — that catches doc.Schemas. All remaining are in task doc only, which is the historical spec — intentional. Good.

Let me also verify no leftover encoding/json import unused — code compiles, tests pass.

Now prioritize issues:

High / immediate:

    AGENTS.md broken sentence at L169 (docs correctness, ships to every AI session). Must fix.

Medium:

    Test coverage is fragment-only: no negative assertions for ### Available Tool Schemas/ ```json, no ordering verification despite "唯一权威格式" claim. Suggest a golden test or at least ordering + negative assertions.

    dsmlXMLClose workaround: acceptable but should be const (Go allows const string concatenation), and comment should accurately describe the host constraint; flag as tech debt/fragility. Cleaner alternative: raw \x3c/... escapes or a tiny helper.

Low:

    TestContent doesn't assert close tags; add.

    Docs authority conflict between task doc and architecture doc ("唯一权威").

    Use const not var.

Let me write the review in English, structured.

I think I have enough. No more tool calls needed.
