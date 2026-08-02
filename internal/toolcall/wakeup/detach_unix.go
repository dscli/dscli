//go:build !windows

package wakeup

import (
	"os/exec"
	"syscall"
)

// detachCmd makes the child process survive its parent: it starts in a
// new session (setsid) with no controlling terminal, so terminal-close
// SIGHUP and process-group signals cannot reach it.  See
// runDisplayCommand for why the display command needs this.
func detachCmd(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setsid = true
}
