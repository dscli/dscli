package code

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/parse"
	"github.com/dscli/dscli/internal/toolcall"
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
					"description": "File or directory path, e.g. main.go or ./internal",
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

// defSearchResult holds search results for a single file.
type defSearchResult struct {
	path       string
	structure  *parse.FileStructure
	results    []string
	matchCount int
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

	// 解析路径：单文件或目录 → 文件列表
	files, resolveErr := resolveDefSearchFiles(path)
	if resolveErr != nil {
		err = fmt.Errorf("解析路径失败: %w", resolveErr)
		return result, warning, err
	}

	outfmt.Printf("🔍 搜索代码定义: path=%s pattern=%s typeFilter=%s caseSensitive=%v files=%d\n",
		path, pattern, typeFilter, caseSensitive, len(files))

	// 逐个文件搜索
	var allResults []defSearchResult
	var searchErrors []string
	totalMatches := 0
	for _, f := range files {
		r, searchErr := searchDefInFile(ctx, f, pattern, typeFilter, caseSensitive)
		if searchErr != nil {
			searchErrors = append(searchErrors, fmt.Sprintf("  - %s: %v", f, searchErr))
			continue
		}
		if r.matchCount > 0 {
			allResults = append(allResults, *r)
			totalMatches += r.matchCount
		}
	}

	// 构建输出
	var sb strings.Builder
	fmt.Fprintf(&sb, "🔍 搜索代码定义\n")
	fmt.Fprintf(&sb, "📝 搜索模式: %s\n", pattern)
	if len(files) > 1 {
		fmt.Fprintf(&sb, "📂 搜索范围: %s (%d 个文件)\n", path, len(files))
	} else if len(files) == 1 {
		fmt.Fprintf(&sb, "📂 文件: %s\n", files[0])
	} else {
		fmt.Fprintf(&sb, "📂 搜索范围: %s (0 个文件)\n", path)
	}
	if typeFilter != "" {
		fmt.Fprintf(&sb, "⚙️  类型过滤: %s\n", typeFilter)
	}
	if caseSensitive {
		sb.WriteString("🔤 大小写敏感: 是\n")
	} else {
		sb.WriteString("🔤 大小写敏感: 否\n")
	}

	// 搜索错误
	if len(searchErrors) > 0 {
		sb.WriteString("\n⚠️ 部分文件解析失败:\n")
		for _, e := range searchErrors {
			sb.WriteString(e + "\n")
		}
	}

	fmt.Fprintf(&sb, "\n📊 匹配结果: %d 个 (共 %d 个文件)\n\n", totalMatches, len(files))

	if totalMatches == 0 {
		sb.WriteString("❌ 未找到匹配的定义\n")
		sb.WriteString("\n💡 建议:\n")
		sb.WriteString("1. 检查搜索模式是否正确\n")
		sb.WriteString("2. 尝试不区分大小写搜索\n")
		sb.WriteString("3. 尝试不使用类型过滤器\n")
		sb.WriteString("4. 使用 search_code_semantic 进行文本搜索\n")
		if len(files) == 1 {
			sb.WriteString("5. 查看文件结构: read_code_structure(path=\"" + files[0] + "\")\n")
		}
		result = sb.String()
		return result, warning, err
	}

	// 逐文件显示匹配
	for i, r := range allResults {
		if len(allResults) > 1 {
			fmt.Fprintf(&sb, "### %s (%d 个匹配)\n", r.path, r.matchCount)
		}
		for j, match := range r.results {
			fmt.Fprintf(&sb, "#### 匹配项 %d\n", j+1)
			sb.WriteString(match)
			sb.WriteString("\n")
		}
		if i < len(allResults)-1 {
			sb.WriteString("---\n\n")
		}
	}

	// 统计摘要
	sb.WriteString("\n📈 搜索统计:\n")
	totalFuncs := 0
	totalClasses := 0
	for _, r := range allResults {
		totalFuncs += len(r.structure.Functions)
		totalClasses += len(r.structure.Classes)
	}
	fmt.Fprintf(&sb, "  - 搜索文件数: %d\n", len(files))
	fmt.Fprintf(&sb, "  - 成功解析: %d\n", len(allResults))
	fmt.Fprintf(&sb, "  - 总函数/方法数: %d\n", totalFuncs)
	fmt.Fprintf(&sb, "  - 总类/结构体数: %d\n", totalClasses)
	fmt.Fprintf(&sb, "  - 匹配定义数: %d\n", totalMatches)
	totalDefs := totalFuncs + totalClasses
	if totalDefs > 0 {
		fmt.Fprintf(&sb, "  - 匹配率: %.1f%%\n", float64(totalMatches)/float64(totalDefs)*100)
	}

	result = sb.String()
	return result, warning, err
}

// searchDefInFile parses a single file and searches for matching definitions.
func searchDefInFile(ctx context.Context, filePath, pattern, typeFilter string, caseSensitive bool) (*defSearchResult, error) {
	structure, err := parse.ParseFileStructure(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("解析文件结构失败: %w", err)
	}

	searchPattern := pattern
	if !caseSensitive {
		searchPattern = strings.ToLower(pattern)
	}

	var results []string
	matchCount := 0

	for _, fn := range structure.Functions {
		if matchesDefinition(fn, searchPattern, typeFilter, caseSensitive) {
			matchCount++
			results = append(results, formatSymbolResult(fn, "函数", filePath))
		}
	}

	for _, cls := range structure.Classes {
		if matchesDefinition(cls, searchPattern, typeFilter, caseSensitive) {
			matchCount++
			results = append(results, formatSymbolResult(cls, "类/结构体", filePath))
		}
	}

	return &defSearchResult{
		path:       filePath,
		structure:  structure,
		results:    results,
		matchCount: matchCount,
	}, nil
}

// resolveDefSearchFiles resolves path to a list of code files.
// If path is a file, returns [path]. If path is a directory, returns all
// code files (filtered by extension) in that directory (non-recursive).
func resolveDefSearchFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{path}, nil
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	var files []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		// 跳过隐藏文件和非代码文件
		if strings.HasPrefix(name, ".") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch ext {
		case ".go", ".py", ".js", ".mjs", ".ts", ".tsx",
			".java", ".cpp", ".cc", ".cxx", ".h", ".hpp", ".c",
			".rs", ".rb", ".php", ".swift", ".kt", ".kts", ".scala",
			".sh", ".bash", ".el", ".vim":
			files = append(files, filepath.Join(path, name))
		}
	}

	return files, nil
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
