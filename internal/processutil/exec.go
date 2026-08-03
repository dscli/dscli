package processutil

import (
	"fmt"
	"os/exec"
)

// RunCommandBackground executes a command with dir as its working directory,
// fire-and-forget and detached from the caller's session.
//
// The command goes straight to exec.Command as (name, args...) — there is
// no shell layer, so nothing is reinterpreted: the argv is used verbatim,
// and no path ever travels through a shell.
//
// The working directory is the handoff channel to the launched process:
// the caller sets cmd.Dir to the target directory, and the child (e.g.
// emacsclient -e, whose eval buffer's default-directory follows the
// client's cwd) reads the target from its own cwd.  No env var, no
// handoff file, no shared state — concurrent launches for different
// targets cannot interfere.
//
// It returns an error only when the command cannot be started.  Once
// started, the child is detached (own session, no controlling terminal)
// and reaped in the background; later errors are intentionally ignored.
func RunCommandBackground(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	// The launched command must outlive the caller.  Without detachment it
	// inherits the caller's process group and controlling terminal, so
	// closing the terminal that hosts the caller (or Ctrl+C on it) sends
	// SIGHUP/SIGINT to the whole foreground process group - taking the
	// freshly launched display down with the caller.  Setsid is stronger
	// than nohup: it starts a new session with no controlling terminal,
	// so terminal-close SIGHUP and process-group signals never reach the
	// child.  A GUI Emacs (DISPLAY set) needs no tty; in a tty-only
	// environment the display dies with the terminal anyway.
	detachCmd(cmd)

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start background command %q: %w", name, err)
	}
	// Detach: reap the child in background.
	go cmd.Wait()
	return nil
}
