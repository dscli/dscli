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
