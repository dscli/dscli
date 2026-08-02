package editor

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeFakeScript creates an executable fake command in dir.
func writeFakeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestResolveEditor exercises emacsclient normalization:
//
//   - When the editor is emacsclient but no Emacs server is running
//     (emacsutil.ModeStandalone), fall back to a standalone emacs and
//     drop emacsclient-specific flags (-c would be parsed by emacs as
//     --no-site-file).
//   - When a server is running, keep emacsclient as configured.
//   - Non-emacs editors pass through unchanged.
func TestResolveEditor(t *testing.T) {
	const fakeEmacs = "#!/bin/sh\nexit 0\n"
	// fakeClient honors FAKE_SERVER_EXIT for --eval probes (0 = server up,
	// 1/empty = down), mirroring internal/emacsutil's probe semantics.
	const fakeClient = `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "--eval" ]; then
		exit "${FAKE_SERVER_EXIT:-1}"
	fi
done
exit 0
`

	setupPATH := func(t *testing.T, emacs, emacsclient bool, serverExit string) {
		t.Helper()
		dir := t.TempDir()
		if emacs {
			writeFakeScript(t, dir, "emacs", fakeEmacs)
		}
		if emacsclient {
			writeFakeScript(t, dir, "emacsclient", fakeClient)
		}
		t.Setenv("PATH", dir)
		if serverExit != "" {
			t.Setenv("FAKE_SERVER_EXIT", serverExit)
		}
	}

	tests := []struct {
		name   string
		editor string
		env    func(*testing.T)
		wantN  string
		wantA  []string
	}{
		{
			name:   "emacsclient without server falls back to emacs",
			editor: "emacsclient -c",
			env:    func(t *testing.T) { setupPATH(t, true, true, "1") },
			wantN:  "emacs",
			wantA:  nil,
		},
		{
			name:   "absolute emacsclient path without server falls back",
			editor: "/usr/bin/emacsclient -c",
			env:    func(t *testing.T) { setupPATH(t, true, true, "1") },
			wantN:  "emacs",
			wantA:  nil,
		},
		{
			name:   "emacsclient with server is kept",
			editor: "emacsclient -c",
			env:    func(t *testing.T) { setupPATH(t, true, true, "0") },
			wantN:  "emacsclient",
			wantA:  []string{"-c"},
		},
		{
			name:   "non-emacs editor passes through",
			editor: "vi -u NONE",
			env:    func(t *testing.T) {},
			wantN:  "vi",
			wantA:  []string{"-u", "NONE"},
		},
		{
			name:   "emacs editor passes through",
			editor: "emacs -nw",
			env:    func(t *testing.T) {},
			wantN:  "emacs",
			wantA:  []string{"-nw"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.env(t)
			gotN, gotA := resolveEditor(tt.editor)
			if gotN != tt.wantN || !reflect.DeepEqual(gotA, tt.wantA) {
				t.Errorf("resolveEditor(%q) = (%q, %v), want (%q, %v)",
					tt.editor, gotN, gotA, tt.wantN, tt.wantA)
			}
		})
	}
}
