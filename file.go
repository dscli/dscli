package main

import (
	"fmt"
	"math/rand"
	"path/filepath"
)

func init() {
}

// 解析文件路径：如果是相对路径，则拼接项目根目录；否则直接使用
func resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ProjectRoot, path)
}

func Shuffle(in string) (out string) {
	runes := []rune(in)
	rand.Shuffle(len(runes), func(i, j int) {
		runes[i], runes[j] = runes[j], runes[i]
	})
	out = string(runes)
	return
}

func parseLineRange(args ToolArgs) (int, int, error) {
	// 解析开始行号
	startLine := ToolArgsValue(args, "start_line", 1)
	// 解析结束行号
	endLine := ToolArgsValue(args, "end_line", -1) // -1 表示到文件末尾
	// 验证行号范围
	if endLine != -1 && endLine < startLine {
		return 0, 0, fmt.Errorf("end_line must be greater than or equal to start_line")
	}
	return startLine, endLine, nil
}
