Let me analyze the diff carefully.

The commit changes ParseDSMLMessage to set OK=true for tool-call-free replies. Let me verify the logic against the actual full function.

Key concern: the test TestMessageContent01 embeds testdata/content01.md and reasoning01.md via go:embed. The testdata content contains literal <invoke name="..."> examples inside fenced code blocks, and the content also contains the problematic text — wait, actually the content01.md contains a review report that itself contains <invoke name="shell"> etc. inside fenced code blocks. But note the testdata content01.md also contains literal DSML-like content in the reasoning01.md? Let me check.

The main potential bug I want to investigate: the //go:embed "testdata/content01.md" pattern with quotes. In Go, go:embed directives with quotes — is that valid? Actually the existing files use //go:embed testdata/case1_edit_argument.txt without quotes. The new file uses quotes: //go:embed "testdata/content01.md". Go's go:embed accepts quoted string literals for patterns in Go 1.16+? Let me recall: the go:embed directive accepts a pattern, which can be a quoted string literal. Actually, per the spec: "The //go:embed directive accepts a pattern, or a quoted string literal containing a pattern?" Let me think. The embed package docs say patterns are like those in path.Match, and "The //go:embed directive accepts multiple space-separated patterns, and each pattern may be a string literal." Actually since Go 1.16, the go:embed directive's argument is a list of patterns. A pattern can be a quoted string. I believe quoted string is allowed. From the go command docs: "The //go:embed directive must be followed by one or more patterns, each of which is... The patterns must not contain '..' ... The patterns are interpreted relative to the package directory... The patterns may not use './' ... " And "A pattern may be written as a string literal, in which case it may contain spaces". Yes, that's legal. So the quotes are fine, though slightly unusual relative to repo convention. Existing repo conventions don't quote. So a nitpick: consistency — remove quotes.

Now the real review concerns.

Let me look at the actual logic change more deeply.

Before the change:
go

if !HasDSMLToolCalls(src) {
    return msg  // OK false
}
calls, strict, err := parseDSMLToolCallsStrict(src)
if err != nil || len(calls) == 0 {
    return msg  // OK false
}
msg.ToolCalls, _ = dsmlCallsToToolCalls(calls)
msg.OK = !strict
if msg.OK { ... strip ... }

After:
go

if !HasDSMLToolCalls(src) {
    msg.OK = true
    return msg
}
calls, strict, err := parseDSMLToolCallsStrict(src)
if err != nil {
    return msg // OK stays false
}
if len(calls) == 0 {
    msg.OK = !strict
    return msg
}
msg.ToolCalls, _ = dsmlCallsToToolCalls(calls)
msg.OK = !strict
if msg.OK { ... strip ... }

Now consider the case where HasDSMLToolCalls(src) is true but parseDSMLToolCallsStrict returns len(calls)==0 and strict==false. This is the "quoted/referenced examples only" case. OK = !strict = true. Good — this is the acceptance target.

But what about the case where src has a call marker, parse produces zero calls, and strict==true? Then OK=false. Good.

Potential issue 1: The comment on line 1275-1276 still says "ToolCalls empty: no executable call; whether the reply merely LOOKS like a broken tool call is SuspectedDSMLToolCalls' job." This is fine.

Potential issue 2: Documentation comment at line 1268-1271 says "OK is also true when the message carries no executable calls at all (no violations): a plain final answer, or quoted/referenced examples only." But for the len(calls)==0 with strict==true case, it IS a violation (normalize/stray marks). The comment says "no violations" as a parenthetical qualifier, which is accurate because msg.OK = !strict means OK=false when there are violations. But the phrasing "no executable calls at all (no violations)" could be read as "no executable calls implies no violations", which is wrong — a quoted example can carry normalize changes (e.g. fullwidth characters). The code sets OK=!strict, correctly. The comment is slightly ambiguous but okay.

Now let me look for the actual bug in the test.

TestMessageContent01:
go

message := ParseDSMLMessage(reasoning01, content01)
if !message.OK || len(message.ToolCalls) != 0 { t.Fatal(...) }
if message.Content != content01 { t.Fatal(message.Content) }
if message.ReasoningContent != reasoning01 { t.Fatal(message.Content) }

Third check uses t.Fatal(message.Content) — copy-paste bug: should be message.ReasoningContent. This is a real defect in the test: when ReasoningContent mismatch occurs, it prints the wrong value. This is worth flagging as a nitpick/correctness-of-test issue.

Now, a bigger concern: does content01.md contain literal <invoke name="..."> strings INSIDE fenced code blocks, but also does it contain the literal XML close tags outside code fences? Let me read the testdata. Actually the content01.md shown in the diff is the review report. Let me check: it contains lines like:
text

