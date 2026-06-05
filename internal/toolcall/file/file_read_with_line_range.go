package file

import (
	"bufio"
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/toolcall"
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
// 输出格式为 NR:TAG content，其中 TAG 是 4 字符校验和(CAS)标签。
// TAG 可传递给 write_file_with_line_range 的 line_tag/line_tags 参数防止竞态写入。
// 结果前包含头部行：📄 path: lines start-end of total
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

	// 验证行范围一致性：startLine 不得大于 endLine
	if endLine != -1 && startLine > endLine {
		err = fmt.Errorf("invalid line range: startLine(%d) must be <= endLine(%d)", startLine, endLine)
		return result, warning, err
	}

	// 打开文件
	file, err := os.Open(fullPath)
	if err != nil {
		err = fmt.Errorf("failed to open file: %w", err)
		return result, warning, err
	}
	defer file.Close()

	// 逐行读取：在范围内构建结果，超出范围继续计数以获取总行数
	var resultBuilder strings.Builder
	scanner := bufio.NewScanner(file)
	lineNum := 0
	linesRead := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()

		inRange := lineNum >= startLine && (endLine == -1 || lineNum <= endLine)
		if inRange {
			tag := computeLineTag(line)
			fmt.Fprintf(&resultBuilder, "%d:%s %s\n", lineNum, tag, line)
			linesRead++
		}
		// 不提前退出：即使超出 endLine，仍继续扫描以获取总行数
	}
	totalLines := lineNum

	if err = scanner.Err(); err != nil {
		err = fmt.Errorf("failed to read file line by line: %w", err)
		return result, warning, err
	}

	// 构建头部行（即使文件为空或没有行被读取，也提供头部元数据）
	var endDisplay string
	if endLine == -1 {
		endDisplay = fmt.Sprintf("%d", totalLines)
	} else {
		endDisplay = fmt.Sprintf("%d", endLine)
	}
	header := fmt.Sprintf("📄 %s: lines %d-%s of %d\n", path, startLine, endDisplay, totalLines)

	result = header + resultBuilder.String()

	// 记录日志
	rangeDesc := fmt.Sprintf("第%d行 - 第%d行", startLine, endLine)
	if endLine == -1 {
		rangeDesc = fmt.Sprintf("第%d行 - 末尾", startLine)
	}
	outfmt.Notice("读取文件 \"%s\" 行范围 %s，共 %d 行（文件总行数: %d）", path, rangeDesc, linesRead, totalLines)

	return result, warning, err
}
