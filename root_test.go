package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestRootPreRunE(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		cmd     *cobra.Command
		args    []string
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := RootPreRunE(tt.cmd, tt.args)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("RootPreRunE() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("RootPreRunE() succeeded unexpectedly")
			}
		})
	}
}

func TestRootExecute(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := RootExecute()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("RootExecute() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("RootExecute() succeeded unexpectedly")
			}
		})
	}
}
