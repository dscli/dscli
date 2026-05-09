package skills

import (
	"testing"

	"github.com/goccy/go-yaml"
)

// TestNormalizeFrontmatter tests the normalizeFrontmatter function
// with various description colon cases.
func TestNormalizeFrontmatter(t *testing.T) {
	tests := []struct {
		name          string
		fm            string
		wantDesc      string
		wantParseFail bool
	}{
		{
			name:          "unquoted colon in description",
			fm:            "name: test\ndescription: Use this skill when: the user asks about PDFs",
			wantDesc:      "Use this skill when: the user asks about PDFs",
			wantParseFail: false,
		},
		{
			name:          "already single-quoted with colon",
			fm:            "name: test\ndescription: 'Use when: needed'",
			wantDesc:      "Use when: needed",
			wantParseFail: false,
		},
		{
			name:          "already double-quoted with colon",
			fm:            `name: test` + "\n" + `description: "Use when: needed"`,
			wantDesc:      "Use when: needed",
			wantParseFail: false,
		},
		{
			name:          "no colon in description",
			fm:            "name: test\ndescription: A simple description",
			wantDesc:      "A simple description",
			wantParseFail: false,
		},
		{
			name:          "description contains single quote",
			fm:            "name: test\ndescription: it's working now: great",
			wantDesc:      "it's working now: great",
			wantParseFail: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalized := normalizeFrontmatter(tt.fm)
			var s Skill
			err := yaml.Unmarshal([]byte(normalized), &s)
			if tt.wantParseFail {
				if err == nil {
					t.Errorf("expected parse failure but got success: normalized=%q", normalized)
				}
				return
			}
			if err != nil {
				t.Fatalf("yaml unmarshal failed: %v\nnormalized=%q", err, normalized)
			}
			if s.Description != tt.wantDesc {
				t.Errorf("Description = %q, want %q", s.Description, tt.wantDesc)
			}
		})
	}
}
