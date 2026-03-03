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

	// 解析起始行号
	startLine := 1
	if startStr, ok := args["start_line"]; ok && startStr != "" {
		start, err := strconv.Atoi(startStr)
		if err != nil {
			return "", fmt.Errorf("invalid start_line parameter: %w", err)
		}
		if start < 1 {
			return "", fmt.Errorf("start_line must be at least 1")
		}
		startLine = start
	}

	// 解析结束行号
	endLine := -1 // -1 表示到文件末尾
	if endStr, ok := args["end_line"]; ok && endStr != "" {
		end, err := strconv.Atoi(endStr)
		if err != nil {
			return "", fmt.Errorf("invalid end_line parameter: %w", err)
		}
		if end < 1 {
			return "", fmt.Errorf("end_line must be at least 1")
		}
		endLine = end
	}

	// 验证行号范围
	if endLine != -1 && endLine < startLine {
		return "", fmt.Errorf("end_line must be greater than or equal to start_line")
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

// handleSearchFileWithPattern 搜索文件中匹配指定模式的行，并显示上下文
// 输出格式与 awk 类似，保持一致性
func handleSearchFileWithPattern(_ context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("parameter error: no path specified")
	}

	pattern, ok := args["pattern"]
	if !ok || pattern == "" {
		return "", fmt.Errorf("parameter error: no pattern specified")
	}

	fullPath := resolvePath(path)

	// 解析上下文行数参数
	contextLines := 5 // 默认上下文行数
	if contextStr, ok := args["context_lines"]; ok && contextStr != "" {
		ctx, err := strconv.Atoi(contextStr)
		if err != nil {
			return "", fmt.Errorf("invalid context_lines parameter: %w", err)
		}
		if ctx < 0 {
			return "", fmt.Errorf("context_lines must be non-negative")
		}
		contextLines = ctx
	}

	// 解析是否区分大小写
	caseSensitive := false
	if caseStr, ok := args["case_sensitive"]; ok && caseStr != "" {
		if caseStr == "true" || caseStr == "1" {
			caseSensitive = true
		}
	}

	// 解析最大匹配数
	maxMatches := 0 // 0表示无限制
	if maxStr, ok := args["max_matches"]; ok && maxStr != "" {
		max, err := strconv.Atoi(maxStr)
		if err != nil {
			return "", fmt.Errorf("invalid max_matches parameter: %w", err)
		}
		if max < 0 {
			return "", fmt.Errorf("max_matches must be non-negative")
		}
		maxMatches = max
	}

	// 读取文件
	file, err := os.Open(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// 读取所有行到内存中，以便获取上下文
	scanner := bufio.NewScanner(file)
	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return "", fmt.Errorf("failed to read file line by line: %w", err)
	}

	// 准备搜索
	searchPattern := pattern
	if !caseSensitive {
		searchPattern = strings.ToLower(pattern)
	}

	// 查找匹配行
	var matches []int // 匹配行的索引（从0开始）
	for i, line := range allLines {
		// 检查是否达到最大匹配数
		if maxMatches > 0 && len(matches) >= maxMatches {
			break
		}

		lineToSearch := line
		if !caseSensitive {
			lineToSearch = strings.ToLower(line)
		}

		if strings.Contains(lineToSearch, searchPattern) {
			matches = append(matches, i)
		}
	}

	// 如果没有匹配项
	if len(matches) == 0 {
		Notice("在文件 \"%s\" 中搜索模式 \"%s\"，未找到匹配项", path, pattern)
		return "", nil
	}

	// 构建结果
	var resultBuilder strings.Builder

	// 用于跟踪已输出的行，避免重复输出（当上下文重叠时）
	outputLines := make(map[int]bool)

	// 用于跟踪上一个匹配项的上下文结束行
	prevEndCtx := -1

	for matchIdx, lineIdx := range matches {
		// 计算上下文范围
		startCtx := max(lineIdx-contextLines, 0)
		endCtx := min(lineIdx+contextLines, len(allLines)-1)

		// 如果这不是第一个匹配项，并且上下文范围与前一个匹配项没有重叠，则添加空行分隔符
		if matchIdx > 0 && startCtx > prevEndCtx {
			resultBuilder.WriteString("\n")
		}

		// 按行号顺序输出上下文行和匹配行
		for i := startCtx; i <= endCtx; i++ {
			// 避免重复输出
			if outputLines[i] {
				continue
			}
			outputLines[i] = true

			// 判断是否是匹配行
			if i == lineIdx {
				// 匹配行用 > 标记
				fmt.Fprintf(&resultBuilder, "> %d: %s\n", i+1, allLines[i])
			} else {
				// 上下文行用两个空格对齐
				fmt.Fprintf(&resultBuilder, "  %d: %s\n", i+1, allLines[i])
			}
		}

		// 更新上一个匹配项的上下文结束行
		prevEndCtx = endCtx
	}

	result := resultBuilder.String()

	// 记录日志
	Notice("在文件 \"%s\" 中搜索模式 \"%s\"，找到 %d 个匹配项，显示上下文 ±%d 行",
		path, pattern, len(matches), contextLines)

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

	// 注册文件模式搜索工具
	RegisterTool(ToolDef{
		Name: "search_file_with_pattern",
		Description: `搜索文件中匹配指定模式的行，并显示上下文内容。
参数：
  path: 文件路径（必需）
  pattern: 搜索模式（必需）
  context_lines: 上下文行数（可选，默认5）
  case_sensitive: 是否区分大小写（可选，默认false）
  max_matches: 最大匹配数（可选，默认无限制）

输出格式：
  > 匹配行号: 匹配行内容（用 > 标记）
    上下文行号: 上下文行内容

示例：
  # 搜索包含"error"的行，显示前后5行上下文
  search_file_with_pattern(path="app.log", pattern="error")
  
  # 搜索"TODO"注释，显示前后3行上下文
  search_file_with_pattern(path="main.go", pattern="TODO", context_lines="3")
  
  # 区分大小写搜索"Config"
  search_file_with_pattern(path="config.yaml", pattern="Config", case_sensitive="true")
  
  # 只显示前10个匹配项
  search_file_with_pattern(path="large.log", pattern="warning", max_matches="10")

功能特点：
1. 支持简单的字符串包含匹配
2. 显示匹配行及其上下文，便于理解上下文
3. 避免重复输出重叠的上下文区域
4. 支持大小写敏感/不敏感搜索
5. 可限制最大匹配数，避免输出过多内容`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "搜索模式（字符串包含匹配）",
				},
				"context_lines": map[string]any{
					"type":        "string",
					"description": "上下文行数（前后各N行），可选，默认5",
				},
				"case_sensitive": map[string]any{
					"type":        "string",
					"description": "是否区分大小写，可选，默认false",
				},
				"max_matches": map[string]any{
					"type":        "string",
					"description": "最大匹配数，可选，默认无限制",
				},
			},
			"required":             []string{"path", "pattern"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleSearchFileWithPattern,
	})
}