`AGENTS.md` lines 167-170 now read:

and fenced code blocks with ```, including:
markdown

section; it now states the strict format requirements
explicitly (complete tool_calls wrapper, string attribute on every
parameter, no extra attributes, every tag closed).

and
go

if strings.Contains(doc.Intro, "### Available Tool Schemas") {

and
xml

<invoke name="shell">

Wait — in the diff, the content01.md contains e.g.:
go

shellXML := strings.Index(doc.Intro, "<invoke name=\"shell\">")

And also in the fenced example:
text

<invoke name="shell">

Actually let me look at the actual content. The diff shows content01.md includes a line:
text

The sentence never closes. The diff dropped the tail ("parameter, no extra attributes such as justification, every tag closed).") but left the opening dangling, and the next paragraph starts mid-thought with a backtick-quoted `` `HandleWebChat` ``.

And there is a section listing dsmlXMLClose with:
go

var (
	dsmlParamClose  = "</para" + "meter>"
	dsmlInvokeClose = "</inv" + "oke>"
	dsmlCallsClose  = "</tool_" + "calls>"
)

Here's the interesting part: this fenced code block contains </para" + "meter> — which is NOT a literal </parameter> close tag. And the diff also includes a code block containing "</parameter>", "</invoke>", "</tool_calls>":
go

+"</parameter>",
+"</invoke>",
+"</tool_calls>",

Wait, those are inside a ```go fenced block in content01.md:
text

```go
+"</parameter>",
+"</invoke>",
+"</tool_calls>",

text


So content01.md DOES contain literal `</parameter>`, `</invoke>`, `</tool_calls>` close tags inside fenced code blocks. And it contains `<invoke name="shell">` opening tags inside fenced code blocks. The test asserts ParseDSMLMessage yields OK=true and zero ToolCalls — i.e., these quoted examples are correctly ignored.

The parser's dsmlCodeRanges handles fenced blocks, so quoted examples in fences are excluded. The inline spans too. So OK=true for len(calls)==0 && strict==false. This is the intended contract.

But wait — there's a subtlety. The testdata files are also the repository's own test corpus now, and the review report content embedded includes text describing a review of a *different* commit (the dsml tool doc refactor). This testdata is essentially a "realistic" sample. That's fine for an acceptance test, but 133 lines of review-report content with embedded literal close tags is fragile: if someone edits content01.md and accidentally breaks a fence, the test silently changes meaning. More importantly, this test does not isolate the specific property — it's a big golden-ish blob whose failure mode gives no diagnostic about which part failed. But the commit says it's the "acceptance target", so it's intentional.

Another real issue: `reasoning01.md` — I should check whether it contains call markers. Let me read it from the diff. The reasoning01.md content in the diff contains references to `\x3c/parameter\x3e` etc. and mentions `<invoke name="shell">`? It contains:

The tests assert <invoke name="shell"> (opening) and <parameter name="script" string="true"> (opening)
text


Wait, that's a literal `<invoke name="shell">` NOT in a code fence? Let me look at the reasoning01.md diff content more carefully. Lines like:

dsmlXMLClose workaround: the commit note asks to verify acceptable or suggest cleaner. ... Cleaner: use \x3c/parameter\x3e style or just write the literal...
text


There are lines with literal `<invoke name="shell">`? Let me scan:

- "AGENTS.md L169 is broken" — no markup.
- "dsmlXMLClose workaround..." — mentions `\x3c/parameter\x3e`.
- "Test coverage gaps..." 
- "paramsLine semicolon join..."
- "dsmlName and heading: ## \shell`— diff uses fmt.Fprintf..." — contains `## \shell`` — not invoke.
- "b.WriteString(\"</tool_calls>\") in doc.go line 82 — hold on, line 82 is an opening tag <tool_calls>\n, literal in source." — contains `<tool_calls>` opening tag literal in prose (not fenced). That's an OPENING wrapper tag, and HasDSMLToolCalls requires a named invoke OPEN (`dsmlNamedInvokeOpenRe`), not just `<tool_calls>`. So no call marker.

- "The tests assert <invoke name="shell"> (opening)" — wait does reasoning01.md actually contain that line? Looking at the diff of reasoning01.md, I see the line: "The tests assert `<invoke name="shell">` (opening)..."? Hmm, actually the diff shows near the end:

TestBuildDSMLToolDocContent order: The container assertions include both "Run a shell script." and "Read a file or line range." — these could appear anywhere. No per-tool ordering verification. Also assertion "## \shell"and"## read_file`"— fine but no check...
text


