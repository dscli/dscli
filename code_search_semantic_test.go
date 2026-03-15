package main

import (
	"testing"
)

func TestHandleSearchCodeSemantic(t *testing.T) {
	ctx := t.Context()
	args := ToolArgs{
		"path":          "parse.py",
		"pattern":       "def parse_java",
		"context_lines": 5,
	}
	result, err := handleSearchCodeSemantic(ctx, args)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(result)
}
