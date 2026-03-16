package main

import (
	"context"
	"fmt"
	"strings"
)

func init() {
	// 注册代码定义搜索工具
	RegisterTool(ToolDef{
		Name: "search_code_definition",
		Description: `搜索代码文件中的定义（函数、方法、类、结构体等）。

参数：
  path: 必需，文件路径
  pattern: 必需，搜索模式（支持部分匹配）
  type_filter: 可选，类型过滤器，如 "function", "method", "class", "struct" 等
  case_sensitive: 可选，是否区分大小写，默认为 false

功能：
1. 搜索代码文件中的定义（函数、方法、类等）
2. 支持类型过滤，只搜索特定类型的定义
3. 显示定义的详细信息（名称、类型、位置、签名等）
4. 基于代码结构解析，比文本搜索更精确

示例：
  # 搜索所有包含"user"的定义
  search_code_definition(path="user.go", pattern="user")
  
  # 只搜索函数定义
  search_code_definition(path="main.go", pattern="handle", type_filter="function")
  
  # 区分大小写搜索
  search_code_definition(path="config.go", pattern="Config", case_sensitive="true")
  
  # 搜索所有方法
  search_code_definition(path="service.go", pattern="", type_filter="method")`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"pattern": map[string]any{
					"type":        "string",
					"description": "搜索模式（支持部分匹配）",
				},
				"type_filter": map[string]any{
					"type":        "string",
					"description": "类型过滤器，如 function, method, class, struct",
				},
				"case_sensitive": map[string]any{
					"type":        "boolean",
					"description": "是否区分大小写，默认为false",
				},
			},
			"required":             []string{"path", "pattern"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleSearchCodeDefinition,
	})
}

// handleSearchCodeDefinition 处理代码定义搜索请求
func handleSearchCodeDefinition(ctx context.Context, args ToolArgs) (string, error) {
	path := ToolArgsValue(args, "path", "")
	if path == "" {
		return "", fmt.Errorf("参数 'path' 缺失")
	}
	pattern := ToolArgsValue(args, "pattern", "")
	if pattern == "" {
		return "", fmt.Errorf("参数 'pattern' 缺失")
	}
	typeFilter := ToolArgsValue(args, "type_filter", "")
	caseSensitive := ToolArgsValue(args, "case_sensitive", false)

	// 解析文件结构
	structure, err := ParseFileStructure(ctx, path)
	if err != nil {
		return "", fmt.Errorf("解析文件结构失败: %w", err)
	}

	// 准备搜索
	searchPattern := pattern
	if !caseSensitive {
		searchPattern = strings.ToLower(pattern)
	}

	// 搜索函数
	var results []string
	matchCount := 0

	// 搜索函数和方法
	for _, fn := range structure.Functions {
		if matchesDefinition(fn, searchPattern, typeFilter, caseSensitive) {
			matchCount++
			result := formatSymbolResult(fn, "函数", path)
			results = append(results, result)
		}
	}

	// 搜索类和结构体
	for _, cls := range structure.Classes {
		if matchesDefinition(cls, searchPattern, typeFilter, caseSensitive) {
			matchCount++
			result := formatSymbolResult(cls, "类/结构体", path)
			results = append(results, result)
		}
	}

	// 构建结果
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔍 在文件 %s 中搜索定义\n", path))
	sb.WriteString(fmt.Sprintf("📝 搜索模式: %s\n", pattern))
	if typeFilter != "" {
		sb.WriteString(fmt.Sprintf("⚙️  类型过滤: %s\n", typeFilter))
	}
	sb.WriteString(fmt.Sprintf("📊 匹配结果: %d 个\n\n", matchCount))

	if matchCount == 0 {
		sb.WriteString("❌ 未找到匹配的定义\n")
		// 提供一些建议
		sb.WriteString("\n💡 建议:\n")
		sb.WriteString("1. 检查搜索模式是否正确\n")
		sb.WriteString("2. 尝试不区分大小写搜索\n")
		sb.WriteString("3. 尝试不使用类型过滤器\n")
		sb.WriteString("4. 使用 search_code_semantic 进行文本搜索\n")
		return sb.String(), nil
	}

	// 显示所有匹配结果
	for i, result := range results {
		sb.WriteString(fmt.Sprintf("### 匹配项 %d\n", i+1))
		sb.WriteString(result)
		sb.WriteString("\n")
	}

	// 显示统计信息
	sb.WriteString("📈 搜索统计:\n")
	sb.WriteString(fmt.Sprintf("  - 总函数数: %d\n", len(structure.Functions)))
	sb.WriteString(fmt.Sprintf("  - 总类/结构体数: %d\n", len(structure.Classes)))
	sb.WriteString(fmt.Sprintf("  - 匹配定义数: %d\n", matchCount))

	return sb.String(), nil
}

// matchesDefinition 检查符号是否匹配搜索条件
func matchesDefinition(symbol *Symbol, pattern, typeFilter string, caseSensitive bool) bool {
	// 类型过滤
	if typeFilter != "" {
		if !strings.EqualFold(symbol.Type, typeFilter) {
			return false
		}
	}

	// 名称匹配
	nameToCheck := symbol.Name
	if !caseSensitive {
		nameToCheck = strings.ToLower(nameToCheck)
	}

	return strings.Contains(nameToCheck, pattern)
}

// formatSymbolResult 格式化符号结果
func formatSymbolResult(symbol *Symbol, symbolType, filePath string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("📋 类型: %s\n", symbolType))
	sb.WriteString(fmt.Sprintf("📝 名称: %s\n", symbol.Name))

	if symbol.Signature != "" {
		sb.WriteString(fmt.Sprintf("🖋️  签名: %s\n", symbol.Signature))
	}

	sb.WriteString(fmt.Sprintf("📍 位置: %s:%d:%d\n", filePath, symbol.Line, symbol.Column))

	if symbol.EndLine > symbol.Line {
		sb.WriteString(fmt.Sprintf("📏 范围: 第%d行 - 第%d行\n", symbol.Line, symbol.EndLine))
	}

	if symbol.Receiver != "" {
		sb.WriteString(fmt.Sprintf("🎯 接收器: %s\n", symbol.Receiver))
	}

	return sb.String()
}
