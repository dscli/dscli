package git

import "testing"

func Test_hasStagedUnstagedChanges(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		statusOut string
		staged    bool
		unstaged  bool
	}{
		{"unstaged", ` M chat.go
 M git_diff.go
 M git_test.go
`, false, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staged, unstaged := hasStagedUnstagedChanges(tt.statusOut)
			if staged != tt.staged {
				t.Errorf("hasStagedUnstagedChanges() = %v, want %v", staged, tt.staged)
			}
			if unstaged != tt.unstaged {
				t.Errorf("hasStagedUnstagedChanges() = %v, want %v", unstaged, tt.unstaged)
			}
		})
	}
}
