package code

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"strings"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/parse"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/dscli/dscli/internal/toolcall/file"
)

//go:embed code_search_semantic.md
var code_search_semantic_md string

// searchCodeSemantic 基于语义搜索代码中的特定模式
// 参数：
//
//	path: 文件路径
//	searchPattern: 搜索模式（字符串包含匹配）
//	contextLines: 上下文行数（前后各N行）
//	caseSensitive: 是否区分大小写
//	maxMatches: 最大匹配数
//
// 返回值：
//
//	result: 搜索结果字符串
//	matchCount: 匹配行数
//	error: 错误信息
func searchCodeSemantic(ctx context.Context, path, searchPattern string, contextLines int, caseSensitive bool, maxMatches int) (string, int, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", 0, fmt.Errorf("文件不存在: %s", path)
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return "", 0, fmt.Errorf("读取文件失败: %w", err)
	}

	// 解析文件结构
	structure, err := parse.ParseFileStructure(ctx, path)
	if err != nil {
		return "", 0, fmt.Errorf("解析文件结构失败: %w", err)
	}
	// 搜索匹配项
	lines := strings.Split(string(content), "\n")
	// 去除文件末尾换行符产生的空元素（与bufio.Scanner行为一致）
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	matches := searchMatches(lines, searchPattern, caseSensitive, maxMatches)

	// 构建结果
	result := buildSearchResult(path, searchPattern, contextLines, caseSensitive, maxMatches, matches, lines, structure)

	return result, len(matches), nil
}

// searchMatches 搜索匹配项
func searchMatches(lines []string, searchPattern string, caseSensitive bool, maxMatches int) []int {
	var matches []int
	pattern := searchPattern
	if !caseSensitive {
		pattern = strings.ToLower(searchPattern)
	}

	for i, line := range lines {
		// 检查是否达到最大匹配数
		if maxMatches > 0 && len(matches) >= maxMatches {
			break
		}

		searchLine := line
		if !caseSensitive {
			searchLine = strings.ToLower(line)
		}

		if strings.Contains(searchLine, pattern) {
			matches = append(matches, i+1) // 行号从1开始
		}
	}

	return matches
}

// buildSearchResult 构建搜索结果
func buildSearchResult(path, searchPattern string, contextLines int, caseSensitive bool, maxMatches int, matches []int, lines []string, structure *parse.FileStructure) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "🔍 搜索文件: %s\n", path)
	fmt.Fprintf(&sb, "📝 搜索模式: %s\n", searchPattern)
	fmt.Fprintf(&sb, "⚙️  参数: 上下文行数=%d, 大小写敏感=%v, 最大匹配数=%d\n", contextLines, caseSensitive, maxMatches)
	fmt.Fprintf(&sb, "📊 匹配结果: %d 个\n\n", len(matches))

	if len(matches) == 0 {
		sb.WriteString("❌ 未找到匹配项\n")
		return sb.String()
	}

	// 显示每个匹配项
	displayedLines := make(map[int]bool)
	for i, matchLine := range matches {
		fmt.Fprintf(&sb, "### 匹配项 %d (第%d行)\n", i+1, matchLine)

		// 显示上下文
		startLine := max(1, matchLine-contextLines)
		endLine := min(len(lines), matchLine+contextLines)

		// 避免重复显示重叠的上下文
		for lineNum := startLine; lineNum <= endLine; lineNum++ {
			if displayedLines[lineNum] {
				continue
			}
			displayedLines[lineNum] = true

			// 如果是匹配行，用 > 标记
			if lineNum == matchLine {
				fmt.Fprintf(&sb, "> %d: %s\n", lineNum, lines[lineNum-1])
			} else {
				fmt.Fprintf(&sb, "  %d: %s\n", lineNum, lines[lineNum-1])
			}
		}

		sb.WriteString("\n")

		// 显示匹配行所在的代码结构信息
		structureInfo := getStructureInfoForLine(structure, matchLine)
		if structureInfo != "" {
			fmt.Fprintf(&sb, "📋 结构信息: %s\n\n", structureInfo)
		}
	}

	// 显示统计信息
	sb.WriteString("📈 搜索统计:\n")
	fmt.Fprintf(&sb, "  - 总行数: %d\n", len(lines))
	fmt.Fprintf(&sb, "  - 匹配数: %d\n", len(matches))
	if maxMatches > 0 && len(matches) >= maxMatches {
		fmt.Fprintf(&sb, "  - 注意: 已达到最大匹配数限制 (%d)\n", maxMatches)
	}

	return sb.String()
}

