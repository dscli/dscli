//go:build windows

package processutil

import "os/exec"

// detachCmd is a no-op on Windows: there is no SIGHUP/process-group
// lifecycle to escape, and the display command (emacsclient/emacs) is
// a Unix path anyway.
func detachCmd(_ *exec.Cmd) {}
