package main

import (
	"context"
	"testing"
)

func TestLoadHistory(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		want    []Message
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := LoadHistory(context.Background())
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("LoadHistory() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("LoadHistory() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("LoadHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSaveMessages(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		msgs    []Message
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := SaveMessages(tt.msgs)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("SaveMessages() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("SaveMessages() succeeded unexpectedly")
			}
		})
	}
}
