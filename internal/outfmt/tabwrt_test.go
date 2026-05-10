package outfmt

import (
	"bytes"
	"strings"
	"testing"
)

func TestTabwrtOutput(t *testing.T) {
	// Save and restore global state
	origWriter := outputWriter
	origMode := outputMode
	defer func() {
		outputWriter = origWriter
		SetOutputMode(origMode)
	}()

	t.Run("content routed through Print", func(t *testing.T) {
		var buf bytes.Buffer
		outputWriter = &buf
		SetOutputMode("markdown")

		wrt := NewTabwrt()
		wrt.Println("ID", "42")
		wrt.Println("Role", "user")
		wrt.Flush()

		got := buf.String()
		if !strings.Contains(got, "ID") || !strings.Contains(got, "42") ||
			!strings.Contains(got, "Role") || !strings.Contains(got, "user") {
			t.Errorf("missing expected content: %q", got)
		}
		if strings.Count(got, "\n") < 2 {
			t.Errorf("expected at least 2 newlines: %q", got)
		}
	})

	t.Run("org mode also outputs content", func(t *testing.T) {
		var buf bytes.Buffer
		outputWriter = &buf
		SetOutputMode("org")

		wrt := NewTabwrt()
		wrt.Println("ID", "42")
		wrt.Println("Role", "user")
		wrt.Flush()

		got := buf.String()
		if !strings.Contains(got, "ID") || !strings.Contains(got, "42") ||
			!strings.Contains(got, "Role") || !strings.Contains(got, "user") {
			t.Errorf("missing expected content: %q", got)
		}
		if strings.Count(got, "\n") < 2 {
			t.Errorf("expected at least 2 newlines: %q", got)
		}
	})
}
