package toolcall

import (
	"testing"
)

func TestHandleSearchCodeSemantic(t *testing.T) {
	ctx := t.Context()
	args := ToolArgs{
		"file_pattern":   "*/*/parse.py",
		"search_pattern": "def parse_java",
		"context_lines":  5,
	}
	result, err := handleSearchCodeSemantic(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(result)
}
