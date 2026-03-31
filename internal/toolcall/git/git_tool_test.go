package git

import (
	"fmt"
	"strings"
	"testing"
)

func Test_handleGit(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		toolArgs   ToolArgs
		result     string
		suggestion string
		errstr     string
	}{
		{"git help diff --help", ToolArgs{
			"command": "help",
			"args":    []string{"diff", "--help"},
		}, `git(command="help", args=["diff","--help"]) 失败`, "用法：git help [-a|--all]", "exit status 129"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			result, suggestion, err := handleGit(ctx, tt.toolArgs)
			errstr := fmt.Sprint(err)
			if errstr != tt.errstr {
				t.Fatal(err)
			}

			if !strings.Contains(result, tt.result) {
				t.Errorf("handleGit() = %v, result %v", result, tt.result)
			}
			if !strings.Contains(suggestion, tt.suggestion) {
				t.Errorf("handleGit() = %v, suggestion %v", suggestion, tt.suggestion)
			}
		})
	}
}
