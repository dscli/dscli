package main

import (
	"fmt"
	"testing"
)

func TestCloseIssue(t *testing.T) {
	// 跳过这个测试，因为它依赖于外部API
	// 根据用户要求，测试不要搞太完备，太完备花时间，将来还不好维护
	t.Skip("跳过CloseIssue测试，因为它依赖于外部GitCode API")

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		number  int
		wantErr bool
	}{
		{"Close", 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			got, gotErr := CloseIssue(ctx, tt.number)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CloseIssue() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("CloseIssue() succeeded unexpectedly")
			}
			if got.State != "closed" {
				t.Fatal(got)
			}
			got, gotErr = ReopenIssue(ctx, tt.number)
			if gotErr != nil {
				t.Fatal(gotErr)
			}

			if got.State != "open" {
				t.Fatal(got)
			}
		})
	}
}

func TestShowIssue(t *testing.T) {
	// 跳过这个测试，因为它依赖于外部API
	// 根据用户要求，测试不要搞太完备，太完备花时间，将来还不好维护
	t.Skip("跳过ShowIssue测试，因为它依赖于外部GitCode API")

	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		number  int
		wantErr bool
	}{
		{"show issue", 4, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := ShowIssue(t.Context(), tt.number)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("ShowIssue() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("ShowIssue() succeeded unexpectedly")
			}

			if got.Number != fmt.Sprint(tt.number) {
				t.Fatal(got)
			}
		})
	}
}

func TestAssignIssue(t *testing.T) {
	t.Skip("跳过AssignIssue测试，因为它依赖于外部GitCode API")
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		number   int
		username string
		want     *Issue
		wantErr  bool
	}{
		{"assign", 4, "nanjunjie", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := AssignIssue(t.Context(), tt.number, tt.username)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("AssignIssue() failed: %v", gotErr)
				}
				return
			}

			if tt.wantErr {
				t.Fatal("AssignIssue() succeeded unexpectedly")
			}
			if got.Assignee == nil {
				t.Errorf("AssignIssue() = %v, want %v", got, tt.want)
			}
		})
	}
}
