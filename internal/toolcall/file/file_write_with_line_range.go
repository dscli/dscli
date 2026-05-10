package file

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/flycheck"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed file_write_with_line_range.md
var file_write_with_line_range_md string

func init() {
	// 注册文件行范围写入工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "write_file_with_line_range",
		Description: file_write_with_line_range_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, e.g. main.go",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "Content to write; empty string to delete lines, max 4096 chars recommended",
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
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFileWithLineRange,
	})
}

// handleWriteFileWithLineRange 写入文件指定行范围的内容
// 如果 content 为空字符串，则删除指定行范围
func handleWriteFileWithLineRange(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	// 检查必需参数
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("parameter error: no path specified")
		return result, warning, err
	}

	content := toolcall.ToolArgsValue(args, "content", "")

	// content 可以为空字符串，表示删除

	fullPath := ResolvePath(ctx, path)

	// 解析起始行号
	startLine, endLine, err := ParseLineRange(args)
	if err != nil {
		err = fmt.Errorf("failed to parse line range: %w", err)
		return result, warning, err
	}

	// 读取原文件所有行
	file, err := os.Open(fullPath)
	if err != nil {
		// 如果文件不存在，创建一个空文件
		if os.IsNotExist(err) {
			// 对于新文件，只能从第1行开始写入
			if startLine != 1 {
				err = fmt.Errorf("cannot write to non-existent file at line %d, must start from line 1", startLine)
				return result, warning, err
			}

			// 创建新文件并写入内容
			if content == "" {
				// 空内容，创建空文件
				var newFile *os.File
				newFile, err = os.Create(fullPath)
				if err != nil {
					err = fmt.Errorf("failed to create file: %w", err)
					return result, warning, err
				}
				newFile.Close()
				outfmt.Notice("创建空文件 \"%s\"", path)
				result = "成功创建空文件"
				return result, warning, err
			}

			// 写入内容到新文件
			err = os.WriteFile(fullPath, []byte(content), 0o644)
			if err != nil {
				err = fmt.Errorf("failed to write to new file: %w", err)
				return result, warning, err
			}

			lines := strings.Count(content, "\n") + 1
			if content == "" || strings.HasSuffix(content, "\n") {
				lines = strings.Count(content, "\n")
			}

			outfmt.Notice("创建文件 \"%s\" 并写入 %d 行内容", path, lines)
			result = fmt.Sprintf("成功创建文件并写入 %d 行内容", lines)
			return result, warning, err
		}
		err = fmt.Errorf("failed to open file: %w", err)
		return result, warning, err
	}

	defer file.Close()

	// 读取所有行
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		err = fmt.Errorf("failed to read file: %w", err)
		return result, warning, err
	}

	// 构建新内容
	var newLines []string

	// 1. 添加 start_line 之前的部分
	beforeStart := min(startLine-1, len(lines))
	if beforeStart > 0 {
		newLines = append(newLines, lines[:beforeStart]...)
	}
	// 2. 如果 startLine 超出文件范围，需要插入空行
	if startLine > len(lines) {
		emptyLinesNeeded := startLine - len(lines) - 1
		for range emptyLinesNeeded {
			newLines = append(newLines, "")
		}
	}

	// 3. 处理新内容
	if content != "" {
		// 分割新内容为多行
		contentLines := strings.Split(content, "\n")
		newLines = append(newLines, contentLines...)
	}
	// 如果 content 为空，这里什么都不添加，相当于删除

	// 4. 添加 end_line 之后的部分
	if endLine != -1 {
		// endLine 是包含的结束行号，所以之后的部分从 endLine 开始
		// 但需要确保 endLine 在文件范围内
		if endLine < len(lines) {
			newLines = append(newLines, lines[endLine:]...)
		}
	}

	// 将新内容写回文件
	var contentBuilder strings.Builder
	for i, line := range newLines {
		contentBuilder.WriteString(line)
		if i < len(newLines)-1 {
			contentBuilder.WriteString("\n")
		}
	}

	err = os.WriteFile(fullPath, []byte(contentBuilder.String()), 0o644)
	if err != nil {
		err = fmt.Errorf("failed to write file: %w", err)
		return result, warning, err
	}

	// 记录操作日志
	operation := "替换"
	if content == "" {
		operation = "删除"
	}

	rangeDesc := fmt.Sprintf("第%d行 - 第%d行", startLine, endLine)
	if endLine == -1 {
		rangeDesc = fmt.Sprintf("第%d行 - 末尾", startLine)
	}

	linesChanged := 0
	if content == "" {
		// 删除的行数
		linesToDelete := 0
		if endLine == -1 {
			linesToDelete = max(0, len(lines)-startLine+1)
		} else {
			linesToDelete = max(0, min(endLine, len(lines))-startLine+1)
		}
		linesChanged = linesToDelete
	} else {
		// 替换/插入的行数
		contentLineCount := strings.Count(content, "\n") + 1
		if content == "" || strings.HasSuffix(content, "\n") {
			contentLineCount = strings.Count(content, "\n")
		}
		linesChanged = contentLineCount
	}

	outfmt.Notice("%s文件 \"%s\" 行范围 %s，影响 %d 行", operation, path, rangeDesc, linesChanged)

	// 构建最终结果
	result = fmt.Sprintf("成功%s文件 \"%s\" 行范围 %s", operation, path, rangeDesc)

	// Run flycheck on the written file and append issues as suggestion
	if flyResult, _, flyErr := flycheck.Flycheck(ctx, path); flyErr == nil && flyResult != "" {
		if warning != "" {
			warning += "\n\n"
		}
		warning += flyResult
	}

	return result, warning, err
}
