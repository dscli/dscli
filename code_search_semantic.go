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
//	searchPattern: 搜索模式（字符串包含匹配）
//	contextLines: 上下文行数（前后各N行）
//	caseSensitive: 是否区分大小写
//	maxMatches: 最大匹配数
func searchCodeSemantic(ctx context.Context, path string, pattern string, contextLines int, caseSensitive bool, maxMatches int) (string, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("文件不存在: %s", path)
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}

	// 解析文件结构
	structure, err := ParseFileStructure(ctx, path)
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

	fmt.Fprintf(&sb, "🔍 搜索文件: %s\n", path)
	fmt.Fprintf(&sb, "📝 搜索模式: %s\n", pattern)
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
			sb.WriteString(fmt.Sprintf("📋 结构信息: %s\n\n", structureInfo))
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

参数：
  file_pattern: 文件搜索模式（必需），支持：
    1. 单个文件：main.go
    2. 通配符：*.go (当前目录)
    3. 多个文件：main.go root.go
    4. 当前目录：. (当前目录所有非隐藏文件)
    5. 递归搜索：**/*.go (所有子目录)
  search_pattern: 搜索模式（必需）
  context_lines: 上下文行数（可选，默认5）
  case_sensitive: 是否区分大小写（可选，默认false）
  max_matches: 最大匹配数（可选，默认无限制）

优势：
1. 基于语义搜索，能理解代码结构
2. 显示匹配行所在的函数、类、方法信息
3. 提供丰富的上下文信息

示例：
  # 搜索当前目录所有.go文件中的"error"
  search_code_semantic(file_pattern="*.go", search_pattern="error")
  
  # 搜索main.go和root.go中的"TODO"注释
  search_code_semantic(file_pattern="main.go root.go", search_pattern="TODO", context_lines="3")
  
  # 搜索当前目录所有文件中的"Config"（区分大小写）
  search_code_semantic(file_pattern=".", search_pattern="Config", case_sensitive="true")`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"file_pattern": map[string]any{
					"type":        "string",
					"description": "文件搜索模式，支持：单个文件、通配符、多个文件、当前目录、递归搜索",
					"pattern":     TitleLikePattern(128),
				},
				"search_pattern": map[string]any{
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
			"required":             []string{"file_pattern", "search_pattern"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleSearchCodeSemantic,
	})
}

func handleSearchCodeSemantic(ctx context.Context, args ToolArgs) (string, error) {
	filePattern := ToolArgsValue(args, "file_pattern", "")
	if filePattern == "" {
		return "", fmt.Errorf("参数 'file_pattern' 缺失")
	}
	searchPattern := ToolArgsValue(args, "search_pattern", "")
	if searchPattern == "" {
		return "", fmt.Errorf("参数 'search_pattern' 缺失")
	}

	// 解析可选参数
	contextLines := ToolArgsValue(args, "context_lines", 5)
	caseSensitive := ToolArgsValue(args, "case_sensitive", false)
	maxMatches := ToolArgsValue(args, "max_matches", 0)

	// 扩展文件模式
	files, err := expandFilePattern(filePattern)
	if err != nil {
		return "", fmt.Errorf("扩展文件模式失败: %w", err)
	}

	if len(files) == 0 {
		return "❌ 未找到匹配的文件", nil
	}

	Printf("🔍 搜索%d个文件中匹配指定模式%s的行\n", len(files), searchPattern)

	// 搜索所有文件
	var results []string
	for _, file := range files {
		result, err := searchCodeSemantic(ctx, file, searchPattern, contextLines, caseSensitive, maxMatches)
		if err != nil {
			results = append(results, fmt.Sprintf("❌ 搜索文件 %s 失败: %v", file, err))
		} else {
			results = append(results, result)
		}
	}

	return strings.Join(results, "\n\n---\n\n"), nil
}
