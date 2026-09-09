package ai

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/toolcall"
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
		role   string   // optional role argument
		want   []string // expected argv, nil for none
	}{
		{name: "no emacs tools", has: nil, want: nil},
		{name: "only emacsclient", has: []string{"emacsclient"}, want: []string{"emacsclient", "-n", "-c", "-e", "(dscli--send-message-raw)"}},
		{name: "emacs without server", has: []string{"emacs", "emacsclient"}, server: "1", want: []string{"emacs", "--no-splash", "--eval", "(dscli--send-message-raw)"}},
		{name: "emacs with server", has: []string{"emacs", "emacsclient"}, server: "0", want: []string{"emacsclient", "-n", "-c", "-e", "(dscli--send-message-raw)"}},
		{name: "emacs only", has: []string{"emacs"}, server: "1", want: []string{"emacs", "--no-splash", "--eval", "(dscli--send-message-raw)"}},
		{
			name: "role via server", has: []string{"emacs", "emacsclient"}, server: "0", role: "dev",
			want: []string{"emacsclient", "-n", "-c", "-e", `(dscli--send-message-raw nil "dev")`},
		},
		{
			name: "role standalone", has: []string{"emacs"}, server: "1", role: "architect",
			want: []string{"emacs", "--no-splash", "--eval", `(dscli--send-message-raw nil "architect")`},
		},
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

			got := detectDisplayCommand(tt.role)
			if !slices.Equal(got, tt.want) {
				t.Errorf("detectDisplayCommand(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

// TestDetectDisplayCommandNoEnvHandoff guards the wakeup handoff contract:
// the project path travels as the command's working directory
// (RunCommandBackground sets cmd.Dir — emacsclient -e evaluates with
// default-directory following the client's cwd), never via a
// prefix-assigned environment variable (emacsclient does not pass those
// into a running server's -e evaluation environment) and never spliced
// into a Lisp string literal.  The role is the one piece that must travel
// inside the form, because the environment cannot carry it.
func TestDetectDisplayCommandNoEnvHandoff(t *testing.T) {
	dir := t.TempDir()
	writeFakeScript(t, dir, "emacs", "#!/bin/sh\nexit 0\n")
	writeFakeScript(t, dir, "emacsclient", probeClient)
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_SERVER_EXIT", "0")

	for _, role := range []string{"", "dev"} {
		got := detectDisplayCommand(role)
		if len(got) == 0 {
			t.Fatalf("detectDisplayCommand(%q) = empty, want emacsclient command", role)
		}
		joined := strings.Join(got, " ")
		if strings.Contains(joined, "DSCLI_WAKEUP_PROJECT") || strings.Contains(joined, "getenv") {
			t.Errorf("command must not use env-var handoff, got: %v", got)
		}
		if strings.Contains(joined, "$1") {
			t.Errorf("command must not reference a positional project arg, got: %v", got)
		}
		if role == "" {
			if strings.Contains(joined, `"dev"`) {
				t.Errorf("role-less command must not carry a role, got: %v", got)
			}
			continue
		}
		if !strings.Contains(joined, `"`+role+`"`) {
			t.Errorf("role must travel as a Lisp string literal, got: %v", got)
		}
	}
}

// TestWakeupLispForm pins the Go→Emacs Lisp contract: no role keeps the
// historical no-argument call; a role is passed as the second argument
// with an explicit nil project root (the path still travels as cwd).
func TestWakeupLispForm(t *testing.T) {
	if got, want := wakeupLispForm(""), "(dscli--send-message-raw)"; got != want {
		t.Errorf("wakeupLispForm(\"\") = %q, want %q", got, want)
	}
	if got, want := wakeupLispForm("dev"), `(dscli--send-message-raw nil "dev")`; got != want {
		t.Errorf("wakeupLispForm(\"dev\") = %q, want %q", got, want)
	}
}

// TestNormalizeWakeupRole covers the optional role argument: empty means
// "use the CLI default", known names are canonicalized, and unknown names
// are rejected: `dscli chat --role` would silently fall back to the dev
// profile and template, starting the wrong persona.
func TestNormalizeWakeupRole(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr bool
	}{
		{name: "empty leaves the CLI default", in: ""},
		{name: "blank leaves the CLI default", in: "   "},
		{name: "trims and lowercases", in: "  Dev ", want: "dev"},
		{name: "architect", in: "architect", want: "architect"},
		{name: "review", in: "review", want: "review"},
		{name: "unknown role rejected", in: "developer", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeWakeupRole(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeWakeupRole(%q) = %q, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeWakeupRole(%q): %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("normalizeWakeupRole(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}

	// Every built-in role must round-trip: the validator and the role
	// registry cannot drift apart.
	for _, role := range roles.Names() {
		got, err := normalizeWakeupRole(role)
		if err != nil || got != role {
			t.Errorf("normalizeWakeupRole(%q) = %q, %v; want %q, nil", role, got, err, role)
		}
	}
}

// TestHandleWakeupRejectsUnknownRole guards the fail-loud policy: an unknown
// role is rejected before any delivery side effect (the check runs before
// the project is touched, so no database access is involved).
func TestHandleWakeupRejectsUnknownRole(t *testing.T) {
	_, _, err := handleWakeup(t.Context(), toolcall.ToolArgs{
		"project": "/nonexistent-project",
		"role":    "developer",
	})
	if err == nil {
		t.Fatal("handleWakeup with an unknown role: want error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid role") {
		t.Errorf("error = %v, want invalid-role error", err)
	}
}
