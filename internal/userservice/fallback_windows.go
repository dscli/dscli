//go:build windows

package userservice

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// daemonizeCmd sets SysProcAttr so the child process survives its parent.
// On Windows this means CREATE_NEW_PROCESS_GROUP and hiding the console window.
func daemonizeCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
		HideWindow:    true,
	}
}

// isProcessAlive reports whether the process with the given pid is alive.
// On Windows this uses tasklist to check for the PID.
func isProcessAlive(pid int) bool {
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(string(out), strconv.Itoa(pid))
}

// killProcess forcibly terminates the process with the given pid.
// On Windows this uses taskkill /F.
func killProcess(pid int) error {
	cmd := exec.Command("taskkill", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}
