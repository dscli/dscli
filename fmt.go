package main

import (
	"fmt"
	"io"
)

// Println 输出一行文本（保持向后兼容）
func Println(a ...any) (n int, err error) {
	return fmt.Fprintln(outputWriterVar, a...)
}

// Printf 输出格式化文本（保持向后兼容）
func Printf(format string, a ...any) (n int, err error) {
	return fmt.Fprintf(outputWriterVar, format, a...)
}

// SetOutputWriter 设置输出写入器
func SetOutputWriter(w io.Writer) {
	outputWriterVar = w
}
