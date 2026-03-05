package main

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestModelsRun(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		cmd  *cobra.Command
		args []string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ModelsRun(tt.cmd, tt.args)
		})
	}
}
