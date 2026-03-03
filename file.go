// Package main 文件操作工具实现
// 包含 read_file 和 read_file_with_line_range 工具的实现
package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// handleReadFileWithLineRange 读取文件指定行范围的内容
func handleReadFileWithLineRange(_ context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("parameter error: no path specified")
	}

	fullPath := resolvePath(path)

	// 解析行范围参数
	startLine, endLine := 1, -1 // 默认从第1行开始，-1表示到文件末尾
	var err error

	if startStr, ok := args["start_line"]; ok && startStr != "" {
		startLine, err = strconv.Atoi(startStr)
		if err != nil {
			return "", fmt.Errorf("invalid start_line parameter: %w", err)
		}
		if startLine < 1 {
			startLine = 1
		}
	}

	if endStr, ok := args["end_line"]; ok && endStr != "" {
		endLine, err = strconv.Atoi(endStr)
		if err != nil {
			return "", fmt.Errorf("invalid end_line parameter: %w", err)
		}
		if endLine < startLine {
			return "", fmt.Errorf("end_line (%d) must be greater than or equal to start_line (%d)", endLine, startLine)
		}
	}

	// 读取文件
	startTime := time.Now()
	file, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 获取文件信息
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file information: %w", err)
	}

	// 逐行读取并过滤
	scanner := bufio.NewScanner(file)
	var lines []string
	lineNum := 0
	totalLines := 0

	for scanner.Scan() {
		totalLines++
		lineNum++

		// 如果还没有到起始行，继续扫描
		if lineNum < startLine {
			continue
		}

		// 如果指定了结束行且已超过结束行，停止扫描
		if endLine > 0 && lineNum > endLine {
			break
		}

		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read file line by line: %w", err)
	}

	// 构建结果内容
	var contentBuilder strings.Builder
	if len(lines) == 0 {
		contentBuilder.WriteString("（指定行范围内无内容）")
	} else {
		for i, line := range lines {
			actualLineNum := startLine + i
			fmt.Fprintf(&contentBuilder, "%4d: %s\n", actualLineNum, line)
		}
		// 移除最后一个换行符
		contentStr := contentBuilder.String()
		if len(contentStr) > 0 && contentStr[len(contentStr)-1] == '\n' {
			contentStr = contentStr[:len(contentStr)-1]
		}
		contentBuilder.Reset()
		contentBuilder.WriteString(contentStr)
	}

	// 构建行范围描述
	rangeDesc := "完整文件"
	if startLine > 1 || endLine > 0 {
		if endLine > 0 {
			rangeDesc = fmt.Sprintf("第 %d-%d 行", startLine, endLine)
		} else {
			rangeDesc = fmt.Sprintf("第 %d 行到文件末尾", startLine)
		}
	}

	// 构建结果
	executionTime := time.Since(startTime)
	result := fmt.Sprintf(`📄 文件内容 (%s):
内容:
%s

文件信息:
- 路径: %s
- 大小: %d 字节
- 总行数: %d 行
- 读取范围: %s
- 读取行数: %d 行
- 权限: %s
- 修改时间: %s

📊 执行统计:
执行时间: %v
状态: 成功`,
		rangeDesc,
		contentBuilder.String(),
		fullPath,
		fileInfo.Size(),
		totalLines,
		rangeDesc,
		len(lines),
		fileInfo.Mode().String(),
		fileInfo.ModTime().Format("2006-01-02 15:04:05"),
		executionTime)

	Notice("读取文件: \"%s\" (%s, %d行)", fullPath, rangeDesc, len(lines))
	return result, nil
}
