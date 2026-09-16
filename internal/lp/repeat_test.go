package lp

import (
	"fmt"
	"strings"
	"testing"
)

// distinctProse builds a long NON-PERIODIC english passage: each sentence
// carries its own number, so no tail block repeats exactly. The detector must
// stay silent on text like this.
func distinctProse(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "Sentence %d covers topic %d, which differs from every other sentence in this passage.\n", i, i*7%113)
	}
	return b.String()
}

// distinctCJK is distinctProse for Chinese text.
func distinctCJK(lines int) string {
	var b strings.Builder
	for i := 0; i < lines; i++ {
		fmt.Fprintf(&b, "第 %d 句描述一个不同的话题，用来检验检测器不会误报。\n", i)
	}
	return b.String()
}

// TestDetectRepeatTail pins the endless-repetition detector: exact tail
// repetition triggers, everything a healthy reply can contain does not.
// The positive samples mirror the shapes users report (a repeated
// `</parameter> </invoke>` pair and long single-character runs).
func TestDetectRepeatTail(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		// Positive: the reported repetition shapes.
		{
			name: "parameter-invoke pair with newline",
			in:   strings.Repeat("</parameter> </invoke>\n", 200),
			want: true,
		},
		{
			name: "parameter-invoke pair no newline",
			in:   strings.Repeat("</parameter> </invoke>", 300),
			want: true,
		},
		{
			name: "parameter and invoke on separate lines",
			in:   strings.Repeat("</parameter>\n</invoke>\n", 200),
			want: true,
		},
		{
			name: "cjk full stop run (uniform branch)",
			in:   strings.Repeat("。", 9000),
			want: true,
		},
		{
			name: "cjk comma-period pair (uniform branch)",
			in:   strings.Repeat("，。", 5000),
			want: true,
		},
		// Negative: healthy output.
		{
			name: "plain english prose",
			in:   distinctProse(150),
			want: false,
		},
		{
			name: "cjk prose",
			in:   distinctCJK(150),
			want: false,
		},
		{
			name: "code with a long single-char run below the uniform run gate",
			in:   "func f() {\n\t" + strings.Repeat("a", 3000) + "\n}\n",
			want: false,
		},
		{
			name: "ab pair below the uniform run gate",
			in:   strings.Repeat("ab", 1200),
			want: false,
		},
		{
			name: "short dash rule",
			in:   strings.Repeat("-", 60) + "\n",
			want: false,
		},
		{
			name: "markdown table",
			in:   "| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n",
			want: false,
		},
		{
			name: "markdown list",
			in:   strings.Repeat("- item\n", 40),
			want: false,
		},
		{
			name: "whitespace-only long run never triggers",
			in:   strings.Repeat("\n", 20000),
			want: false,
		},
		{
			name: "spaces and tabs never trigger",
			in:   strings.Repeat(" \t", 10000),
			want: false,
		},
		{
			name: "empty",
			in:   "",
			want: false,
		},
		{
			name: "repetition not at the tail",
			in:   strings.Repeat("</parameter> </invoke>\n", 300) + "and then a normal closing sentence.\n",
			want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			unitLen, copies, ok := detectRepeatTail(tc.in)
			if ok != tc.want {
				t.Fatalf("detectRepeatTail ok = %v, want %v (unit=%d copies=%d)", ok, tc.want, unitLen, copies)
			}
			if ok && (unitLen <= 0 || copies < 2) {
				t.Errorf("detector reported ok with unit=%d copies=%d", unitLen, copies)
			}
		})
	}
}

// TestDetectRepeatTailThresholds pins the exact boundaries. For a 3-byte rich
// unit the binding gate is run >= repeatRichMinRun (2048), i.e. 683 copies
// (2049 bytes) trigger and 682 (2046 bytes) do not. For the 1-byte uniform
// unit the run gate (8192) is the binding one.
func TestDetectRepeatTailThresholds(t *testing.T) {
	const richCopies = (repeatRichMinRun + 2) / 3 // ceil(2048/3) = 683
	if _, copies, ok := detectRepeatTail(strings.Repeat("abc", richCopies)); !ok {
		t.Errorf("rich run at the threshold must trigger (copies=%d)", copies)
	}
	if _, _, ok := detectRepeatTail(strings.Repeat("abc", richCopies-1)); ok {
		t.Errorf("rich run one copy below the threshold must not trigger")
	}
	if _, copies, ok := detectRepeatTail(strings.Repeat("a", repeatUniformMinRun)); !ok {
		t.Errorf("uniform run at the threshold must trigger (copies=%d)", copies)
	}
	if _, _, ok := detectRepeatTail(strings.Repeat("a", repeatUniformMinRun-1)); ok {
		t.Errorf("uniform run one byte below the threshold must not trigger")
	}
}

// TestDetectRepeatTailWhitespaceOnly: a block made of whitespace alone has
// zero distinct non-space runes and must never trigger, however long.
func TestDetectRepeatTailWhitespaceOnly(t *testing.T) {
	for _, s := range []string{
		strings.Repeat(" ", 40000),
		strings.Repeat("\n \n", 10000),
		strings.Repeat("\r\n", 20000),
		"\u00a0\u00a0\u00a0", // non-breaking spaces are unicode space
	} {
		if _, _, ok := detectRepeatTail(s); ok {
			t.Errorf("whitespace-only input must not trigger: %q", s[:min(len(s), 20)])
		}
	}
}

// TestDistinctNonSpaceRunes pins the capped distinct-rune counter.
func TestDistinctNonSpaceRunes(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"   \n\t", 0},
		{"aaaa", 1},
		{"abab", 2},
		{"abcabc", 3},
		{"abcdef", 4}, // capped
		{"a b\tc\nd", 4},
	}
	for _, tc := range tests {
		if got := distinctNonSpaceRunes(tc.in); got != tc.want {
			t.Errorf("distinctNonSpaceRunes(%q) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// TestDetectRepeatTailWindow pins the scan window boundaries: at the window
// size the repetition is still seen, and an oversized prefix does not hide a
// qualifying tail (the window is anchored at the end, where repetition
// lives).
func TestDetectRepeatTailWindow(t *testing.T) {
	unit := "</parameter> </invoke>\n"

	// A repetition longer than the window: the tail-anchored window keeps a
	// whole number of copies and must still trigger.
	over := strings.Repeat(unit, repeatWindowBytes/len(unit)+64)
	if len(over) <= repeatWindowBytes {
		t.Fatalf("test fixture is too short: %d bytes", len(over))
	}
	if _, _, ok := detectRepeatTail(over); !ok {
		t.Errorf("a repetition longer than the window must still trigger")
	}

	// Exactly the window: build it by prefixing a filler whose length is
	// computed and asserted non-negative, so no silent wrap-around.
	const copies = 200
	repeated := strings.Repeat(unit, copies)
	fillerLen := repeatWindowBytes - len(repeated)
	if fillerLen < 0 {
		t.Fatalf("fixture does not fit the window: %d repeated + %d bytes", len(repeated), repeatWindowBytes)
	}
	exact := strings.Repeat("x", fillerLen) + repeated
	if len(exact) != repeatWindowBytes {
		t.Fatalf("fixture = %d bytes, want exactly %d", len(exact), repeatWindowBytes)
	}
	if _, _, ok := detectRepeatTail(exact); !ok {
		t.Errorf("a qualifying tail inside an exactly-window-sized input must trigger")
	}
}
