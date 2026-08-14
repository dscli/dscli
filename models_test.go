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

func TestFormatPrice(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want string
	}{
		{"unknown price is blank", 0, ""},
		{"integer price", 2, "2"},
		{"fractional price", 0.025, "0.025"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPrice(tt.in); got != tt.want {
				t.Fatalf("formatPrice(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
