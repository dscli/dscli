package emacsutil

import (
	"os"
	"path/filepath"
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
const probeClient = `#!/bin/sh
for arg in "$@"; do
	if [ "$arg" = "--eval" ]; then
		exit "${FAKE_SERVER_EXIT:-1}"
	fi
done
exit 0
`

// TestDetect exercises mode detection with fake emacs/emacsclient
// binaries on PATH.  The fake emacsclient reports server state via
// FAKE_SERVER_EXIT for --eval probes; the fake emacs just exists so the
// standalone fallback branch can be reached.
func TestDetect(t *testing.T) {
	const fakeEmacs = "#!/bin/sh\nexit 0\n"

	tests := []struct {
		name   string
		has    []string // fake binaries to put on PATH
		server string   // FAKE_SERVER_EXIT for the probe (empty = down)
		want   Mode
	}{
		{name: "no emacs tools", has: nil, want: ModeNone},
		{name: "only emacsclient with server", has: []string{"emacsclient"}, server: "0", want: ModeClientServer},
		{name: "only emacsclient without server", has: []string{"emacsclient"}, want: ModeClientOnly},
		{name: "emacs and emacsclient with server", has: []string{"emacs", "emacsclient"}, server: "0", want: ModeClientServer},
		{name: "emacs and emacsclient without server", has: []string{"emacs", "emacsclient"}, want: ModeStandalone},
		{name: "emacs only", has: []string{"emacs"}, want: ModeStandalone},
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

			if got := Detect(); got != tt.want {
				t.Errorf("Detect() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestServerRunning checks the emacsclient probe: exit 0 means the
// server socket is reachable, anything else (missing binary included)
// means it is not.
func TestServerRunning(t *testing.T) {
	t.Run("server up", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeScript(t, dir, "emacsclient", probeClient)
		t.Setenv("PATH", dir)
		t.Setenv("FAKE_SERVER_EXIT", "0")
		if !ServerRunning() {
			t.Error("ServerRunning() = false, want true")
		}
	})
	t.Run("server down", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeScript(t, dir, "emacsclient", probeClient)
		t.Setenv("PATH", dir)
		if ServerRunning() {
			t.Error("ServerRunning() = true, want false")
		}
	})
	t.Run("no emacsclient", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if ServerRunning() {
			t.Error("ServerRunning() = true, want false")
		}
	})
}

// TestHasExecutable checks PATH lookup behavior.
func TestHasExecutable(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeScript(t, dir, "emacsclient", "#!/bin/sh\nexit 0\n")
		t.Setenv("PATH", dir)
		if !HasExecutable("emacsclient") {
			t.Error("HasExecutable(emacsclient) = false, want true")
		}
	})
	t.Run("missing", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		if HasExecutable("emacsclient") {
			t.Error("HasExecutable(emacsclient) = true, want false")
		}
	})
}
