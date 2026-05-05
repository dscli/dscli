package outfmt

import (
	"strings"
	"testing"
)

func TestFillParagraph(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxWidth int
		want     string
	}{
		{
			name:     "empty string",
			input:    "",
			maxWidth: 80,
			want:     "",
		},
		{
			name:     "short text no wrap",
			input:    "Hello world",
			maxWidth: 80,
			want:     "Hello world",
		},
		{
			name:     "long text wraps at word boundary",
			input:    "The quick brown fox jumps over the lazy dog. This sentence is quite long and should wrap.",
			maxWidth: 30,
			want:     "The quick brown fox jumps over\nthe lazy dog. This sentence is\nquite long and should wrap.",
		},
		{
			name:     "single newline normalized to space",
			input:    "Line one\nLine two\nLine three",
			maxWidth: 80,
			want:     "Line one Line two Line three",
		},
		{
			name:     "multiple spaces collapsed",
			input:    "Hello    world   test",
			maxWidth: 80,
			want:     "Hello world test",
		},
		{
			name:     "very long word exceeds width",
			input:    "Supercalifragilisticexpialidocious is a long word",
			maxWidth: 20,
			want:     "Supercalifragilisticexpialidocious\nis a long word",
		},
		{
			name:     "default width on zero",
			input:    "short",
			maxWidth: 0,
			want:     "short",
		},
		{
			name:     "negative width uses default",
			input:    "short",
			maxWidth: -1,
			want:     "short",
		},
		{
			name:     "already wrapped text preserved",
			input:    "Line one\nLine two\n\nLine three\nLine four",
			maxWidth: 80,
			want:     "Line one Line two\n\nLine three Line four",
		},
		{
			name:     "trailing and leading whitespace trimmed",
			input:    "  \n  hello world  \n  ",
			maxWidth: 80,
			want:     "hello world",
		},
		{
			name:     "multiple paragraphs with wrapping",
			input:    "This is the first paragraph which is quite long and needs to be wrapped at word boundaries.\n\nThis is the second paragraph which is also very long and should be wrapped nicely.",
			maxWidth: 40,
			want:     "This is the first paragraph which is\nquite long and needs to be wrapped at\nword boundaries.\n\nThis is the second paragraph which is\nalso very long and should be wrapped\nnicely.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FillParagraph(tt.input, tt.maxWidth)
			if got != tt.want {
				t.Errorf("FillParagraph() =\n%q\nwant:\n%q", got, tt.want)
			}
		})
	}
}

func TestFillParagraph_NoLineExceedsWidth(t *testing.T) {
	text := strings.Repeat("word ", 100)
	maxWidth := 40
	result := FillParagraph(text, maxWidth)

	for _, line := range strings.Split(result, "\n") {
		if len(line) > maxWidth {
			t.Errorf("line exceeds max width %d: %q (len=%d)", maxWidth, line, len(line))
		}
	}
}

func TestFillParagraph_Idempotent(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog. This is a test of the emergency broadcast system."
	maxWidth := 40
	first := FillParagraph(text, maxWidth)
	second := FillParagraph(first, maxWidth)
	if first != second {
		t.Errorf("FillParagraph is not idempotent:\nfirst:  %q\nsecond: %q", first, second)
	}
}