I don't see a literal `<invoke name="shell">` in reasoning01.md from the diff. But there is `\x3c/parameter\x3e` etc. Those are backslash-x escapes, not literal `<`.

Actually wait, there IS a literal in reasoning01.md:

b.WriteString("</tool_calls>") in doc.go line 82 — hold on, line 82 is an opening tag <tool_calls>\n, literal in source.
text


`<tool_calls>` is literal. But HasDSMLToolCalls matches `dsmlNamedInvokeOpenRe` — a named invoke open tag. `<tool_calls>` is not that. So no trigger. Good.

Also reasoning01.md contains `<parameter name="query" string="true">...</parameter>` and `<parameter name="path" string="true">`? Let me look:

... they do NOT assert the close tags </parameter>, </invoke>, </tool_calls> are present! TestGenerated does assert full close tags in line 207-208 backtick strings: <parameter name="query" string="true">...</parameter> — yes that includes close tags.
text


This includes `<parameter ...>` opening tag and `</parameter>` close tag literal in prose. Does `HasDSMLToolCalls` match `<parameter ...>`? No — it matches invoke open tags only. So no call marker in reasoning01.

But then, `CallSource(reasoning01, content01)`: content01 has HasDSMLToolCalls? content01.md contains `<invoke name="shell">` inside fences. HasDSMLToolCalls calls normalizeDSMLText then matches dsmlNamedInvokeOpenRe. Does dsmlNamedInvokeOpenRe respect code fences? No! HasDSMLToolCalls is just a regex on normalized text — it does NOT exclude fenced code. Look at line 87-89:

