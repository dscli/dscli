package main

import (
	"context"
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
		{"git status --short", []string{"status", "--short"}, ` M chat.go
 M git_diff.go
 M git_test.go`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := gitCommand(context.Background(), tt.args...)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("gitCommand() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("gitCommand() succeeded unexpectedly")
			}
			if tt.want != got {
				t.Errorf("gitCommand() = \n[%v]\n, want \n[%v]\n", got, tt.want)
			}
		})
	}
}
