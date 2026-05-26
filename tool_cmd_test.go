package main

import "testing"

func TestFirstContentLine(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "heading first",
			input: "# Title\nReal description",
			want:  "Real description",
		},
		{
			name:  "no newlines",
			input: "Just a description",
			want:  "Just a description",
		},
		{
			name:  "blank lines first",
			input: "\n\nActual content",
			want:  "Actual content",
		},
		{
			name:  "all headings",
			input: "# A\n# B\n",
			want:  "",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "indented heading",
			input: "  # Title\nContent",
			want:  "Content",
		},
		{
			name:  "typical tool description",
			input: "# note\n\nSummarize session for future recall.\n\nRecord a key summary...",
			want:  "Summarize session for future recall.",
		},
		{
			name:  "hash without space skipped as heading",
			input: "#not-a-heading\nReal text",
			want:  "Real text",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firstContentLine(tt.input)
			if got != tt.want {
				t.Errorf("firstContentLine(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
