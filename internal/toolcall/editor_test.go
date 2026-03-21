package toolcall

import (
	"os"
	"testing"
)

func TestOpenEditor(t *testing.T) {
	t.Skip()
	tests := []struct {
		editor         string
		initialContent string
	}{
		{"emacsclient", "随便输入什么但不要留空，保存退出"},
	}
	for _, tt := range tests {
		t.Run(tt.editor, func(t *testing.T) {
			os.Setenv("EDITOR", tt.editor)
			defer os.Unsetenv("EDITOR")
			got, err := OpenEditor(t.Context(), tt.initialContent)
			if err != nil {
				t.Fatal(err)
			}
			if got == "" {
				t.Fatal()
			}
		})
	}
}