```go
func HasDSMLToolCalls(text string) bool {
	return dsmlNamedInvokeOpenRe.MatchString(normalizeDSMLText(text))
}

So content01.md, containing <invoke name="shell"> inside fenced code blocks, WILL match HasDSMLToolCalls → true. Therefore CallSource returns content (content first). Then ParseDSMLMessage calls parseDSMLToolCallsStrict(content01), which uses dsmlBlockRangesStrict + dsmlCodeRanges to exclude fenced blocks → zero calls, strict=false (provided no normalize changes). So OK=true. Good — that's the acceptance test path.

But this reveals something subtle: the first branch !HasDSMLToolCalls(src) is only for genuinely marker-free text (plain prose with no <invoke name= at all). The quoted-example path goes through the full parse. The commit's comment at line 1287 "No call marker at all" is accurate.

Now, an important correctness issue in the test: the test does not verify that content01.md actually contains quoted invoke examples (i.e., it doesn't guard against the testdata accidentally losing its <invoke> markers). If someone later edits content01.md and removes all invoke markers, the test would still pass via the first branch (OK=true) — it wouldn't exercise the quoted-example path at all. This is a test-robustness gap: the acceptance test for the quoted-example path should assert the input really contains invoke markers. Minor.

Now, let me think about whether there's a genuine logic bug in the new code.

Case analysis:

    No marker → OK=true. Is this always correct? What about a reply that is truncated in a way that HasDSMLToolCalls can't see (e.g., cut off at <invoke name="she with no >)? Line 83-86 documents that cut-off tags without > are indistinguishable from prose and pass through. That's a pre-existing documented limitation, not introduced here.

    Marker exists, err != nil (truncated/unclosed) → OK=false. Good.

    Marker exists, err nil, len(calls)==0 → OK=!strict. For quoted examples, strict=false → OK=true. For stray close marks or normalize changes → OK=false. Good.

    Marker exists, len(calls)>0 → OK=!strict, strip when OK.

Now the subtle semantic change: previously, a plain prose reply (no marker) returned OK=false. Now OK=true. The msg.OK field has dual semantics documented in message.go. Who reads OK for the webchat path? In handle.go line 643: if !msg.OK { InjectStrictWarning }. This is only reached when calls were parsed and executed (len(calls)>0 path). For the plain-prose/no-call path, the loop exits before reading msg.OK (line 597-618: parseErr != nil || len(calls)==0 → cleanExitVerbatim or Suspected re-issue). So for the webchat loop, OK's value on the no-call path is never consulted.

But there might be other consumers of ParseDSMLMessage. Grep showed only handle.go:576 calls it. And OK is also recomputed in history.go for a different purpose (pairing). Since ParseDSMLMessage constructs a fresh prompt.Message, its OK doesn't interact with history pairing. So the OK=true change is essentially only contract-visible via tests and possibly future callers. Fine.

But wait — in handle.go, line 597: if parseErr != nil || len(calls) == 0. Note this re-parses with ParseDSMLToolCalls (non-strict), not the strict parse. For a quoted-example reply, len(calls)==0 → enters the branch → SuspectedDSMLToolCalls(src)? For quoted examples, SuspectedDSMLToolCalls returns false because hasUnquotedInvokeOpen returns false (invoke markers are inside fences). So → cleanExitVerbatim(). Good, no re-issue. The OK value is irrelevant. So the semantic change does not alter routing. Good — the commit's claim "HasDSMLToolCalls deliberately unchanged" aligns.

Now, there's a potential issue with CallSource and HasDSMLToolCalls when reasoning contains quoted invoke markers (from the model's thinking quoting an example) and content is plain. Then fromReasoning=true, src=reasoning, parse gives zero calls, OK=true, and the strip branch is NOT reached (len(calls)==0 returns before strip). So msg.Content stays = content, msg.ReasoningContent stays = reasoning (unmodified). The test asserts both unchanged. Good.

Now let me check the content01.md embedded content for a potential real problem: the testdata contains the literal text </para" + "meter> and "</parameter>" inside fences. The strict parser's dsmlCodeRanges must handle nested backticks. content01.md has inline code with double backticks like `HandleWebChat` and fenced blocks. The parser presumably handles this. Since the test is the acceptance and presumably passes, fine. But I should verify the test actually passes — I should run go test ./internal/dsml/ -run TestMessageContent01 -v. That's a read-only check and valuable. Let me run it.

Also, I should verify whether the //go:embed "testdata/content01.md" (quoted) compiles. If it didn't, go test would fail to build. Running the test will confirm.

Let me also think about whether setting OK=true when !HasDSMLToolCalls(src) is the right contract given comment at line 1268: "OK=true: no violations". A plain final answer has no violations, so OK=true is consistent. Good.

Potential improvement: the len(calls) == 0 branch and the !HasDSMLToolCalls(src) branch now both return OK=true under slightly different conditions, but semantically they should be merged: msg.OK = !strict after parse; and when no marker, strict is trivially false. Actually the no-marker branch could just be folded: calls, strict, err := parseDSMLToolCallsStrict(src) would return nil, false, nil for no-marker text, so OK would be !strict = true — same result. The early return is just an optimization avoiding parse work. Fine.

One subtle behavioral gap: the comment at line 1269 says "OK is also true when the message carries no executable calls at all (no violations)". But what about a reply whose src has a marker, len(calls)==0, strict==false, but which actually has stray/incomplete markup that parseDSMLToolCallsStrict would set strict for? Handled via strict flag. But what about unclosed > 0 returning err — that's covered as OK=false. Good.

Now another issue: the doc comment at line 1272-1274 says "OK=false: violations observed (see parseDSMLToolCallsStrict); the Content/ReasoningContent keep the original text for the caller's fallback judgement." But in the len(calls)==0 branch with strict=true (quoted examples with normalize changes), the content is also kept original — consistent. And in the err branch, content kept original. Consistent.

Now the test nitpick: t.Fatal(message.Content) for the ReasoningContent mismatch. Also, the test prints raw values without context, making failures hard to debug. And the assertion doesn't use require.NoError etc.

Also, content01 and reasoning01 variable names: package-level vars named like a global (content01) conflict with nothing, but the embed block pattern with quoted strings differs from repo convention (unquoted). Nitpick: keep consistent, drop quotes.

Another point: The commit message says "User's new test TestMessageContent01 ... is the acceptance target." The test asserts message.Content == content01 and message.ReasoningContent == reasoning01. But since the no-strip path is taken (len(calls)==0 returns before strip), content isn't stripped of quoted examples. If the model's review report quotes <invoke> examples in content, should the final displayed content have those quotes stripped? The webchat loop's cleanExitStripped() handles stripping at exit for the "long reply quoting examples" path (line 622). ParseDSMLMessage itself does NOT strip in the len(calls)==0 case. So the test asserting content unchanged is consistent with current behavior, but is that the desired contract? For a review report quoting examples, leaving them verbatim is correct (they're content). Good.

Now, is there a risk that content01.md, when embedded, has a trailing newline that doesn't match? content01 embeds the file verbatim including final newline. ParseDSMLMessage returns msg.Content = content (the passed-in argument), and the test passes content01 as the argument, so it's trivially equal. Same for reasoning01. So those two assertions are almost tautological — they only verify ParseDSMLMessage didn't strip, which is the point. Fine.

Let me run the test to confirm.

Also run the full dsml package tests to check for regressions, since the plain-prose expectation changed.
