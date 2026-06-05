// Package lockfile 提供项目级与全局级文件锁。
//
// 基于 flock(2) 实现，进程退出或文件关闭时内核自动释放锁，
// 无残留问题。Linux 与 macOS 均支持。
//
// # 用法
//
//	// 项目级锁
//	lk, ok, err := lockfile.TryLockLocal()
//	if ok {
//		defer lk.Close()
//		// 持有锁，执行主逻辑
//	}
//
//	// 全局锁
//	lk, ok, err := lockfile.TryLockGlobal()
package lockfile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/context"
)

// Lock 表示一个排他文件锁。
// 调用方在不再需要锁时必须调用 Close 释放。
type Lock struct {
	file *os.File
	path string
}

// tryLock 尝试以非阻塞方式获取排他锁。
//
// 参数：
//   - configDir: 配置目录
//
// 返回值：
//   - lock: 持锁对象，仅在 acquired==true 时有效
//   - acquired: true 表示成功获取锁，false 表示其他进程已持有
//   - err: 系统错误（目录创建失败、文件打开失败等）
func tryLock(configDir string) (*Lock, bool, error) {
	path := filepath.Join(configDir, "locks", "dscli.lock")
	// 确保锁文件目录存在
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, false, fmt.Errorf("lockfile: mkdir %s: %w", dir, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, false, fmt.Errorf("lockfile: open %s: %w", path, err)
	}

	// 非阻塞排他锁：已被持有时返回 EWOULDBLOCK
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			// 锁已被持有。若持有者是当前进程的父进程
			// （code review / ask expert 由主 chat 进程 fork），
			// 视为已获取锁，允许子进程继续执行。
			if PID(configDir) == os.Getppid() {
				return nil, true, nil
			}
			return nil, false, nil // 已被其他进程持有，正常情况
		}
		return nil, false, fmt.Errorf("lockfile: flock %s: %w", path, err)
	}

	// 写入 PID 方便排查
	f.Truncate(0)
	f.Seek(0, 0)
	fmt.Fprintf(f, "%d\n", os.Getpid())

	return &Lock{file: f, path: path}, true, nil
}

// TryLockLocal 获取项目级排他锁。
// 使用 context.ProjectRoot 确定项目根目录，
// 锁文件位于 <projectDir>/.dscli/locks/dscli.lock。
func TryLockLocal() (*Lock, bool, error) {
	return tryLock(filepath.Join(context.ProjectRoot, ".dscli"))
}

// TryLockGlobal 获取全局级排他锁。
// 锁文件位于 ~/.dscli/locks/dscli.lock。
func TryLockGlobal() (*Lock, bool, error) {
	return tryLock(config.ConfigDir)
}

// LockDB 获取数据库排他锁（阻塞等待）。
//
// 锁文件位于 ~/.dscli/locks/sqlite.db.lock，与 chat 进程锁
// (dscli.lock) 独立。调用方负责 Close 释放；进程退出时
// 内核自动释放。
//
// 用于消除多进程并发访问 sqlite.db 时的 SQLITE_BUSY 错误。
// LockDB 获取指定数据库文件名的文件锁（阻塞排他锁）。
//
// dbName 如 "sqlite.db" 或 "wechat.db"，
// 锁文件路径为 ~/.dscli/locks/<dbName>.lock。
func LockDB(dbName string) (*Lock, error) {
	dir := filepath.Join(config.ConfigDir, "locks")
	path := filepath.Join(dir, dbName+".lock")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("lockfile: mkdir %s: %w", dir, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("lockfile: open %s: %w", path, err)
	}

	// 阻塞排他锁 — 等待直到获取成功
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("lockfile: flock %s: %w", path, err)
	}

	// 写入 PID 方便排查
	f.Truncate(0)
	f.Seek(0, 0)
	fmt.Fprintf(f, "%d\n", os.Getpid())

	return &Lock{file: f, path: path}, nil
}

// Close 释放锁并关闭文件。
func (l *Lock) Close() error {
	if l.file == nil {
		return nil
	}
	// 显式解锁（close 也会自动释放，但显式做更清晰）
	syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	err := l.file.Close()
	l.file = nil
	return err
}

// PID 读取锁文件中记录的进程 PID。
// 仅在锁未被持有时有意义（用于调试，判断谁持有了锁）。
// 返回 0 表示无法读取（文件不存在或格式错误）。
func PID(configDir string) int {
	path := filepath.Join(configDir, "locks", "dscli.lock")
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	return pid
}
