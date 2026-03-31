package git

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_handleGit(t *testing.T) {
	tests := []struct {
		name       string
		toolArgs   ToolArgs
		result     string
		suggestion string
		errstr     string
	}{
		{"git help diff --help", ToolArgs{
			"command": "help",
			"args":    []string{"diff", "--help"},
		}, `git(command="help", args=["diff","--help"]) 失败`, "用法：git help [-a|--all]", "exit status 129"},
		{"git help diff", ToolArgs{
			"command": "help",
			"args":    []string{"diff"},
		}, `git-diff - Show changes between commits, commit and working tree, etc`, ``, `<nil>`},
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

func Test_handleGit_C(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		result     string
		suggestion string
		errstr     string
	}{
		{
			"git help diff --help",
			[]string{"help", "diff", "--help"},
			`"diff","--help"]) 失败`, "用法：git help [-a|--all]", "exit status 129",
		},
		{
			"git help diff",
			[]string{"help", "diff"},
			`git-diff - Show changes between commits, commit and working tree, etc`, ``, `<nil>`,
		},

		{
			"git log",
			[]string{"log"},
			`Init`, ``, `<nil>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			dir, err := os.MkdirTemp("", "handleGit_C*")
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() func() {
				_, _, err = GitCommand(ctx, "-C", dir, "init")
				if err != nil {
					t.Fatal(err)
				}
				name := filepath.Join(dir, "README.md")
				err = os.WriteFile(name, []byte("# Testing\nTest it"), 0o600)
				if err != nil {
					t.Fatal(err)
				}
				_, _, err = GitCommand(ctx, "-C", dir, "add", "README.md")
				if err != nil {
					t.Fatal(err)
				}
				_, _, err = GitCommand(ctx, "-C", dir, "commit", "-m", "Init")
				if err != nil {
					t.Fatal(err)
				}

				return func() {
					if err = os.RemoveAll(dir); err != nil {
						t.Fatal(err)
					}
				}
			}())
			toolArgs := ToolArgs{
				"command": "-C",
				"args":    append([]string{dir}, tt.args...),
			}
			result, suggestion, err := handleGit(ctx, toolArgs)
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
