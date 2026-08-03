package ai

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
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
		want   []string // expected argv, nil for none
	}{
		{name: "no emacs tools", has: nil, want: nil},
		{name: "only emacsclient", has: []string{"emacsclient"}, want: []string{"emacsclient", "-n", "-c", "-e", "(dscli--send-message-raw)"}},
		{name: "emacs without server", has: []string{"emacs", "emacsclient"}, server: "1", want: []string{"emacs", "--eval", "(dscli--send-message-raw)"}},
		{name: "emacs with server", has: []string{"emacs", "emacsclient"}, server: "0", want: []string{"emacsclient", "-n", "-c", "-e", "(dscli--send-message-raw)"}},
		{name: "emacs only", has: []string{"emacs"}, server: "1", want: []string{"emacs", "--eval", "(dscli--send-message-raw)"}},
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
			if !slices.Equal(got, tt.want) {
				t.Errorf("detectDisplayCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestDetectDisplayCommandNoEnvHandoff guards the wakeup handoff contract:
// the project path travels as the command's working directory
// (runDisplayCommand sets cmd.Dir — emacsclient -e evaluates with
// default-directory following the client's cwd), never via a
// prefix-assigned environment variable (emacsclient does not pass those
// into a running server's -e evaluation environment) and never spliced
// into a Lisp string literal.
func TestDetectDisplayCommandNoEnvHandoff(t *testing.T) {
	dir := t.TempDir()
	writeFakeScript(t, dir, "emacs", "#!/bin/sh\nexit 0\n")
	writeFakeScript(t, dir, "emacsclient", probeClient)
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_SERVER_EXIT", "0")

	got := detectDisplayCommand()
	if len(got) == 0 {
		t.Fatal("detectDisplayCommand() = empty, want emacsclient command")
	}
	joined := strings.Join(got, " ")
	if strings.Contains(joined, "DSCLI_WAKEUP_PROJECT") || strings.Contains(joined, "getenv") {
		t.Errorf("command must not use env-var handoff, got: %v", got)
	}
	if strings.Contains(joined, "$1") {
		t.Errorf("command must not reference a positional project arg, got: %v", got)
	}
}

// TestRunDisplayCommandUsesProjectDir verifies the display command runs
// with the target project as its working directory — that is the handoff
// channel to the Emacs side (emacsclient -e evaluates with
// default-directory following the client's cwd), replacing the old
// handoff file.  A shell `pwd` is captured to prove the cwd.
func TestRunDisplayCommandUsesProjectDir(t *testing.T) {
	project := t.TempDir()
	out := filepath.Join(t.TempDir(), "cwd.txt")
	script := filepath.Join(t.TempDir(), "capture-cwd.sh")
	// The script records its own working directory into the marker file
	// passed as $1.  runDisplayCommand executes it directly (no shell
	// wrapper), so the captured cwd proves cmd.Dir == project.
	if err := os.WriteFile(script, []byte("#!/bin/sh\npwd > \"$1\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	runDisplayCommand(project, script, out)

	// The command starts async children; poll for the capture file.
	deadline := time.Now().Add(2 * time.Second)
	var data []byte
	for {
		var err error
		data, err = os.ReadFile(out)
		if err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("capture file %s not written: %v", out, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := strings.TrimSpace(string(data)); got != project {
		t.Errorf("command cwd = %q, want %q", got, project)
	}
}
