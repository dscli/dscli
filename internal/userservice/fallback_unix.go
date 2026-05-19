//go:build !windows

package userservice

import (
	"os"
	"os/exec"
	"syscall"
)

// daemonizeCmd sets SysProcAttr so the child process survives its parent.
// On Unix this means creating a new session (setsid).
func daemonizeCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}

// isProcessAlive reports whether the process with the given pid is alive.
// Uses signal 0 (null signal) which the kernel delivers without side-effects;
// ESRCH means the process does not exist.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

// killProcess sends SIGTERM to the given pid.
func killProcess(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Signal(syscall.SIGTERM)
}
