package main

import (
	"context"
	"testing"
)

func TestCodeMakeFormat(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		wantErr bool
	}{
		{"normal test", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := CodeMakeFormat(context.Background())
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CodeMakeFormat() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("CodeMakeFormat() succeeded unexpectedly")
			}
			if got == "" {
				t.Errorf("CodeMakeFormat() = %v", got)
			}
		})
	}
}
