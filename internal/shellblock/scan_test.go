package shellblock

import (
	"strings"
	"testing"
)

// TestCodeRanges covers the quoted-content scanner on its own: fence and
// inline-span pairing, the mixed-marker line rule, and the inRanges
// membership walk.
func TestCodeRanges(t *testing.T) {
	tests := []struct {
		name string
		text string
		// wantOut are offsets that must NOT be inside any range (outside
		// quoted content); wantIn must be.
		wantIn  []int
		wantOut []int
	}{
		{
			name:    "closed fence",
			text:    "before\n```\nquoted\n```\nafter",
			wantIn:  []int{strings.Index("before\n```\nquoted\n```\nafter", "quoted")},
			wantOut: []int{0, len("before\n```\nquoted\n```\nafter") - 1},
		},
		{
			name:   "unclosed fence extends to EOF",
			text:   "```\nstill quoted",
			wantIn: []int{len("```\nstill")},
		},
		{
			name:    "tilde fence",
			text:    "x\n~~~\ncode\n~~~\n",
			wantIn:  []int{6},
			wantOut: []int{0, 1},
		},
		{
			name:    "mixed marker line is not a fence",
			text:    "`~~\nplain",
			wantOut: []int{0, len("`~~\nplain") - 1},
		},
		{
			name:    "inline span pairs by run length",
			text:    "a `code` b",
			wantIn:  []int{4},
			wantOut: []int{0, len("a `code` b") - 1},
		},
		{
			name:    "mismatched run lengths do not pair",
			text:    "a ``code` b",
			wantOut: []int{3, 8},
		},
		{
			name:    "unmatched backtick opens nothing",
			text:    "markdown uses ` for code",
			wantOut: []int{0, len("markdown uses ` for code") - 1},
		},
		{
			// The first run closes at the last run; the inner `` is content
			// of the same span, not a new one.
			name:   "inner ticks inside a longer span are content",
			text:   "`a `` b  a`",
			wantIn: []int{0, 4, 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ranges := codeRanges(tt.text)
			for _, pos := range tt.wantIn {
				if !inRanges(ranges, pos) {
					t.Errorf("pos %d must be inside a quoted range; ranges=%v", pos, ranges)
				}
			}
			for _, pos := range tt.wantOut {
				if inRanges(ranges, pos) {
					t.Errorf("pos %d must be outside every quoted range; ranges=%v", pos, ranges)
				}
			}
		})
	}
}
