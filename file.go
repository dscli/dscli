package main

import (
	"fmt"
	"math/rand"
	"path/filepath"
	"strconv"
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

func parseLineRange(args map[string]string) (int, int, error) {
	startLine := 1
	if startStr, ok := args["start_line"]; ok && startStr != "" {
		start, err := strconv.Atoi(startStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid start_line parameter: %w", err)
		}
		if start < 1 {
			return 0, 0, fmt.Errorf("start_line must be at least 1")
		}
		startLine = start
	}

	// 解析结束行号
	endLine := -1 // -1 表示到文件末尾
	if endStr, ok := args["end_line"]; ok && endStr != "" {
		end, err := strconv.Atoi(endStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid end_line parameter: %w", err)
		}
		if end < 1 {
			return 0, 0, fmt.Errorf("end_line must be at least 1")
		}
		endLine = end
	}

	// 验证行号范围
	if endLine != -1 && endLine < startLine {
		return 0, 0, fmt.Errorf("end_line must be greater than or equal to start_line")
	}
	return startLine, endLine, nil
}
