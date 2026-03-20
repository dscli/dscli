package main

import (
	"os"
	"path/filepath"
	"testing"

	"gitcode.com/dscli/dscli/internal/context"
)

func TestMain(t *testing.T) {
	// Just stay here, nothing to do
}

func TestGetenv(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		key    string
		dvalue string
		want   string
	}{
		{"EmptyKey00", "", "", ""},
		{"EmptyKey01", "", "yes", "yes"},
		{"WithKey01", "withkey01", "", ""},
		{"WithKey02", "withkey02", "yes", "yes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := context.Getenv(tt.key, tt.dvalue)
			if got != tt.want {
				t.Fatalf("Getenv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetProjectRoot(t *testing.T) {
	projectRoot := func() string {
		p, err := filepath.Abs(".")
		if err != nil {
			t.Fatal(err)
		}
		return p
	}()

	currentWorkDir := func() string {
		cwd, err := os.Getwd()
		if err != nil {
			t.Fatal(err)
		}
		return cwd
	}()

	tests := []struct {
		name string // description of this test case
		cwd  string
		want string
	}{
		{"current", currentWorkDir, projectRoot},
		{"docs", filepath.Join(currentWorkDir, "docs"), projectRoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.Chdir(tt.cwd)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Chdir(currentWorkDir); err != nil {
					t.Fatal(err)
				}
			}()
			got := context.GetProjectRoot()
			if got != tt.want {
				t.Fatalf("GetProjectRoot() = %v, want %v", got, tt.want)
			}
		})
	}
}
