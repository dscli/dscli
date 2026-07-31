package lp

import "testing"

func TestExtractResponse(t *testing.T) {
	tests := []struct {
		name     string
		baseline string
		current  string
		want     string
	}{
		{name: "appended", baseline: "abc", current: "abcd", want: "d"},
		{name: "unchanged", baseline: "abc", current: "abc", want: ""},
		{name: "shrunk", baseline: "abcd", current: "abc", want: ""},
		// The U+FFFD bug: current no longer starts with baseline (textarea
		// cleared after send), so a suffix slice would return garbage.
		{name: "prefix mismatch", baseline: "xyz", current: "abcd", want: ""},
		{name: "empty", baseline: "", current: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractResponse(tt.baseline, tt.current); got != tt.want {
				t.Errorf("extractResponse(%q, %q) = %q, want %q", tt.baseline, tt.current, got, tt.want)
			}
		})
	}
}

func TestIsCompleteResponse(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{name: "empty", s: "", want: false},
		{name: "replacement char", s: "\uFFFD", want: false},
		// The model pauses after emitting a simulated tool call that the
		// web UI cannot execute; the fragment must not be returned.
		{name: "tool call fragment", s: "<read_file path=\"AGENTS.md\" />", want: false},
		{name: "quoted tool call fragment", s: "> <read_file path=\"AGENTS.md\" />", want: false},
		// A full answer that includes the simulated call plus body is fine.
		{name: "full review with tool call", s: "> <read_file path=\"AGENTS.md\" />\n\n> <tool_result>\n# AGENTS.md\n\n## Overall Assessment\nSolid.", want: true},
		{name: "short with tool result", s: "<read_file />\n<tool_result>ok</tool_result>", want: true},
		// A genuine short answer with an XML-like tag must not be rejected
		// as a tool-call fragment (regression guard for the whitelist).
		{name: "short answer with html tag", s: "<b>bold</b> is fine", want: true},
		{name: "plain answer", s: "The change looks correct.", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCompleteResponse(tt.s); got != tt.want {
				t.Errorf("isCompleteResponse(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStripBaselinePrefix(t *testing.T) {
	tests := []struct {
		name string
		resp string
		base string
		want string
	}{
		{name: "prefix stripped", resp: "history\nnew", base: "history", want: "new"},
		{name: "no prefix", resp: "history\nnew", base: "other", want: "history\nnew"},
		{name: "empty baseline", resp: "new", base: "", want: "new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripBaselinePrefix(tt.resp, tt.base); got != tt.want {
				t.Errorf("stripBaselinePrefix(%q, %q) = %q, want %q", tt.resp, tt.base, got, tt.want)
			}
		})
	}
}
