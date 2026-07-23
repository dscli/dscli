package file

import "testing"

func TestDetectCASTags(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{
			name:    "empty content",
			content: "",
			want:    0,
		},
		{
			name:    "single short line",
			content: "abc",
			want:    0,
		},
		{
			name: "no tags - normal text",
			content: `Hello, this is a normal file.
It has normal content without any CAS tags.
Just regular text.`,
			want: 0,
		},
		{
			name: "no tags - common english 4-letter words (all lowercase)",
			content: `echo "hello world"
test some code
data flows here
file contents are
list all things`,
			want: 0,
		},
		{
			name: "no tags - code with 4-letter prefixes but colon separator",
			content: `Note: this is important
TODO: fix this bug
FIXME: this is broken
HACK: this is ugly`,
			want: 0,
		},
		{
			name: "no tags - capitalized english words (e.g. Note, Time)",
			content: `Note that this is important
Time will tell if this works
Next steps involve testing
Last but not least, verify`,
			want: 0,
		},
		{
			name: "detect - bare CAS tags with mixed chars",
			content: `[Q8fA] package main
[eh7b] import "fmt"
[4Y5Q] func main() {
[_1aB] fmt.Println("hello")
}`,
			want: 4,
		},
		{
			name: "detect - full colon format with line numbers",
			content: `1:[Q8fA] package main
2:[eh7b] import "fmt"
3:[4Y5Q] func main() {
4:[_1aB] fmt.Println("hello")
}`,
			want: 4,
		},
		{
			name: "detect - below threshold (2 lines bare tag)",
			content: `[Q8fA] line one
[eh7b] line two
normal line without tags`,
			want: 2,
		},
		{
			name: "detect - mixed format (colon + bare)",
			content: `1:[Q8fA] package main
[eh7b] import "fmt"
[4Y5Q] func main() {
normal line without tags`,
			want: 3,
		},
		{
			name: "detect - tags with digits only",
			content: `[1abc] line one
[2def] line two
[3ghi] line three`,
			want: 3,
		},
		{
			name: "detect - tags with underscore",
			content: `[_abc] line one
[_d2f] line two
[_g3i] line three`,
			want: 3,
		},
		{
			name: "no tags - single tag line alone (below threshold)",
			content: `[Q8fA] just one line with a tag
rest of file is normal and clean`,
			want: 1,
		},
		{
			name: "detect - tags with non-first uppercase",
			content: `[Q8fA] first line
[aBcd] second line
[AbCd] third line`,
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectCASTags(tt.content)
			if got != tt.want {
				t.Errorf("detectCASTags() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsTagLike(t *testing.T) {
	tests := []struct {
		s    string
		want bool
	}{
		// Tag-like (should be detected)
		{"Q8fA", true}, // has digit
		{"4Y5Q", true}, // has digit
		{"eh7b", true}, // has digit
		{"_1aB", true}, // has underscore
		{"a_bc", true}, // has underscore
		{"aBcd", true}, // non-first uppercase
		{"AbCd", true}, // non-first uppercase (C)
		{"ABCD", true}, // non-first uppercase
		{"DATA", true}, // non-first uppercase
		{"ECHO", true}, // non-first uppercase
		// Not tag-like (common English words)
		{"Note", false}, // capitalized word
		{"Time", false}, // capitalized word
		{"Next", false}, // capitalized word
		{"Last", false}, // capitalized word
		{"echo", false}, // all lowercase
		{"data", false}, // all lowercase
		{"test", false}, // all lowercase
		{"file", false}, // all lowercase
	}
	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			if got := isTagLike(tt.s); got != tt.want {
				t.Errorf("isTagLike(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStripCASTags(t *testing.T) {
	tests := []struct {
		name         string
		content      string
		expectedTags []string
		wantContent  string
		wantChanged  bool
	}{
		{
			name:         "empty content",
			content:      "",
			expectedTags: []string{"Q8fA"},
			wantContent:  "",
			wantChanged:  false,
		},
		{
			name:         "no expected tags",
			content:      "hello world",
			expectedTags: nil,
			wantContent:  "hello world",
			wantChanged:  false,
		},
		{
			name:         "strip bracketed tag",
			content:      "[Q8fA] package main",
			expectedTags: []string{"Q8fA"},
			wantContent:  "package main",
			wantChanged:  true,
		},
		{
			name:         "strip bare tag",
			content:      "eh7b import \"fmt\"",
			expectedTags: []string{"eh7b"},
			wantContent:  "import \"fmt\"",
			wantChanged:  true,
		},
		{
			name:         "strip colon with bracketed tag",
			content:      "1:[Q8fA] package main",
			expectedTags: []string{"Q8fA"},
			wantContent:  "package main",
			wantChanged:  true,
		},
		{
			name:         "strip colon with bare tag",
			content:      "3:eh7b import \"fmt\"",
			expectedTags: []string{"eh7b"},
			wantContent:  "import \"fmt\"",
			wantChanged:  true,
		},
		{
			name:         "strip multiple lines",
			content:      "1:[Q8fA] package main\n2:[eh7b] import \"fmt\"\n3:[4Y5Q] func main() {\n4:[_1aB]     fmt.Println(\"hello\")\n}",
			expectedTags: []string{"Q8fA", "eh7b", "4Y5Q", "_1aB"},
			wantContent:  "package main\nimport \"fmt\"\nfunc main() {\n    fmt.Println(\"hello\")\n}",
			wantChanged:  true,
		},
		{
			name:         "bracketed and bare mixed",
			content:      "[Q8fA] package main\neh7b import \"fmt\"\n3:[4Y5Q] func main() {\n4:_1aB     fmt.Println(\"hello\")\n}",
			expectedTags: []string{"Q8fA", "eh7b", "4Y5Q", "_1aB"},
			wantContent:  "package main\nimport \"fmt\"\nfunc main() {\n    fmt.Println(\"hello\")\n}",
			wantChanged:  true,
		},
		{
			name:         "more lines than tags -- stop after matching tags",
			content:      "[Q8fA] line one\n[eh7b] line two\nline three untouched\nline four untouched",
			expectedTags: []string{"Q8fA", "eh7b"},
			wantContent:  "line one\nline two\nline three untouched\nline four untouched",
			wantChanged:  true,
		},
		{
			name:         "no match return unchanged",
			content:      "normal text without any tags",
			expectedTags: []string{"Q8fA"},
			wantContent:  "normal text without any tags",
			wantChanged:  false,
		},
		{
			name:         "tag mismatch prefix does not match",
			content:      "[XXXX] some content",
			expectedTags: []string{"Q8fA"},
			wantContent:  "[XXXX] some content",
			wantChanged:  false,
		},
		{
			name:         "empty expected tag in middle",
			content:      "[Q8fA] line one\nsome content\n[eh7b] line three",
			expectedTags: []string{"Q8fA", "", "eh7b"},
			wantContent:  "line one\nsome content\nline three",
			wantChanged:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotContent, gotChanged := stripCASTags(tt.content, tt.expectedTags)
			if gotChanged != tt.wantChanged {
				t.Errorf("stripCASTags() changed = %v, want %v", gotChanged, tt.wantChanged)
			}
			if gotContent != tt.wantContent {
				t.Errorf("stripCASTags() content = %q, want %q", gotContent, tt.wantContent)
			}
		})
	}
}
