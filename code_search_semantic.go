package main

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// searchCodeSemantic 基于语义搜索代码中的特定模式
// 参数：
//
//	path: 文件路径
//	pattern: 搜索模式（字符串包含匹配）
//	contextLines: 上下文行数（前后各N行）
//	caseSensitive: 是否区分大小写
//	maxMatches: 最大匹配数
func searchCodeSemantic(path string, pattern string, contextLines int, caseSensitive bool, maxMatches int) (string, error) {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("文件不存在: %s", path)
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	// 解析文件结构
	structure, err := ParseFileStructure(path, string(content))
	if err != nil {
		return "", fmt.Errorf("解析文件结构失败: %w", err)
	}

	// 搜索匹配项
	lines := strings.Split(string(content), "\n")
	matches := searchMatches(lines, pattern, caseSensitive, maxMatches)

	// 构建结果
	result := buildSearchResult(path, pattern, contextLines, caseSensitive, maxMatches, matches, lines, structure)

	return result, nil
}

// searchMatches 搜索匹配项
func searchMatches(lines []string, pattern string, caseSensitive bool, maxMatches int) []int {
	var matches []int
	searchPattern := pattern
	if !caseSensitive {
		searchPattern = strings.ToLower(pattern)
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

		if strings.Contains(searchLine, searchPattern) {
			matches = append(matches, i+1) // 行号从1开始
		}
	}

	return matches
}

// buildSearchResult 构建搜索结果
func buildSearchResult(path, pattern string, contextLines int, caseSensitive bool, maxMatches int, matches []int, lines []string, structure *FileStructure) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("🔍 搜索文件: %s\n", path))
	sb.WriteString(fmt.Sprintf("📝 搜索模式: %s\n", pattern))
	sb.WriteString(fmt.Sprintf("⚙️  参数: 上下文行数=%d, 大小写敏感=%v, 最大匹配数=%d\n", contextLines, caseSensitive, maxMatches))
	sb.WriteString(fmt.Sprintf("📊 匹配结果: %d 个\n\n", len(matches)))

	if len(matches) == 0 {
		sb.WriteString("❌ 未找到匹配项\n")
		return sb.String()
	}

	// 显示每个匹配项
	displayedLines := make(map[int]bool)
	for i, matchLine := range matches {
		sb.WriteString(fmt.Sprintf("### 匹配项 %d (第%d行)\n", i+1, matchLine))

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
				sb.WriteString(fmt.Sprintf("> %d: %s\n", lineNum, lines[lineNum-1]))
			} else {
				sb.WriteString(fmt.Sprintf("  %d: %s\n", lineNum, lines[lineNum-1]))
			}
		}

		sb.WriteString("\n")

		// 显示匹配行所在的代码结构信息
		structureInfo := getStructureInfoForLine(structure, matchLine)
		if structureInfo != "" {
			sb.WriteString(fmt.Sprintf("📋 结构信息: %s\n\n", structureInfo))
		}
	}

	// 显示统计信息
	sb.WriteString("📈 搜索统计:\n")
	sb.WriteString(fmt.Sprintf("  - 总行数: %d\n", len(lines)))
	sb.WriteString(fmt.Sprintf("  - 匹配数: %d\n", len(matches)))
	if maxMatches > 0 && len(matches) >= maxMatches {
		sb.WriteString(fmt.Sprintf("  - 注意: 已达到最大匹配数限制 (%d)\n", maxMatches))
	}

	return sb.String()
}

// getStructureInfoForLine 获取指定行所在的结构信息
func getStructureInfoForLine(structure *FileStructure, line int) string {
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
	RegisterTool(ToolDef{
		Name: "search_code_semantic",
		Description: `搜索文件中匹配指定模式的行，并显示上下文内容。
✅ 推荐：这是基于语义的新工具，比基于行号的搜索更智能、更准确。

参数：
  path: 文件路径（必需）
  pattern: 搜索模式（必需）
  context_lines: 上下文行数（可选，默认5）
  case_sensitive: 是否区分大小写（可选，默认false）
  max_matches: 最大匹配数（可选，默认无限制）

优势：
1. 基于语义搜索，能理解代码结构
2. 显示匹配行所在的函数、类、方法信息
3. 提供丰富的上下文信息
4. 比 search_file_with_pattern 更智能、更准确

功能特点：
1. 支持简单的字符串包含匹配
2. 显示匹配行及其上下文，便于理解上下文
3. 避免重复输出重叠的上下文区域
4. 支持大小写敏感/不敏感搜索
5. 可限制最大匹配数，避免输出过多内容
6. 显示匹配行所在的代码结构信息（函数、类、方法）

示例：
  # 搜索包含"error"的行，显示前后5行上下文
  search_code_semantic(path="main.go", pattern="error")
  
  # 搜索"TODO"注释，显示前后3行上下文
  search_code_semantic(path="main.go", pattern="TODO", context_lines="3")
  
  # 区分大小写搜索"Config"
  search_code_semantic(path="config.go", pattern="Config", case_sensitive="true")
  
  # 只显示前10个匹配项
  search_code_semantic(path="large.go", pattern="warning", max_matches="10")`,
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
		Category: "code_ops",
		Handler:  handleSearchCodeSemantic,
	})
}

func handleSearchCodeSemantic(ctx context.Context, args ToolArgs) (string, error) {
	path := ToolArgsValue(args, "path", "")
	if path == "" {
		return "", fmt.Errorf("参数 'path' 缺失")
	}
	pattern := ToolArgsValue(args, "pattern", "")
	if pattern == "" {
		return "", fmt.Errorf("参数 'pattern' 缺失")
	}

	// 解析可选参数
	contextLines := ToolArgsValue(args, "context_lines", 5)
	caseSensitive := ToolArgsValue(args, "case_sensitive", false)
	maxMatches := ToolArgsValue(args, "max_matches", 0)
	Printf("搜索文件%s中匹配指定模式%s的行", path, pattern)
	return searchCodeSemantic(path, pattern, contextLines, caseSensitive, maxMatches)
}
