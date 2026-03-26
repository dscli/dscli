package toolcall

import (
	_ "embed"
	"encoding/json"
	"testing"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

func TestToolArgsValue(t *testing.T) {
	args := ToolArgs{
		"path":   "main.go",
		"append":  true,
		"content": "very long content...actual not so log",
	}

	b, err := outfmt.JSONMarshal(&args)
	if err != nil {
		t.Fatal(err, string(b))
	}

	// Go marshal output may have different sequence order, so hardcoded it here
	b = []byte(`{"path":"main.go","append":true,"content":"very long content...actual not so log"}`)
	truncateds := string(b[0 : len(b)-10])
	want := `{"path":"main.go","append":true,"content":"very long content...actual no`
	if truncateds != want {
		t.Fatal(truncateds)
	}

	truncatedArgs := ToolArgs{".rawArgs": truncateds}
	rawArgs := ToolArgsValue(truncatedArgs, ".rawArgs", "")
	if len(rawArgs) == 0 {
		t.Fatal(rawArgs)
	}

	s := string([]byte(rawArgs))
	if s != want {
		t.Fatal(s)
	}

	s += `"}`
	data := []byte(s)
	err = json.Unmarshal(data, &truncatedArgs)
	if err != nil {
		t.Fatal(err)
	}

	append := ToolArgsValue(truncatedArgs, "append", false)
	if !append {
		t.Fatal(append)
	}
	path := ToolArgsValue(truncatedArgs, "path", "")

	if path != "main.go" {
		t.Fatal(path)
	}

	content := ToolArgsValue(truncatedArgs, "content", "")
	if content != "very long content...actual no" {
		t.Fatal(content)
	}
}

func TestToolArgs_Unmarshal(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		wantErr bool
		rawArgs string
	}{
		{
			"truncated", false,
			"{\"path\": \"architecture-design-v3.md\", " +
				"\"content\": \"\\n## 7. 监控与告警" +
				"\\n\\n### tmpl'\\n```\\n\\n### 7.4 仪表板",
		},
		{
			"almost done", false,
			"{\"path\": \"architecture-design-v3.md\", " +
				"\"content\": \"\\n## 7. 监控与告警" +
				"\\n\\n### tmpl'\\n```\\n\\n### 7.4 仪表板" + `"`,
		},
		{
			"done actually", false,
			"{\"path\": \"architecture-design-v3.md\", " +
				"\"content\": \"\\n## 7. 监控与告警" +
				"\\n\\n### tmpl'\\n```\\n\\n### 7.4 仪表板" + `"}`,
		},

		{
			"done actually", false,
			"{\"path\": \"architecture-design-v3.md\", " +
				"\"content\": \"\\n## 7. 监控与告警" +
				"\\n\\n### tmpl'\\n```\\n\\n### 7.4 仪表板" + `\\}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// TODO: construct the receiver type.
			args := ToolArgs{".rawArgs": tt.rawArgs}
			gotErr := args.Unmarshal()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Unmarshal() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Unmarshal() succeeded unexpectedly")
			}
			path := ToolArgsValue(args, "path", "")
			if path != "architecture-design-v3.md" {
				t.Fatal(path, args)
			}
			content := ToolArgsValue(args, "content", "")
			if content != `
## 7. 监控与告警

### tmpl`+"'"+`
`+"```"+`

` {
				t.Fatal(content)
			}
		})
	}
}
