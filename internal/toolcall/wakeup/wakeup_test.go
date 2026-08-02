package wakeup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeScript creates an executable fake command in dir.
func writeFakeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestDetectDisplayCommand exercises display-command detection with fake
// emacs/emacsclient binaries on PATH.  The fake emacs exits with
// FAKE_SERVER_EXIT for the --batch server probe (0 = server up) and 0
// for a normal launch.
func TestDetectDisplayCommand(t *testing.T) {
	const fakeEmacs = `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "--batch" ]; then
		exit "${FAKE_SERVER_EXIT:-1}"
	fi
done
exit 0
`
	const fakeClient = "#!/bin/sh\nexit 0\n"

	tests := []struct {
		name   string
		has    []string // fake binaries to put on PATH
		server string   // FAKE_SERVER_EXIT for the probe (empty = not running)
		want   string   // substring the returned template must contain
	}{
		{name: "no emacs tools", has: nil, want: ""},
		{name: "only emacsclient", has: []string{"emacsclient"}, want: "emacsclient -n -c"},
		{name: "emacs without server", has: []string{"emacs", "emacsclient"}, server: "1", want: "emacs --eval"},
		{name: "emacs with server", has: []string{"emacs", "emacsclient"}, server: "0", want: "emacsclient -n -c"},
		{name: "emacs only", has: []string{"emacs"}, server: "1", want: "emacs --eval"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, name := range tt.has {
				if name == "emacs" {
					writeFakeScript(t, dir, name, fakeEmacs)
				} else {
					writeFakeScript(t, dir, name, fakeClient)
				}
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			if tt.server != "" {
				t.Setenv("FAKE_SERVER_EXIT", tt.server)
			}

			got := detectDisplayCommand()
			if !strings.Contains(got, tt.want) {
				t.Errorf("detectDisplayCommand() = %q, want containing %q", got, tt.want)
			}
		})
	}
}
