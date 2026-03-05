package main

import (
	"context"
	"testing"
)

func TestLoadPrompts(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		want    []Message
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := LoadPrompts(context.Background())
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("LoadPrompts() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("LoadPrompts() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("LoadPrompts() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetSystemPrompt(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		want string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetSystemPrompt()
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("GetSystemPrompt() = %v, want %v", got, tt.want)
			}
		})
	}
}
