//go:build windows

package editor

import (
	"errors"
	"os"
)

// openTTY 在 Windows 上目前返回错误，触发安全降级到 os.Stdin/os.Stdout/os.Stderr。
// Windows 上控制终端的等价物是 CONIN$（控制台输入）和 CONOUT$（控制台输出），
// 但两者是分开的设备，不能像 Unix /dev/tty 那样用一个文件描述符同时读写。
//
// 未来如需支持 Windows 真实终端重定向，可改为：
//
//	func openTTY() (*os.File, error) {
//	    return os.OpenFile("CONIN$", os.O_RDWR, 0)
//	}
//
// 注意：Windows 上可能需要分别处理 stdin 和 stdout，这会改变调用方的接口语义。
// 目前保持降级策略，待有真实用户反馈后再实现完整方案。
func openTTY() (*os.File, error) {
	return nil, errors.New("tty redirection not yet implemented on Windows, falling back to standard I/O")
}
