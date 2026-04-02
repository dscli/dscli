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
	if RipgrepExists() {
		return
	}
	// 注册文件模式搜索工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "search_file_with_pattern",
		Description: `搜索文件中匹配指定模式的行，并显示上下文内容。
⚠️ 注意：这是基于行号操作的旧工具。建议优先使用基于语义的新工具 search_code_semantic。

参数：
  path: 文件路径（必需）
  pattern: 搜索模式（必需）
  context_lines: 上下文行数（可选，默认5）
  case_sensitive: 是否区分大小写（可选，默认false）
  max_matches: 最大匹配数（可选，默认无限制）

输出格式：
  > 匹配行号: 匹配行内容（用 > 标记）
     上下文行号: 上下文行内容

适用场景：
- 简单的文本模式搜索
- 处理非代码文件（如日志文件、配置文件等）
- 新工具 search_code_semantic 无法满足需求时的后备方案

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
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
					"pattern":     toolcall.TitleLikePattern(128),
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "搜索模式（字符串包含匹配）",
				},
				"context_lines": map[string]any{
					"type":        "integer",
					"description": "上下文行数（前后各N行），可选，默认5",
				},
				"case_sensitive": map[string]any{
					"type":        "boolean",
					"description": "是否区分大小写，可选，默认false",
				},
				"max_matches": map[string]any{
					"type":        "integer",
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

// handleSearchFileWithPattern 搜索文件中匹配指定模式的行，并显示上下文
// 输出格式与 awk 类似，保持一致性
func handleSearchFileWithPattern(ctx context.Context, args toolcall.ToolArgs) (result string, user string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("parameter error: no path specified")
		return
	}

	pattern := toolcall.ToolArgsValue(args, "pattern", "")
	if pattern == "" {
		err = fmt.Errorf("parameter error: no pattern specified")
		return
	}

	fullPath := ResolvePath(ctx, path)

	// 解析上下文行数参数
	contextLines := int(toolcall.ToolArgsValue(args, "context_lines", int64(5))) // 默认上下文行数
	if contextLines < 0 {
		err = fmt.Errorf("context_lines must be non-negative")
		return
	}

	// 解析是否区分大小写
	caseSensitive := toolcall.ToolArgsValue(args, "case_sensitive", false)

	// 解析最大匹配数
	maxMatches := int(toolcall.ToolArgsValue(args, "max_matches", int64(0))) // 0表示无限制
	if maxMatches < 0 {
		err = fmt.Errorf("max_matches must be non-negative")
		return
	}

	// 读取文件
	file, err := os.Open(fullPath)
	if err != nil {
		err = fmt.Errorf("failed to open file: %w", err)
		return
	}
	defer file.Close()

	// 读取所有行到内存中，以便获取上下文
	scanner := bufio.NewScanner(file)
	var allLines []string
	for scanner.Scan() {
		allLines = append(allLines, scanner.Text())
	}
	if err = scanner.Err(); err != nil {
		err = fmt.Errorf("failed to read file line by line: %w", err)
		return
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
		outfmt.Notice("在文件 \"%s\" 中搜索模式 \"%s\"，未找到匹配项", path, pattern)
		return
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

	result = resultBuilder.String()

	// 记录日志
	outfmt.Notice("在文件 \"%s\" 中搜索模式 \"%s\"，找到 %d 个匹配项，显示上下文 ±%d 行",
		path, pattern, len(matches), contextLines)

	return
}