package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
)

func init() {
	// 注册文件行范围写入工具
	RegisterTool(ToolDef{
		Name: "write_file_with_line_range",
		Description: `写入文件指定行范围的内容，支持替换、插入和删除操作。
⚠️ 注意：这是基于行号操作的旧工具。建议优先使用基于代码结构的新工具 write_code_section。

参数：
  path: 文件路径（必需）
  content: 要写入的内容（必需，可以为空字符串表示删除）
  start_line: 起始行号（可选，默认1）
  end_line: 结束行号（可选，默认到文件末尾）

功能说明：
1. 替换：用 content 替换指定行范围的内容
2. 删除：当 content 为空字符串时，删除指定行范围的内容
3. 插入：当 start_line 超出文件行数时，在文件末尾追加内容
4. 新建：当文件不存在时，创建新文件并写入内容

适用场景：
- 处理非代码文件（如配置文件、文档等）
- 需要精确行号控制的场景
- 新工具 write_code_section 无法满足需求时的后备方案

示例：
  # 替换第5-10行的内容
  write_file_with_line_range(path="file.txt", start_line=5, end_line=10, content="新内容")
  
  # 删除第5-10行的内容
  write_file_with_line_range(path="file.txt", start_line=5, end_line=10, content="")
  
  # 从第5行开始替换到文件末尾
  write_file_with_line_range(path="file.txt", start_line=5, content="新内容")
  
  # 删除从第5行到文件末尾的内容
  write_file_with_line_range(path="file.txt", start_line=5, content="")
  
  # 替换整个文件
  write_file_with_line_range(path="file.txt", content="全新内容")
  
  # 清空文件
  write_file_with_line_range(path="file.txt", content="")
  
  # 创建新文件并写入内容
  write_file_with_line_range(path="new.txt", content="文件内容")`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要写入的内容，可以为空字符串表示删除",
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
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFileWithLineRange,
	})
}

// handleWriteFileWithLineRange 写入文件指定行范围的内容
// 如果 content 为空字符串，则删除指定行范围
func handleWriteFileWithLineRange(_ context.Context, args ToolArgs) (string, error) {
	// 检查必需参数
	path := ToolArgsValue(args, "path", "")
	if path == "" {
		return "", fmt.Errorf("parameter error: no path specified")
	}

	content := ToolArgsValue(args, "content", "")

	// content 可以为空字符串，表示删除

	fullPath := resolvePath(path)

	// 解析起始行号
	startLine, endLine, err := parseLineRange(args)
	if err != nil {
		return "", err
	}

	// 读取原文件所有行
	file, err := os.Open(fullPath)
	if err != nil {
		// 如果文件不存在，创建一个空文件
		if os.IsNotExist(err) {
			// 对于新文件，只能从第1行开始写入
			if startLine != 1 {
				return "", fmt.Errorf("cannot write to non-existent file at line %d, must start from line 1", startLine)
			}

			// 创建新文件并写入内容
			if content == "" {
				// 空内容，创建空文件
				newFile, err := os.Create(fullPath)
				if err != nil {
					return "", fmt.Errorf("failed to create file: %w", err)
				}
				newFile.Close()
				Notice("创建空文件 \"%s\"", path)
				return "成功创建空文件", nil
			}

			// 写入内容到新文件
			err = os.WriteFile(fullPath, []byte(content), 0o644)
			if err != nil {
				return "", fmt.Errorf("failed to write to new file: %w", err)
			}

			lines := strings.Count(content, "\n") + 1
			if content == "" || strings.HasSuffix(content, "\n") {
				lines = strings.Count(content, "\n")
			}

			Notice("创建文件 \"%s\" 并写入 %d 行内容", path, lines)
			return fmt.Sprintf("成功创建文件并写入 %d 行内容", lines), nil
		}
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 读取所有行
	scanner := bufio.NewScanner(file)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
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
		return "", fmt.Errorf("failed to write file: %w", err)
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

	Notice("%s文件 \"%s\" 行范围 %s，影响 %d 行", operation, path, rangeDesc, linesChanged)

	return fmt.Sprintf("成功%s文件 \"%s\" 行范围 %s", operation, path, rangeDesc), nil
}
