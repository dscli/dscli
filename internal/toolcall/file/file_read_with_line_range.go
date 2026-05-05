package file

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	// 注册文件行范围读取工具（与awk格式完全兼容）
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "read_file_with_line_range",
		Description: `读取文件指定行范围的内容，输出格式与awk完全兼容。

参数：
  path: 文件路径（必需）
  start_line: 起始行号（可选，默认1）
  end_line: 结束行号（可选，默认到文件末尾）

输出格式与 awk 'NR>=start && NR<=end {print NR": "$0}' 完全一致。

适用场景：
- 处理非代码文件（如配置文件、文档等）
- 需要精确行号控制的场景

示例：
  # 显示所有行：read_file_with_line_range(path="file.txt")
  # 显示单行：read_file_with_line_range(path="file.txt", start_line=3, end_line=3)
  # 显示范围：read_file_with_line_range(path="file.txt", start_line=10, end_line=20)
  # 从某行到末尾：read_file_with_line_range(path="file.txt", start_line=50)`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"description": "起始行号（从1开始），可选，默认1",
				},
				"end_line": map[string]any{
					"type":        "integer",
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
func handleReadFileWithLineRange(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("parameter error: no path specified")
		return result, warning, err
	}

	fullPath := ResolvePath(ctx, path)

	startLine, endLine, err := ParseLineRange(args)
	if err != nil {
		err = fmt.Errorf("failed to parse line range: %w", err)
		return result, warning, err
	}

	// 打开文件
	file, err := os.Open(fullPath)
	if err != nil {
		err = fmt.Errorf("failed to open file: %w", err)
		return result, warning, err
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

	if err = scanner.Err(); err != nil {
		err = fmt.Errorf("failed to read file line by line: %w", err)
		return result, warning, err
	}

	// 如果起始行号超出文件范围，返回空字符串（与awk行为一致）
	if linesRead == 0 {
		return result, warning, err
	}

	result = resultBuilder.String()

	// 记录日志
	rangeDesc := fmt.Sprintf("第%d行 - 第%d行", startLine, endLine)
	if endLine == -1 {
		rangeDesc = fmt.Sprintf("第%d行 - 末尾", startLine)
	}
	outfmt.Notice("读取文件 \"%s\" 行范围 %s，共 %d 行", path, rangeDesc, linesRead)

	return result, warning, err
}
