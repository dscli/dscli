//go:build !windows

package editor

import "os"

// openTTY 打开当前进程的控制终端（/dev/tty），返回可读写的文件描述符。
// 外部编辑器需要真实终端来正确控制光标、处理 raw mode 等，
// 如果 stdin/stdout 是管道，编辑器会出现显示异常（如"楼梯效应"）。
// 通过将 cmd.Stdin/Stdout/Stderr 定向到 /dev/tty 可确保编辑器
// 始终连接到真实终端，不受 dscli 自身输入输出重定向的影响。
func openTTY() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_RDWR, 0)
}
