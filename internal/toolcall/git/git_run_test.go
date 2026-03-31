package git

import (
	"slices"
	"strings"
	"testing"
)

func TestGitCommand(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		args    []string
		want    string
		wantErr bool
	}{
		{"git status --short", []string{"status", "--short"}, "", false},
		{"git log --oneline -1", []string{"log", "--oneline", "-1"}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			got, _, gotErr := GitCommand(ctx, tt.args...)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("gitCommand() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("gitCommand() succeeded unexpectedly")
			}
			// 对于git status --short，我们只检查是否没有错误
			// 对于git log --oneline -1，我们检查是否包含commit信息
			if tt.args[0] == "log" && tt.args[1] == "--oneline" {
				if !strings.Contains(got, " ") {
					t.Errorf("gitCommand() for log --oneline -1 should return commit info, got: %v", got)
				}
			}
		})
	}
}

func TestSubCommands(t *testing.T) {
	commands := SubCommands()
	if len(commands) == 0 {
		t.Fatal(commands)
	}
	for _, command := range []string{
		"clone",
		"add",
		"mv",
		"restore",
		"rm",
		"bisect",
		"diff",
		"grep",
		"log",
		"show",
		"status",
		"backfill",
		"branch",
		"commit",
		"merge",
		"rebase",
		"reset",
		"switch",
		"tag",
		"fetch",
		"pull",
		"push",
		"format-patch",
		"-C",
	} {
		if !slices.Contains(commands, command) {
			t.Fatal(command, commands)
		}
	}
}

