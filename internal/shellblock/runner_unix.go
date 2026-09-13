//go:build !windows

package shellblock

import (
	"os/exec"
	"syscall"
)

// prepareProcessGroup puts the script into its own process group so a
// timeout can kill the whole tree (bash, the tools it spawned, and their
// children - `go test`'s compilers, `make`'s children).
func prepareProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the script's process group (the negative PID
// targets every process in the group).
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
