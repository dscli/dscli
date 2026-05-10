package outfmt

import (
	"strings"
	"testing"
)

func TestOrgToMarkdown_Headings(t *testing.T) {
	tests := []struct {
		name  string
	input string
		want  string
	}{
		{
			name:  "level 1",
			input: "* Title",
			want:  "# Title",
		},
		{
			name:  "level 2",
			input: "** Section",
			want:  "## Section",
		},
		{
			name:  "level 3",
			input: "*** Subsection",
			want:  "### Subsection",
		},
		{
			name:  "heading with inline formatting",
			input: "* *bold* title",
			want:  "# **bold** title",
		},
		{
			name:  "not a heading - bold at line start",
			input: "*bold* text",
			want:  "**bold** text",
		},
		{
			name:  "not a heading - no space after star",
			input: "*bold*",
			want:  "**bold**",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OrgToMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("OrgToMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrgToMarkdown_InlineCode(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple inline code",
			input: "use =fmt.Println()= to print",
			want:  "use `fmt.Println()` to print",
		},
		{
			name:  "tilde verbatim",
			input: "use ~code~ here",
			want:  "use `code` here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OrgToMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("OrgToMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrgToMarkdown_BoldItalicStrikeUnderline(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "bold",
			input: "this is *bold* text",
			want:  "this is **bold** text",
		},
		{
			name:  "italic",
			input: "this is /italic/ text",
			want:  "this is *italic* text",
		},
		{
			name:  "strikethrough",
			input: "this is +struck+ text",
			want:  "this is ~~struck~~ text",
		},
		{
			name:  "underline",
			input: "this is _under_ text",
			want:  "this is <u>under</u> text",
		},
		{
			name:  "mixed bold and italic",
			input: "*bold* and /italic/",
			want:  "**bold** and *italic*",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OrgToMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("OrgToMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrgToMarkdown_Links(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "link with description",
			input: "see [[https://example.com][Example]] here",
			want:  "see [Example](https://example.com) here",
		},
		{
			name:  "link without description",
			input: "see [[https://example.com]] here",
			want:  "see [https://example.com](https://example.com) here",
		},
		{
			name:  "file link",
			input: "see [[file:main.go][main.go]] here",
			want:  "see [main.go](file:main.go) here",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := OrgToMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("OrgToMarkdown(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestOrgToMarkdown_CodeBlocks(t *testing.T) {
	input := strings.Join([]string{
		"#+begin_src go",
		"func main() {",
		`    fmt.Println("hello")`,
		"}",
		"#+end_src",
	}, "\n")
	want := strings.Join([]string{
		"```go",
		"func main() {",
		`    fmt.Println("hello")`,
		"}",
		"```",
	}, "\n")

	got := OrgToMarkdown(input)
	if got != want {
		t.Errorf("OrgToMarkdown code block:\n got:\n%q\n want:\n%q", got, want)
	}
}

func TestOrgToMarkdown_ExampleBlocks(t *testing.T) {
	input := strings.Join([]string{
		"#+begin_example",
		"some example",
		"text here",
		"#+end_example",
	}, "\n")
	want := strings.Join([]string{
		"```",
		"some example",
		"text here",
		"```",
	}, "\n")

	got := OrgToMarkdown(input)
	if got != want {
		t.Errorf("OrgToMarkdown example block:\n got:\n%q\n want:\n%q", got, want)
	}
}

func TestOrgToMarkdown_QuoteBlocks(t *testing.T) {
	input := strings.Join([]string{
		"#+begin_quote",
		"this is a quote",
		"with *bold* text",
		"#+end_quote",
	}, "\n")
	want := strings.Join([]string{
		"> this is a quote",
		"> with **bold** text",
	}, "\n")

	got := OrgToMarkdown(input)
	if got != want {
		t.Errorf("OrgToMarkdown quote block:\n got:\n%q\n want:\n%q", got, want)
	}
}

func TestOrgToMarkdown_MixedContent(t *testing.T) {
	input := strings.Join([]string{
		"* Title",
		"",
		"Some =code= and *bold* text.",
		"",
		"#+begin_src go",
		"func main() {}",
		"#+end_src",
		"",
		"See [[https://go.dev][Go]].",
	}, "\n")
	want := strings.Join([]string{
		"# Title",
		"",
		"Some `code` and **bold** text.",
		"",
		"```go",
		"func main() {}",
		"```",
		"",
		"See [Go](https://go.dev).",
	}, "\n")

	got := OrgToMarkdown(input)
	if got != want {
		t.Errorf("OrgToMarkdown mixed:\n got:\n%q\n want:\n%q", got, want)
	}
}

func TestOrgToMarkdown_EmptyInput(t *testing.T) {
	got := OrgToMarkdown("")
	if got != "" {
		t.Errorf("OrgToMarkdown(\"\") = %q, want \"\"", got)
	}
}

func TestOrgToMarkdown_PreservesNonOrg(t *testing.T) {
	input := "just plain text without any org markup"
	got := OrgToMarkdown(input)
	if got != input {
		t.Errorf("OrgToMarkdown(%q) = %q, want unchanged", input, got)
	}
}

func TestOrgToMarkdown_ZeroWidthSpace(t *testing.T) {
	// Zero-width spaces inserted by markdown2org should be stripped
	input := "text with \u200bzero-width\u200b spaces"
	want := "text with zero-width spaces"
	got := OrgToMarkdown(input)
	if got != want {
		t.Errorf("OrgToMarkdown with ZWS: got %q, want %q", got, want)
	}
}
