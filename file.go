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
)

// handleReadFileWithLineRange 读取文件指定行范围的内容
// 输出格式与 awk 'NR>=start && NR<=end {print NR": "$0}' 完全一致
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
	file, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

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

	// 构建结果内容 - 与awk格式完全一致: NR": "$0
	var contentBuilder strings.Builder
	if len(lines) == 0 {
		// 空范围时返回空字符串，与awk行为一致
		return "", nil
	} else {
		for i, line := range lines {
			actualLineNum := startLine + i
			// 与 awk 'NR>=start && NR<=end {print NR": "$0}' 格式完全一致
			fmt.Fprintf(&contentBuilder, "%d: %s\n", actualLineNum, line)
		}
	}

	// 移除最后一个换行符（如果需要保持与awk完全一致，可以保留）
	result := contentBuilder.String()

	// 记录日志但不包含在返回结果中
	rangeDesc := "完整文件"
	if startLine > 1 || endLine > 0 {
		if endLine > 0 {
			rangeDesc = fmt.Sprintf("第 %d-%d 行", startLine, endLine)
		} else {
			rangeDesc = fmt.Sprintf("第 %d 行到文件末尾", startLine)
		}
	}
	Notice("读取文件: \"%s\" (%s, %d行)", fullPath, rangeDesc, len(lines))

	return result, nil
}

func init() {
	// 注册文件行范围读取工具（与awk格式完全兼容）
	RegisterTool(ToolDef{
		Name: "read_file_with_line_range",
		Description: `读取文件指定行范围的内容，输出格式与awk完全兼容。
参数：
  path: 文件路径（必需）
  start_line: 起始行号（可选，默认1）
  end_line: 结束行号（可选，默认到文件末尾）

输出格式与 awk 'NR>=start && NR<=end {print NR": "$0}' 完全一致。

示例：
  read_file_with_line_range(path="file.txt", start_line="5", end_line="10")
  等价于：awk 'NR>=5 && NR<=10 {print NR": "$0}' file.txt

常用场景：
1. 显示所有行：read_file_with_line_range(path="file.txt")
2. 显示单行：read_file_with_line_range(path="file.txt", start_line="3", end_line="3")
3. 显示范围：read_file_with_line_range(path="file.txt", start_line="10", end_line="20")
4. 从某行到末尾：read_file_with_line_range(path="file.txt", start_line="50")`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"start_line": map[string]any{
					"type":        "string",
					"description": "起始行号（从1开始），可选，默认1",
				},
				"end_line": map[string]any{
					"type":        "string",
					"description": "结束行号，可选，默认到文件末尾",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleReadFileWithLineRange,
	})
}
