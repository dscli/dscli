package lp

import (
	"strings"
	"testing"
)

func TestPrettyJSONInMarkdown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single-line JSON object gets pretty-printed",
			in:   "```\n{\"key\":\"value\"}\n```",
			want: "```json\n{\n  \"key\": \"value\"\n}\n```",
		},
		{
			name: "single-line JSON array gets pretty-printed",
			in:   "```\n[1,2,3]\n```",
			want: "```json\n[\n  1,\n  2,\n  3\n]\n```",
		},
		{
			name: "pkg.go.dev API response (preserves key order)",
			in:   "```\n{\"modulePath\":\"github.com/google/go-cmp\",\"version\":\"v0.7.0\",\"isLatest\":true}\n```",
			want: "```json\n{\n  \"modulePath\": \"github.com/google/go-cmp\",\n  \"version\": \"v0.7.0\",\n  \"isLatest\": true\n}\n```",
		},
		{
			name: "non-JSON code block left unchanged",
			in:   "```\nfunc main() {}\n```",
			want: "```\nfunc main() {}\n```",
		},
		{
			name: "already has json language tag with compact JSON",
			in:   "```json\n{\"a\":1}\n```",
			want: "```json\n{\n  \"a\": 1\n}\n```",
		},
		{
			name: "multiple blocks, mix of JSON and non-JSON",
			in:   "# Title\n\n```\n{\"x\":1}\n```\n\nSome text.\n\n```\nplain\n```",
			want: "# Title\n\n```json\n{\n  \"x\": 1\n}\n```\n\nSome text.\n\n```\nplain\n```",
		},
		{
			name: "no code blocks — passthrough",
			in:   "# Hello\n\nWorld",
			want: "# Hello\n\nWorld",
		},
		{
			name: "empty code block",
			in:   "```\n```",
			want: "```\n```",
		},
		{
			name: "JSON with language tag already pretty",
			in:   "```json\n{\n  \"a\": 1\n}\n```",
			want: "```json\n{\n  \"a\": 1\n}\n```",
		},
		{
			name: "invalid JSON preserved",
			in:   "```\n{invalid}\n```",
			want: "```\n{invalid}\n```",
		},
		{
			name: "code block without closing fence",
			in:   "```\n{\"a\":1}",
			want: "```\n{\"a\":1}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := prettyJSONInMarkdown(tt.in)
			if got != tt.want {
				t.Errorf("prettyJSONInMarkdown:\n  got:  %q\n  want: %q", got, tt.want)
			}
		})
	}
}

func TestTryParseJSON(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantJSON bool
		want     string // expected pretty output (if isJSON)
	}{
		{
			name:     "simple object",
			in:       "{\"a\":1}",
			wantJSON: true,
			want:     "{\n  \"a\": 1\n}",
		},
		{
			name:     "simple array",
			in:       "[1,2,3]",
			wantJSON: true,
			want:     "[\n  1,\n  2,\n  3\n]",
		},
		{
			name:     "non-JSON string",
			in:       "hello world",
			wantJSON: false,
		},
		{
			name:     "empty string",
			in:       "",
			wantJSON: false,
		},
		{
			name:     "already pretty JSON stays structurally same",
			in:       "{\n  \"a\": 1\n}",
			wantJSON: true,
			want:     "{\n  \"a\": 1\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tryParseJSON(tt.in)
			if ok != tt.wantJSON {
				t.Errorf("tryParseJSON(%q) ok=%v, want %v", tt.in, ok, tt.wantJSON)
			}
			if ok && got != tt.want {
				t.Errorf("tryParseJSON(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestPrettyJSONInMarkdown_RoundTrip(t *testing.T) {
	in := "```\n{\"name\":\"cmp\",\"synopsis\":\"Package cmp determines equality of values.\",\"isRedistributable\":true}\n```"
	got := prettyJSONInMarkdown(in)
	if !strings.Contains(got, "\n  \"") {
		t.Errorf("expected indented JSON, got: %s", got)
	}
}
