package wakeup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
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

// TestDetectDisplayCommandNoEnvHandoff guards the wakeup handoff contract:
// the project path must travel via the wakeup-project file, never through
// a prefix-assigned environment variable (emacsclient does not pass those
// into a running server's -e evaluation environment) and never spliced
// into a Lisp string literal.
func TestDetectDisplayCommandNoEnvHandoff(t *testing.T) {
	dir := t.TempDir()
	writeFakeScript(t, dir, "emacs", "#!/bin/sh\nexit 0\n")
	writeFakeScript(t, dir, "emacsclient", probeClient)
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_SERVER_EXIT", "0")

	got := detectDisplayCommand()
	if got == "" {
		t.Fatal("detectDisplayCommand() = empty, want emacsclient template")
	}
	if strings.Contains(got, "DSCLI_WAKEUP_PROJECT") || strings.Contains(got, "getenv") {
		t.Errorf("template must not use env-var handoff, got: %q", got)
	}
	if strings.Contains(got, "$1") {
		t.Errorf("template must not splice $1 into Lisp, got: %q", got)
	}
}

// TestWriteWakeupProjectFile verifies the handoff file is created with the
// project path and that a missing parent directory is created on demand.
func TestWriteWakeupProjectFile(t *testing.T) {
	dir := t.TempDir()
	orig := wakeupProjectFile
	wakeupProjectFile = func() string {
		return filepath.Join(dir, "nested", "wakeup-project")
	}
	t.Cleanup(func() { wakeupProjectFile = orig })

	project := "/home/nanjj/.local/src/github.com/dscli/dscli"
	if err := writeWakeupProjectFile(project); err != nil {
		t.Fatalf("writeWakeupProjectFile() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "nested", "wakeup-project"))
	if err != nil {
		t.Fatalf("read handoff file: %v", err)
	}
	if string(data) != project {
		t.Errorf("handoff file = %q, want %q", data, project)
	}

	// Overwrite on the next wakeup.
	if err := writeWakeupProjectFile("/other/project"); err != nil {
		t.Fatalf("second write error = %v", err)
	}
	data, err = os.ReadFile(filepath.Join(dir, "nested", "wakeup-project"))
	if err != nil {
		t.Fatalf("re-read handoff file: %v", err)
	}
	if string(data) != "/other/project" {
		t.Errorf("handoff file after overwrite = %q, want %q", data, "/other/project")
	}
}

// TestWriteWakeupProjectFileRejectsControlChars ensures a project path
// with newline or NUL cannot corrupt the Emacs-side file read.
func TestWriteWakeupProjectFileRejectsControlChars(t *testing.T) {
	dir := t.TempDir()
	orig := wakeupProjectFile
	wakeupProjectFile = func() string {
		return filepath.Join(dir, "wakeup-project")
	}
	t.Cleanup(func() { wakeupProjectFile = orig })

	for _, bad := range []string{"/proj\nect", "/proj\x00ect"} {
		if err := writeWakeupProjectFile(bad); err == nil {
			t.Errorf("writeWakeupProjectFile(%q) error = nil, want control-char rejection", bad)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "wakeup-project")); !os.IsNotExist(err) {
		t.Errorf("handoff file should not be created for invalid paths, stat err = %v", err)
	}
}

// TestWakeupProjectFileRestored ensures the default handoff path lives in
// the global config directory, so Emacs and dscli agree on its location.
func TestWakeupProjectFileRestored(t *testing.T) {
	if got := wakeupProjectFile(); got != filepath.Join(config.ConfigDir, "wakeup-project") {
		t.Errorf("wakeupProjectFile() = %q, want %q", got, filepath.Join(config.ConfigDir, "wakeup-project"))
	}
}
