package file

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed file_read_with_line_range.md
var file_read_with_line_range_md string

func init() {
	// 注册文件行范围读取工具（与awk格式完全兼容）
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "read_file_with_line_range",
		Description: file_read_with_line_range_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, e.g. main.go",
				},
				"start_line": map[string]any{
					"type":        "integer",
					"description": "Start line (1-based), optional, default 1",
				},
				"end_line": map[string]any{
					"type":        "integer",
					"description": "End line, optional, default end of file",
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
