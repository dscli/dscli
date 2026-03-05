package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestFimRunE(t *testing.T) {
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
			gotErr := FimRunE(tt.cmd, tt.args)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("FimRunE() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("FimRunE() succeeded unexpectedly")
			}
		})
	}
}
