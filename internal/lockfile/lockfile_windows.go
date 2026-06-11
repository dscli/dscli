//go:build windows

package lockfile

import (
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procLockFileEx     = kernel32.NewProc("LockFileEx")
	procUnlockFileEx   = kernel32.NewProc("UnlockFileEx")
)

const (
	lockfileExclusiveLock    = 2
	lockfileFailImmediately  = 1

	// Windows 系统错误码（kernel32 错误，不在 syscall 包中导出）
	windowsErrorLockViolation = 33
	windowsErrorIOPending     = 997
)

// flockExNb 非阻塞排他锁（Windows 用 LockFileEx）。
func flockExNb(fd uintptr) error {
	var overlapped syscall.Overlapped
	r, _, err := procLockFileEx.Call(
		fd,
		lockfileExclusiveLock|lockfileFailImmediately,
		0, // reserved
		1, // low bytes of length
		0, // high bytes of length
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r == 0 {
		if errno, ok := err.(syscall.Errno); ok &&
			(errno == windowsErrorLockViolation || errno == windowsErrorIOPending) {
			return syscall.EWOULDBLOCK
		}
		return err
	}
	return nil
}

// flockEx 阻塞排他锁。
func flockEx(fd uintptr) error {
	var overlapped syscall.Overlapped
	r, _, err := procLockFileEx.Call(
		fd,
		lockfileExclusiveLock,
		0, // reserved
		1, // low bytes of length
		0, // high bytes of length
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r == 0 {
		return err
	}
	return nil
}

// flockUn 解锁。
func flockUn(fd uintptr) error {
	var overlapped syscall.Overlapped
	r, _, err := procUnlockFileEx.Call(
		fd,
		0, // reserved
		1, // low bytes of length
		0, // high bytes of length
		uintptr(unsafe.Pointer(&overlapped)),
	)
	if r == 0 {
		return err
	}
	return nil
}

// isBlocking 判断错误是否表示锁被其他进程持有。
func isBlocking(err error) bool {
	return err == syscall.EWOULDBLOCK
}
