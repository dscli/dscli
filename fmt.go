package main

import (
	"fmt"
	"io"
	"os"
)

// 输出写入器
var outputWriter io.Writer = os.Stdout

// Println 根据当前模式输出
func Println(a ...any) (n int, err error) {
	return fmt.Fprintln(outputWriter, a...)
}

// Printf 根据当前模式输出
func Printf(format string, a ...any) (n int, err error) {
	return fmt.Fprintf(outputWriter, format, a...)
}

// SetOutputWriter 设置输出写入器
func SetOutputWriter(w io.Writer) {
	outputWriter = w
}
