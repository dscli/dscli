package shellblock

import (
	"sort"
	"strings"
)

// codeRanges returns the byte ranges of QUOTED content in text: fenced blocks
// (``` or ~~~, per CommonMark) and inline code spans (a matched pair of
// backtick runs). Quoted content never delimits a <shell> block: a model
// showing how to write the protocol, or an expert quoting the test corpus,
// must not trigger an execution.
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
func codeRanges(text string) [][2]int {
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
	sort.Slice(ranges, func(i, j int) bool { return ranges[i][0] < ranges[j][0] })
	return ranges
}

// inRanges reports whether pos falls inside any of the sorted [start, end)
// byte ranges (quoted code). The walk stops once past pos, since ranges is
// sorted by offset.
func inRanges(ranges [][2]int, pos int) bool {
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
