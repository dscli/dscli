//go:build windows

package ai

import "os/exec"

// detachCmd is a no-op on Windows: there is no SIGHUP/process-group
// lifecycle to escape, and the sh -c based display command is a Unix
// path anyway.
func detachCmd(_ *exec.Cmd) {}
