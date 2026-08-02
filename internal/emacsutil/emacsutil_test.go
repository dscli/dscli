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

// TestDetect exercises mode detection with fake emacs/emacsclient
// binaries on PATH.  The fake emacs exits with FAKE_SERVER_EXIT for the
// --batch server probe (0 = server up) and 0 for a normal launch.
func TestDetect(t *testing.T) {
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
		want   Mode
	}{
		{name: "no emacs tools", has: nil, want: ModeNone},
		{name: "only emacsclient", has: []string{"emacsclient"}, want: ModeClientOnly},
		{name: "emacs without server", has: []string{"emacs", "emacsclient"}, server: "1", want: ModeStandalone},
		{name: "emacs with server", has: []string{"emacs", "emacsclient"}, server: "0", want: ModeClientServer},
		{name: "emacs only", has: []string{"emacs"}, server: "1", want: ModeStandalone},
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

// TestServerRunning checks that the probe honors a batch-mode-only emacs
// that reports server state via its exit code.
func TestServerRunning(t *testing.T) {
	t.Run("server up", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeScript(t, dir, "emacs", "#!/bin/sh\nexit 0\n")
		t.Setenv("PATH", dir)
		if !ServerRunning() {
			t.Error("ServerRunning() = false, want true")
		}
	})
	t.Run("server down", func(t *testing.T) {
		dir := t.TempDir()
		writeFakeScript(t, dir, "emacs", "#!/bin/sh\nexit 1\n")
		t.Setenv("PATH", dir)
		if ServerRunning() {
			t.Error("ServerRunning() = true, want false")
		}
	})
	t.Run("no emacs", func(t *testing.T) {
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

// TestDetectNoServerClientOnly guards the last-resort branch: emacsclient
// alone yields ModeClientOnly regardless of server state (we cannot probe
// without an emacs binary).
func TestDetectNoServerClientOnly(t *testing.T) {
	dir := t.TempDir()
	writeFakeScript(t, dir, "emacsclient", "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", dir)
	if got := Detect(); got != ModeClientOnly {
		t.Errorf("Detect() = %v, want ModeClientOnly", got)
	}
}
