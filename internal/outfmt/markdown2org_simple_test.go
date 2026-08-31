package outfmt

import "testing"

func TestMarkdownToOrgConverter_ConvertMarkdownSimple(t *testing.T) {
	c := &MarkdownToOrgConverter{}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"bold", "**bold**", "*bold*"},
		{"bold unclosed", "**unclosed", "**unclosed"},
		{"italic", "*italic*", "/italic/"},
		{"italic middle", "a*b*c", "a/b/c"},
		{"bold italic", "***bolditalic***", "**bolditalic**"},
		{"strike", "~~strike~~", "+strike+"},
		{"strike unclosed", "~~unclosed", "~~unclosed"},
		{"inline code", "`code`", " =code= "},
		{"double backtick pass through", "``not code``", "``not code``"},
		{"link", "[text](url)", "[[url][text]]"},
		{"link unclosed", "[text](unclosed", "[text](unclosed"},
		{"bracket only", "[text]", "[text]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.convertMarkdownSimple(tc.in); got != tc.want {
				t.Fatalf("convertMarkdownSimple(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMarkdownToOrgConverter_ConvertItalicInBold(t *testing.T) {
	c := &MarkdownToOrgConverter{}
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"unclosed star", "*abc", "*abc"},
		{"middle italic", "a*b*", "a/b/"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.convertItalicInBold(tc.in); got != tc.want {
				t.Fatalf("convertItalicInBold(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
