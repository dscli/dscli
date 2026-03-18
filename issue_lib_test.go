package main

import (
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
		{"Close", 14, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := CloseIssue(t.Context(), tt.number)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("CloseIssue() failed: %v", gotErr)
				}
				return
			}
			t.Log(got)
			if tt.wantErr {
				t.Fatal("CloseIssue() succeeded unexpectedly")
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
		{"show issue", 5, false},
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
			t.Log(got.ID)
		})
	}
}
