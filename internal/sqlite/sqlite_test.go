package sqlite

import (
	"os"
	"testing"
)

func TestOpen(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		wantErr bool
	}{
		{"Normal test", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp("", "test_*_open.db")
			if err != nil {
				t.Fatal(err)
			}
			dbPath := f.Name()
			got, gotErr := Open(dbPath)
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("Open() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("Open() succeeded unexpectedly")
			}
			if got == nil {
				t.Errorf("Open() = %v", got)
			}
		})
	}
}
