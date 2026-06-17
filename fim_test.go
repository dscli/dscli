package main

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)
func TestFimRunE(t *testing.T) {
	// Empty prompt should return an error.
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	err := FimRunE(cmd, nil)
	if err == nil {
		t.Fatal("FimRunE() with empty prompt should fail")
	}
}
