// Package shellblock implements the <shell> block tool protocol for WebChat:
// the web model emits one bash script per round inside a <shell> block, the
// judge decides whether to execute the block, warn about its format, or
// accept the reply as the final answer, and the runner executes the script
// locally (scriptN.sh / scriptN.txt under the project root) and feeds the
// merged output back into the conversation as an attached text file.
//
// The protocol and its judgement rules were validated in a 64-round manual
// experiment; docs/task-shell-block.md records the contract and the archive
// evidence. The judge follows the DSML channel's discipline (see
// internal/dsml): quoted examples never execute, malformed shapes never
// execute, and the destructive-command interception (dsml.BlockedCmdRe) is
// shared - a remote web model is not a trusted local agent.
package shellblock

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/dsml"
)

// Action is the judge's decision for one model reply.
type Action int

const (
	// ActionFinal is a reply without a block that reads as a final report:
	// the loop exits and returns the reply as the outcome.
	ActionFinal Action = iota
	// ActionExecute is a well-formed block, ready to run (Block is set).
	ActionExecute
	// ActionWarn is a malformed or stalled reply: send Warning back into the
	// conversation and keep the session alive.
	ActionWarn
)

// Block is one extracted <shell> block.
type Block struct {
	// Script is the verbatim script body between the <script> and </script>
	// lines (a trailing newline is ensured when the file is written).
	Script string
	// Summary is the <summary> value, "" when absent.
	Summary string
	// Timeout is the <timeout> value, defaulted to DefaultTimeout and capped
	// at MaxTimeout. Never zero for an extracted block.
	Timeout time.Duration
}

// Verdict is the judge's decision for one reply.
type Verdict struct {
	Action Action
	// Block is set when Action is ActionExecute.
	Block *Block
	// Warning is the message sent back to the model when Action is
	// ActionWarn.
	Warning string
	// Issue names the malformed shape for logs (warn verdicts only).
	Issue string
}

const (
	// DefaultTimeout is the <timeout> default: plenty for a quick command,
	// small enough that a stuck round does not stall the session.
	DefaultTimeout = 120 * time.Second
	// MaxTimeout caps <timeout>: a test suite or a build can need much more
	// than the default, but a single round stays bounded.
	MaxTimeout = 1800 * time.Second

	// minFinalRunes is the stalled-reply threshold: a block-less reply
	// shorter than this neither contains a block nor reads as a final
	// report. The shortest legitimate no-block round in the manual
	// experiment was 113 characters, so 100 leaves headroom.
	minFinalRunes = 100
)

// Judge classifies one model reply. content comes first; reasoning is the
// fallback only when content is blank.
//
// Quoted content (fenced code blocks and inline code spans, scanned with
// dsml.CodeRanges) never delimits a block: a model quoting the protocol
// example must not trigger an execution. The same scan applies to every
// tag check below.
//
// A well-formed block (all four tags, each exactly once, on their own
// lines, in order, non-empty body) is executable. A malformed attempt
// (missing, duplicated, out-of-order or garbled tags, an empty body) gets
// the format contract back. DSML call shapes are warned about even when no
// block is present - a silently dropped attempt is worse than a redundant
// warning. Otherwise a short reply is treated as a stalled round and a long
// one as the final report.
func Judge(reasoning, content string) Verdict {
	text := content
	if strings.TrimSpace(text) == "" {
		text = reasoning
	}
	quoted := dsml.CodeRanges(text)
	if block, issue := extract(text, quoted); block != nil {
		return Verdict{Action: ActionExecute, Block: block}
	} else if issue != "" {
		return Verdict{
			Action:  ActionWarn,
			Issue:   issue,
			Warning: MalformedWarning(issue, hasDSMLShape(text)),
		}
	}
	if hasDSMLCallShape(text) {
		return Verdict{Action: ActionWarn, Issue: "dsml-shape", Warning: DSMLShapeWarning()}
	}
	if utf8.RuneCountInString(text) <= minFinalRunes {
		return Verdict{Action: ActionWarn, Issue: "no-block-short", Warning: NoBlockWarning(hasDSMLShape(text))}
	}
	return Verdict{Action: ActionFinal}
}

// ShouldEnter reports whether a reply routes into the shell loop: any
// verdict other than ActionFinal (a block to run, or a shape that needs a
// format warning). Shared by the first-round routing in HandleWebChat and
// the resume branch so the two gates cannot drift.
func ShouldEnter(reasoning, content string) bool {
	return Judge(reasoning, content).Action != ActionFinal
}

// tagLine is one exact tag line: the tag it matched, its start offset, and
// the offset just past its line (including the newline, when present).
type tagLine struct {
	tag        string
	start, end int
}

// collectTagLines returns the exact tag lines (strict whole-line equality)
// that are not inside quoted content, in text order.
func collectTagLines(text string, quoted [][2]int) []tagLine {
	var hits []tagLine
	off := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if !dsml.InRanges(quoted, off) {
			switch tag := strings.TrimRight(line, " \t\r\n"); tag {
			case "<shell>", "<script>", "</script>", "</shell>":
				hits = append(hits, tagLine{tag: tag, start: off, end: off + len(line)})
			}
		}
		off += len(line)
	}
	return hits
}

