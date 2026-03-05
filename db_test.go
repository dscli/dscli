package main

import (
	"testing"
)

func TestOpenDB(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		wantErr bool
	}{
		{"simple", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := OpenDB()
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("OpenDB() failed: %v", gotErr)
				}
				return
			}
			if got == nil {
				t.Fatal()
			}
		})
	}
}
