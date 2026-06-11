//go:build unix

package lockfile

import "syscall"

// flockExNb 非阻塞排他锁。
func flockExNb(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB)
}

// flockEx 阻塞排他锁。
func flockEx(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_EX)
}

// flockUn 解锁。
func flockUn(fd uintptr) error {
	return syscall.Flock(int(fd), syscall.LOCK_UN)
}

// isBlocking 判断错误是否表示锁被其他进程持有。
func isBlocking(err error) bool {
	return err == syscall.EWOULDBLOCK || err == syscall.EAGAIN
}
