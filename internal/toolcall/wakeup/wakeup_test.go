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

// probeClient is a fake emacsclient that honors FAKE_SERVER_EXIT for
// --eval probes (0 = server up, 1/empty = down) and exits 0 otherwise.
// This mirrors internal/emacsutil's probe semantics.
const probeClient = `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "--eval" ]; then
		exit "${FAKE_SERVER_EXIT:-1}"
	fi
done
exit 0
`

// TestDetectDisplayCommand exercises display-command detection with fake
// emacs/emacsclient binaries on PATH.  The fake emacsclient reports
// server state via FAKE_SERVER_EXIT; the fake emacs just exists so the
// standalone fallback branch can be reached.
func TestDetectDisplayCommand(t *testing.T) {
	const fakeEmacs = "#!/bin/sh\nexit 0\n"

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
					writeFakeScript(t, dir, name, probeClient)
				}
			}
			t.Setenv("PATH", dir)
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