// indexOfTag returns the index of the first hit with the given tag at or
// after from, or -1.
func indexOfTag(hits []tagLine, tag string, from int) int {
	for i := from; i < len(hits); i++ {
		if hits[i].tag == tag {
			return i
		}
	}
	return -1
}

// extract scans for exactly one well-formed block on a first-match basis:
//
//   - the first <shell> line opens the block; tag-shaped lines before it are
//     prose and are ignored;
//   - the first <script> line after it opens the body, and everything up to
//     the first </script> line after THAT is opaque body - a script that
//     writes a protocol example may legitimately contain tag lines as data;
//   - the first </shell> line after the body closes the block; a further
//     <shell> open after the close means a second attempt and is refused as
//     "more than once" - the only duplicate check that survives first-match
//     semantics (stray duplicate lines elsewhere are tolerated, so a valid
//     block is never false-rejected).
//
// It returns the block on success, or a non-empty issue naming the
// malformed shape. An empty issue means "no block attempt at all": the
// reply then falls through to the final-report judgement.
func extract(text string, quoted [][2]int) (*Block, string) {
	hits := collectTagLines(text, quoted)
	if len(hits) == 0 {
		return nil, variantIssue(text, quoted)
	}
	iShell := indexOfTag(hits, "<shell>", 0)
	if iShell < 0 {
		return nil, "the `<shell>` line is missing"
	}
	// A <script> before the <shell> is an ordering error, not a missing line.
	if i := indexOfTag(hits, "<script>", 0); i >= 0 && i < iShell {
		return nil, "the tags are out of order"
	}
	iScript := indexOfTag(hits, "<script>", iShell+1)
	if iScript < 0 {
		return nil, "the `<script>` line is missing"
	}
	iCloseScript := indexOfTag(hits, "</script>", iScript+1)
	if iCloseScript < 0 {
		return nil, "the `</script>` line is missing"
	}
	iCloseShell := indexOfTag(hits, "</shell>", iCloseScript+1)
	if iCloseShell < 0 {
		return nil, "the `</shell>` line is missing"
	}
	if indexOfTag(hits, "<shell>", iCloseShell+1) >= 0 {
		return nil, "the tags appear more than once - send exactly one block"
	}
	scriptOpen, scriptClose := hits[iScript], hits[iCloseScript]
	shellClose := hits[iCloseShell]
	body := text[scriptOpen.end:scriptClose.start]
	if strings.TrimSpace(body) == "" {
		return nil, "the script body is empty"
	}
	// <summary>/<timeout> are parsed loosely in the window between the
	// </script> line and the </shell> line; a missing or invalid value
	// falls back to its default.
	window := text[scriptClose.end:shellClose.start]
	return &Block{
		Script:  body,
		Summary: fieldValue(window, "summary"),
		Timeout: parseTimeout(window),
	}, ""
}

// variantIssue names the tag-shaped lines that are not exact tags: an
// indented tag, a garbled/badge-rendered one, or a truncated fragment. It
// runs only when no exact tag line was found at all; a reply with no
// tag-shaped content returns "" (nothing was attempted).
func variantIssue(text string, quoted [][2]int) string {
	off := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		if dsml.InRanges(quoted, off) {
			off += len(line)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			off += len(line)
			continue
		}
		for _, tag := range []string{"<shell>", "</shell>", "<script>", "</script>"} {
			if trimmed == tag {
				return "an indented tag variant"
			}
		}
		for _, fragment := range []string{"<shell", "</shell", "<script", "</script"} {
			if strings.HasPrefix(trimmed, fragment) {
				return "a garbled or truncated tag variant"
			}
		}
		if strings.Contains(trimmed, "\uff5c") &&
			(strings.Contains(trimmed, "shell") || strings.Contains(trimmed, "script")) {
			return "a badge-rendered tag variant"
		}
		off += len(line)
	}
	return ""
}

// fieldValue returns the value of the first line in window shaped like
// <name>value</name> (built from single lines only, per the protocol).
func fieldValue(window, name string) string {
	open, closeTag := "<"+name+">", "</"+name+">"
	for _, line := range strings.Split(window, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, open) && strings.HasSuffix(t, closeTag) {
			return strings.TrimSpace(t[len(open) : len(t)-len(closeTag)])
		}
	}
	return ""
}

// parseTimeout parses the <timeout> value: seconds, defaulted when absent,
// invalid, or non-positive, capped at MaxTimeout.
func parseTimeout(window string) time.Duration {
	raw := fieldValue(window, "timeout")
	if raw == "" {
		return DefaultTimeout
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return DefaultTimeout
	}
	timeout := time.Duration(seconds) * time.Second
	if timeout > MaxTimeout {
		return MaxTimeout
	}
	return timeout
}

// hasDSMLShape reports whether text carries DSML tool-call shapes. The site
// badges and mangles them (rounds 43/53 of the manual experiment), so every
// warning for such a reply appends the "do not use DSML" note.
func hasDSMLShape(text string) bool {
	return strings.Contains(text, "<invoke") ||
		strings.Contains(text, "<tool_calls") ||
		strings.Contains(text, "\uff5c")
}

// hasDSMLCallShape is the strong subset of hasDSMLShape: an actual call
// attempt (an <invoke> open or the badge's fullwidth bars). A block-less
// reply carrying it is warned about even when long.
func hasDSMLCallShape(text string) bool {
	return strings.Contains(text, "<invoke") || strings.Contains(text, "\uff5c")
}
