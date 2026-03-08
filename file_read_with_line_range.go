package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

func init() {
	// 注册文件行范围读取工具（与awk格式完全兼容）
	RegisterTool(ToolDef{
		Name: "read_file_with_line_range",
		Description: `读取文件指定行范围的内容，输出格式与awk完全兼容。
⚠️ 注意：这是基于行号操作的旧工具。建议优先使用基于代码结构的新工具 read_code_section。

参数：
  path: 文件路径（必需）
  start_line: 起始行号（可选，默认1）
  end_line: 结束行号（可选，默认到文件末尾）

输出格式与 awk 'NR>=start && NR<=end {print NR": "$0}' 完全一致。

适用场景：
- 处理非代码文件（如配置文件、文档等）
- 需要精确行号控制的场景
- 新工具 read_code_section 无法满足需求时的后备方案

示例：
  # 显示所有行：read_file_with_line_range(path="file.txt")
  # 显示单行：read_file_with_line_range(path="file.txt", start_line="3", end_line="3")
  # 显示范围：read_file_with_line_range(path="file.txt", start_line="10", end_line="20")
  # 从某行到末尾：read_file_with_line_range(path="file.txt", start_line="50")`,
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

// handleReadFileWithLineRange 读取文件指定行范围的内容
// 输出格式与 awk 'NR>=start && NR<=end {print NR": "$0}' 完全一致
func handleReadFileWithLineRange(_ context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("parameter error: no path specified")
	}

	fullPath := resolvePath(path)

	startLine, endLine, err := parseLineRange(args)
	if err != nil {
		return "", err
	}

	// 打开文件
	file, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 逐行读取并构建结果
	var resultBuilder strings.Builder
	scanner := bufio.NewScanner(file)
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		// 检查是否在指定范围内
		if lineNum >= startLine && (endLine == -1 || lineNum <= endLine) {
			fmt.Fprintf(&resultBuilder, "%d: %s\n", lineNum, line)
			linesRead++
		}

		// 如果已经超过结束行号，可以提前退出
		if endLine != -1 && lineNum > endLine {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read file line by line: %w", err)
	}

	// 如果起始行号超出文件范围，返回空字符串（与awk行为一致）
	if linesRead == 0 {
		return "", nil
	}

	result := resultBuilder.String()

	// 记录日志
	Notice("读取文件 \"%s\" 行范围 %d-%d，共 %d 行", path, startLine, endLine, linesRead)

	return result, nil
}
