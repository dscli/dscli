package file

import (
	"reflect"
	"testing"
)

func TestCountContentLines(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"empty", "", 0},
		{"single no newline", "a", 1},
		{"single with trailing newline", "a\n", 1},
		{"two lines trailing newline", "a\n\n", 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := countContentLines(tc.content); got != tc.want {
				t.Fatalf("countContentLines(%q) = %d, want %d", tc.content, got, tc.want)
			}
		})
	}
}

func TestJoinLinesWithNewline(t *testing.T) {
	tests := []struct {
		name  string
		lines []string
		want  string
	}{
		{"nil", nil, ""},
		{"empty", []string{}, ""},
		{"single", []string{"a"}, "a\n"},
		{"two", []string{"a", "b"}, "a\nb\n"},
		{"trailing empty", []string{"a", ""}, "a\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := joinLinesWithNewline(tc.lines); got != tc.want {
				t.Fatalf("joinLinesWithNewline(%v) = %q, want %q", tc.lines, got, tc.want)
			}
		})
	}
}

func TestBuildReplacementLines(t *testing.T) {
	lines := []string{"L1", "L2", "L3", "L4", "L5"}
	tests := []struct {
		name      string
		startLine int
		endLine   int
		content   string
		want      []string
	}{
		{
			name:      "replace middle",
			startLine: 2,
			endLine:   4,
			content:   "N2\nN3",
			want:      []string{"L1", "N2", "N3", "L5"},
		},
		{
			name:      "start beyond EOF pads empty lines",
			startLine: 7,
			endLine:   7,
			content:   "X",
			want:      []string{"L1", "L2", "L3", "L4", "L5", "", "X"},
		},
		{
			name:      "endLine -1 keeps to tail",
			startLine: 3,
			endLine:   -1,
			content:   "R",
			want:      []string{"L1", "L2", "R"},
		},
		{
			name:      "empty content deletes range",
			startLine: 2,
			endLine:   3,
			content:   "",
			want:      []string{"L1", "L4", "L5"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := buildReplacementLines(lines, tc.startLine, tc.endLine, tc.content)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("buildReplacementLines(...) = %v, want %v", got, tc.want)
			}
		})
	}
}
