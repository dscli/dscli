//go:build !windows

package processutil

import "syscall"

// IsAlive checks whether a process with the given PID is currently running.
//
// On Unix-like systems, this sends signal 0, which is a no-op that
// checks process existence without actually sending a signal.
func IsAlive(pid int) bool {
	return syscall.Kill(pid, syscall.Signal(0)) == nil
}
