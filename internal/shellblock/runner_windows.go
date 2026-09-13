//go:build windows

package shellblock

import "os/exec"

// prepareProcessGroup is a no-op on Windows: process trees are not managed
// through SysProcAttr there.
func prepareProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup kills the script process; the OS reaps its children with
// the job when it exits.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
