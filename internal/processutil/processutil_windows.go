//go:build windows

package processutil

import "syscall"

// IsAlive checks whether a process with the given PID is currently running.
//
// On Windows, this uses OpenProcess with minimal access rights to verify
// that the process exists without terminating it.
func IsAlive(pid int) bool {
	const (
		processQueryInformation = 0x0400
		processVMRead           = 0x0010
	)
	h, err := syscall.OpenProcess(
		processQueryInformation|processVMRead,
		false,
		uint32(pid),
	)
	if err != nil {
		return false
	}
	syscall.CloseHandle(h)
	return true
}
