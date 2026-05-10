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

//go:embed file_search_with_pattern.md
var file_search_with_pattern_md string

func init() {
	// 注册文件模式搜索工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "search_file_with_pattern",
		Description: file_search_with_pattern_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path, e.g. main.go",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "Search pattern (substring match)",
				},
				"context_lines": map[string]any{
					"type":        "integer",
					"description": "Context lines (N before and after), optional, default 5",
				},
				"case_sensitive": map[string]any{
					"type":        "boolean",
					"description": "Case-sensitive search, optional, default false",
				},
				"max_matches": map[string]any{
					"type":        "integer",
					"description": "Max matches, optional, default unlimited",
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
func handleSearchFileWithPattern(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("parameter error: no path specified")
		return result, warning, err
	}

	pattern := toolcall.ToolArgsValue(args, "pattern", "")
	if pattern == "" {
		err = fmt.Errorf("parameter error: no pattern specified")
		return result, warning, err
	}

	fullPath := ResolvePath(ctx, path)

	// 解析上下文行数参数
	contextLines := int(toolcall.ToolArgsValue(args, "context_lines", int64(5))) // 默认上下文行数
	if contextLines < 0 {
		err = fmt.Errorf("context_lines must be non-negative")
		return result, warning, err
	}

	// 解析是否区分大小写
	caseSensitive := toolcall.ToolArgsValue(args, "case_sensitive", false)

	// 解析最大匹配数
	maxMatches := int(toolcall.ToolArgsValue(args, "max_matches", int64(0))) // 0表示无限制
	if maxMatches < 0 {
		err = fmt.Errorf("max_matches must be non-negative")
		return result, warning, err
	}

	// 读取文件
	file, err := os.Open(fullPath)
	if err != nil {
		err = fmt.Errorf("failed to open file: %w", err)
		return result, warning, err
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
		return result, warning, err
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
		return result, warning, err
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

	return result, warning, err
}
