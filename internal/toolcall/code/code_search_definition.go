package code

import (
	"context"
	_ "embed"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/parse"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed code_search_definition.md
var code_search_definition_md string

func init() {
	// 注册代码定义搜索工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "search_code_definition",
		Description: code_search_definition_md,
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
					"description": "Search pattern (supports partial match)",
				},
				"type_filter": map[string]any{
					"type":        "string",
					"description": "Type filter: function, method, class, struct",
				},
				"case_sensitive": map[string]any{
					"type":        "boolean",
					"description": "Case-sensitive search, default false",
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
func handleSearchCodeDefinition(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("参数 'path' 缺失")
		return result, warning, err
	}
	pattern := toolcall.ToolArgsValue(args, "pattern", "")
	if pattern == "" {
		err = fmt.Errorf("参数 'pattern' 缺失")
		return result, warning, err
	}
	typeFilter := toolcall.ToolArgsValue(args, "type_filter", "")
	caseSensitive := toolcall.ToolArgsValue(args, "case_sensitive", false)
	outfmt.Printf("🔍 搜索代码定义: path=%s pattern=%s typeFilter=%s caseSensitive=%v\n", path, pattern, typeFilter, caseSensitive)
	// 解析文件结构
	structure, err := parse.ParseFileStructure(ctx, path)
	if err != nil {
		err = fmt.Errorf("解析文件结构失败: %w", err)
		return result, warning, err
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
	fmt.Fprintf(&sb, "🔍 在文件 %s 中搜索定义\n", path)
	fmt.Fprintf(&sb, "📝 搜索模式: %s\n", pattern)

	if typeFilter != "" {
		fmt.Fprintf(&sb, "⚙️  类型过滤: %s\n", typeFilter)
	}

	if caseSensitive {
		sb.WriteString("🔤 大小写敏感: 是\n")
	} else {
		sb.WriteString("🔤 大小写敏感: 否\n")
	}

	fmt.Fprintf(&sb, "📊 匹配结果: %d 个\n\n", matchCount)

	if matchCount == 0 {
		sb.WriteString("❌ 未找到匹配的定义\n")

		// 显示文件中的可用定义类型
		sb.WriteString("\n📋 文件中的定义类型:\n")
		if len(structure.Functions) > 0 {
			fmt.Fprintf(&sb, "  - 函数: %d 个\n", len(structure.Functions))
		}
		if len(structure.Classes) > 0 {
			fmt.Fprintf(&sb, "  - 类/结构体: %d 个\n", len(structure.Classes))
		}

		// 提供一些建议
		sb.WriteString("\n💡 建议:\n")
		sb.WriteString("1. 检查搜索模式是否正确\n")
		sb.WriteString("2. 尝试不区分大小写搜索\n")
		sb.WriteString("3. 尝试不使用类型过滤器\n")
		sb.WriteString("4. 使用 search_code_semantic 进行文本搜索\n")
		sb.WriteString("5. 查看文件结构: read_code_structure(path=\"" + path + "\")\n")
		result = sb.String()
		return result, warning, err
	}

	// 显示所有匹配结果
	for i, result := range results {
		fmt.Fprintf(&sb, "### 匹配项 %d\n", i+1)
		sb.WriteString(result)
		sb.WriteString("\n")
	}

	// 显示统计信息
	sb.WriteString("📈 搜索统计:\n")
	fmt.Fprintf(&sb, "  - 总函数数: %d\n", len(structure.Functions))
	fmt.Fprintf(&sb, "  - 总类/结构体数: %d\n", len(structure.Classes))
	fmt.Fprintf(&sb, "  - 匹配定义数: %d\n", matchCount)

	// 显示搜索效率
	totalDefinitions := len(structure.Functions) + len(structure.Classes)
	if totalDefinitions > 0 {
		efficiency := float64(matchCount) / float64(totalDefinitions) * 100
		fmt.Fprintf(&sb, "  - 搜索效率: %.1f%%\n", efficiency)
	}

	result = sb.String()
	return result, warning, err
}

// matchesDefinition 检查符号是否匹配搜索条件
func matchesDefinition(symbol *parse.Symbol, pattern, typeFilter string, caseSensitive bool) bool {
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
func formatSymbolResult(symbol *parse.Symbol, symbolType, filePath string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "📋 类型: %s\n", symbolType)
	fmt.Fprintf(&sb, "📝 名称: %s\n", symbol.Name)

	// 显示函数/方法的详细信息
	if symbol.Signature != "" {
		fmt.Fprintf(&sb, "🖋️  签名: %s\n", symbol.Signature)
	}

	fmt.Fprintf(&sb, "📍 位置: %s:%d:%d\n", filePath, symbol.Line, symbol.Column)

	if symbol.EndLine > symbol.Line {
		fmt.Fprintf(&sb, "📏 范围: 第%d行 - 第%d行\n", symbol.Line, symbol.EndLine)
	}

	if symbol.Receiver != "" {
		fmt.Fprintf(&sb, "🎯 接收器: %s\n", symbol.Receiver)
	}

	// 添加更多上下文信息
	if symbol.Type != "" {
		fmt.Fprintf(&sb, "🔧 符号类型: %s\n", symbol.Type)
	}

	// 计算代码行数
	if symbol.EndLine > symbol.Line {
		lineCount := symbol.EndLine - symbol.Line + 1
		fmt.Fprintf(&sb, "📊 代码行数: %d 行\n", lineCount)
	}

	return sb.String()
}