// getStructureInfoForLine 获取指定行所在的结构信息
func getStructureInfoForLine(structure *parse.FileStructure, line int) string {
	// 检查是否在函数内
	for _, fn := range structure.Functions {
		if line >= fn.Line && line <= fn.EndLine {
			info := fmt.Sprintf("函数: %s", fn.Name)
			if fn.Type == "method" && fn.Receiver != "" {
				info = fmt.Sprintf("方法: %s.%s", fn.Receiver, fn.Name)
			}
			if fn.Signature != "" {
				info += fmt.Sprintf(" (%s)", fn.Signature)
			}
			return info
		}
	}

	// 检查是否在类/结构体内
	for _, cls := range structure.Classes {
		if line >= cls.Line && line <= cls.EndLine {
			return fmt.Sprintf("%s: %s", cls.Type, cls.Name)
		}
	}

	return "全局作用域"
}

func init() {
	// 注册 searchCodeSemantic 工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "search_code_semantic",
		Description: code_search_semantic_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_pattern": map[string]any{
					"type":        "string",
					"description": "File search pattern: single file, wildcards, multiple files, current dir, recursive",
				},
				"search_pattern": map[string]any{
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
			"required":             []string{"file_pattern", "search_pattern"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleSearchCodeSemantic,
	})
}

func handleSearchCodeSemantic(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	filePattern := toolcall.ToolArgsValue(args, "file_pattern", "")
	if filePattern == "" {
		err = fmt.Errorf("参数 'file_pattern' 缺失")
		return result, warning, err
	}
	searchPattern := toolcall.ToolArgsValue(args, "search_pattern", "")
	if searchPattern == "" {
		err = fmt.Errorf("参数 'search_pattern' 缺失")
		return result, warning, err
	}

	// 解析可选参数
	contextLines := int(toolcall.ToolArgsValue(args, "context_lines", int64(5)))
	caseSensitive := toolcall.ToolArgsValue(args, "case_sensitive", false)
	maxMatches := int(toolcall.ToolArgsValue(args, "max_matches", int64(0)))

	// 扩展文件模式
	files, err := file.ExpandFilePattern(filePattern)
	if err != nil {
		err = fmt.Errorf("扩展文件模式失败: %w", err)
		return result, warning, err
	}

	if len(files) == 0 {
		result = "❌ 未找到匹配的文件"
		return result, warning, err
	}

	outfmt.Printf("🔍 搜索%d个文件中匹配指定模式%s的行\n", len(files), searchPattern)

	// 搜索所有文件
	var results []string
	var errors []string
	totalMatches := 0

	for _, file := range files {
		result, matchCount, err := searchCodeSemantic(ctx, file, searchPattern, contextLines, caseSensitive, maxMatches)
		if err != nil {
			errors = append(errors, fmt.Sprintf("❌ 搜索文件 %s 失败: %v", file, err))
		} else {
			results = append(results, result)
			totalMatches += matchCount
		}

		// 检查全局匹配数限制
		if maxMatches > 0 && totalMatches >= maxMatches {
			outfmt.Printf("⚠️ 已达到全局最大匹配数限制 (%d)，停止搜索\n", maxMatches)
			break
		}
	}

	// 构建最终结果
	var sb strings.Builder

	// 显示错误信息
	if len(errors) > 0 {
		sb.WriteString("❌ 错误信息:\n")
		for _, err := range errors {
			fmt.Fprintf(&sb, "  - %s\n", err)
		}
		sb.WriteString("\n")
	}

	// 显示搜索结果
	if len(results) > 0 {
		sb.WriteString("✅ 搜索结果:\n")
		sb.WriteString(strings.Join(results, "\n\n---\n\n"))
	}

	// 显示统计信息
	fmt.Fprintf(&sb, "\n📈 全局统计:\n")
	fmt.Fprintf(&sb, "  - 搜索文件数: %d\n", len(files))
	fmt.Fprintf(&sb, "  - 成功搜索文件数: %d\n", len(results))
	fmt.Fprintf(&sb, "  - 失败文件数: %d\n", len(errors))
	fmt.Fprintf(&sb, "  - 总匹配数: %d\n", totalMatches)
	if maxMatches > 0 && totalMatches >= maxMatches {
		fmt.Fprintf(&sb, "  - 注意: 已达到全局最大匹配数限制 (%d)\n", maxMatches)
	}
	result = sb.String()
	return result, warning, err
}
