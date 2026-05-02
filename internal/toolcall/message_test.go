package toolcall

import "testing"

func TestSaveMessages(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		msgs    []Message
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	ctx := t.Context()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotErr := SaveMessages(ctx, tt.msgs...)
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
