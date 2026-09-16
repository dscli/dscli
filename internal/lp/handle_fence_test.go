package lp

import (
	"strings"
	"testing"
)

// TestFenceShellBlocks pins the print-only fence transform: `<shell>` and
// `</shell>` delimiter lines get xml code fences, while anything already
// fenced or merely quoting the markers stays untouched.
func TestFenceShellBlocks(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "no block",
			in:   "plain text\nstill plain\n",
			want: "plain text\nstill plain\n",
		},
		{
			name: "middle block",
			in:   "before\n<shell>\necho hi\n</shell>\nafter\n",
			want: "before\n```xml\n<shell>\necho hi\n</shell>\n```\nafter\n",
		},
		{
			name: "block opens on first line",
			in:   "<shell>\necho hi\n</shell>\nafter\n",
			want: "```xml\n<shell>\necho hi\n</shell>\n```\nafter\n",
		},
		{
			name: "block closes on last line without trailing newline",
			in:   "before\n<shell>\necho hi\n</shell>",
			want: "before\n```xml\n<shell>\necho hi\n</shell>\n```\n",
		},
		{
			name: "two blocks",
			in:   "<shell>\na\n</shell>\nmid\n<shell>\nb\n</shell>\n",
			want: "```xml\n<shell>\na\n</shell>\n```\nmid\n```xml\n<shell>\nb\n</shell>\n```\n",
		},
		{
			name: "already fenced",
			in:   "```xml\n<shell>\necho hi\n</shell>\n```\n",
			want: "```xml\n<shell>\necho hi\n</shell>\n```\n",
		},
		{
			name: "already fenced then a real block",
			in:   "```xml\n<shell>\nx\n</shell>\n```\n<shell>\ny\n</shell>\n",
			want: "```xml\n<shell>\nx\n</shell>\n```\n```xml\n<shell>\ny\n</shell>\n```\n",
		},
		{
			name: "tilde fence toggles too",
			in:   "~~~\n<shell>\nx\n</shell>\n~~~\n",
			want: "~~~\n<shell>\nx\n</shell>\n~~~\n",
		},
		{
			name: "markers inside a sentence are untouched",
			in:   "use <shell> like this\nand </shell> that way\n",
			want: "use <shell> like this\nand </shell> that way\n",
		},
		{
			name: "surrounding whitespace is trimmed for matching",
			in:   "before\n  <shell>  \nx\n  </shell>  \nafter\n",
			want: "before\n```xml\n  <shell>  \nx\n  </shell>  \n```\nafter\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fenceShellBlocks(tc.in); got != tc.want {
				t.Errorf("fenceShellBlocks mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestFenceShellBlocksClosingTagAtEOF: the closing fence must be emitted even
// when the block's `</shell>` line ends the text with no newline.
func TestFenceShellBlocksClosingTagAtEOF(t *testing.T) {
	got := fenceShellBlocks("<shell>\nx\n</shell>")
	if !strings.HasSuffix(got, "</shell>\n```\n") {
		t.Errorf("closing fence missing at EOF: %q", got)
	}
}
