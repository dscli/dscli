package main

import (
	"bufio"
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// 解析文件路径：如果是相对路径，则拼接项目根目录；否则直接使用
func resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ProjectRoot, path)
}

// handleReadFile 读取文件（纯Go实现）
func handleReadFile(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("parameter error: no path specified")
	}

	fullPath := resolvePath(path)

	// 读取文件
	startTime := time.Now()
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// 获取文件信息
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file information: %w", err)
	}

	// 构建结果
	executionTime := time.Since(startTime)
	result := fmt.Sprintf(`📄 文件内容:
内容:
%s

文件信息:
- 路径: %s
- 大小: %d 字节
- 权限: %s
- 修改时间: %s

📊 执行统计:
执行时间: %v
状态: 成功`,
		string(content),
		fullPath,
		fileInfo.Size(),
		fileInfo.Mode().String(),
		fileInfo.ModTime().Format("2006-01-02 15:04:05"),
		executionTime)
	Notice("读取文件: \"%s\"（%d字节）", fullPath, fileInfo.Size())
	return result, nil
}

func Shuffle(in string) (out string) {
	runes := []rune(in)
	rand.Shuffle(len(runes), func(i, j int) {
		runes[i], runes[j] = runes[j], runes[i]
	})
	out = string(runes)
	return
}

// handleWriteFile 写入文件（纯Go实现）
func handleWriteFile(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("参数错误: 缺少path参数")
	}

	fullPath := resolvePath(path)

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录%q失败: %w", dir, err)
	}

	content, ok := args["content"]
	if !ok { // no content specified means touch
		content = ""
	}
	// 写入文件
	startTime := time.Now()
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	// 获取文件信息用于统计
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}

	// 构建成功响应
	executionTime := time.Since(startTime)
	result := fmt.Sprintf(`✅ 写入成功:
已成功写入文件: \"%s\"
文件大小: %d 字节
权限: %s
路径: %s

📊 执行统计:
执行时间: %v
状态: 成功`,
		path,
		fileInfo.Size(),
		fileInfo.Mode().String(),
		fullPath,
		executionTime)

	Notice("写入文件: \"%s\"（%d字节）", path, fileInfo.Size())
	return result, nil
}

// handleSearchFiles 搜索文件
func handleSearchFiles(ctx context.Context, args map[string]string) (string, error) {
	pattern, ok := args["pattern"]
	if !ok {
		pattern = ""
	}
	content, ok := args["content"]
	if !ok {
		content = ""
	}
	// 使用find和grep命令实现搜索
	// 基础find命令：从当前目录开始，排除.git目录，只搜索文件
	script := `find . -type f -not -path "./.git/*"`

	// 添加文件名模式匹配
	if pattern != "" {
		// 将Go的glob模式转换为find的-name模式
		// 注意：这里简化处理，复杂的glob模式可能需要转换
		// 转义单引号：将'替换为'\''
		escapedPattern := strings.ReplaceAll(pattern, "'", "'\"'\"'")
		script += fmt.Sprintf(` -name '%s'`, escapedPattern)
	}

	// 添加内容匹配
	if content != "" {
		// 使用-exec和grep进行内容搜索
		// -l: 只显示包含匹配内容的文件名
		// -q: 安静模式，只返回退出状态
		// 转义单引号：将'替换为'\''
		escapedContent := strings.ReplaceAll(content, "'", "'\"'\"'")
		script += fmt.Sprintf(` -exec grep -lq '%s' {} \;`, escapedContent)
	}

	// 输出结果并限制数量
	script += ` -print 2>/dev/null | head -50`

	// 处理空结果
	script += ` || echo "未找到匹配的文件"`

	return runShell(ctx, script)
}

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

// handleWriteFileWithLineRange 写入文件指定行范围的内容
// 如果 content 为空字符串，则删除指定行范围
func handleWriteFileWithLineRange(_ context.Context, args map[string]string) (string, error) {
	// 检查必需参数
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("parameter error: no path specified")
	}

	content, ok := args["content"]
	if !ok {
		return "", fmt.Errorf("parameter error: no content specified")
	}
	// content 可以为空字符串，表示删除

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
		// 计算需要插入的空行数
		emptyLinesNeeded := startLine - len(lines) - 1
		for i := 0; i < emptyLinesNeeded; i++ {
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

	rangeDesc := fmt.Sprintf("%d-%d", startLine, endLine)
	if endLine == -1 {
		rangeDesc = fmt.Sprintf("%d-末尾", startLine)
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

	RegisterTool(ToolDef{
		Name:        "read_file",
		Description: "读取项目内指定文件的内容，返回文件内容和元数据信息（大小、权限、修改时间等）",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleReadFile,
	})

	RegisterTool(ToolDef{
		Name:        "write_file",
		Description: "将内容写入文件，如果文件不存在则创建，如果存在则覆盖。支持创建目录结构。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要写入的内容",
				},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFile,
	})

	RegisterTool(ToolDef{
		Name:        "search_files",
		Description: "在项目目录中搜索文件，支持文件名模式匹配（如*.go）和文件内容搜索。自动排除.git目录。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "文件名模式，如 '*.go'，为空则匹配所有文件",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要搜索的内容（如果提供则搜索文件内容）",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleSearchFiles,
	})

	// 注册文件行范围写入工具
	RegisterTool(ToolDef{
		Name: "write_file_with_line_range",
		Description: `写入文件指定行范围的内容，支持替换、插入和删除操作。
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

示例：
  # 替换第5-10行的内容
  write_file_with_line_range(path="file.txt", start_line="5", end_line="10", content="新内容")
  
  # 删除第5-10行的内容
  write_file_with_line_range(path="file.txt", start_line="5", end_line="10", content="")
  
  # 从第5行开始替换到文件末尾
  write_file_with_line_range(path="file.txt", start_line="5", content="新内容")
  
  # 删除从第5行到文件末尾的内容
  write_file_with_line_range(path="file.txt", start_line="5", content="")
  
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
					"type":        "string",
					"description": "起始行号（从1开始），可选，默认1",
				},
				"end_line": map[string]any{
					"type":        "string",
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
