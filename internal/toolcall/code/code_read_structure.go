package code

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/parse"
	"gitcode.com/dscli/dscli/internal/toolcall"
)
//go:embed code_read_structure.md
var code_read_structure_md string

// 这个工具让LLM能够获取代码文件的结构信息（函数、类、方法等），
// 为后续的代码操作提供基础。
// 支持单文件和目录：目录时聚合展示所有代码文件的结构。
func readCodeStructure(ctx context.Context, path string) (string, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", fmt.Errorf("路径不存在: %s", path)
	}
	if err != nil {
		return "", fmt.Errorf("访问路径失败: %w", err)
	}

	if info.IsDir() {
		return readCodeStructureDir(ctx, path)
	}
	return readCodeStructureFile(ctx, path)
}

// readCodeStructureFile 解析单个文件的结构。
func readCodeStructureFile(ctx context.Context, path string) (string, error) {
	structure, err := parse.ParseFileStructure(ctx, path)
	if err != nil {
		return "", fmt.Errorf("解析文件结构失败: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(structure, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结构失败: %w", err)
	}

	summary := buildStructureSummary(structure)
	return fmt.Sprintf("%s\n\n完整结构信息（JSON格式）:\n%s", summary, string(jsonBytes)), nil
}

// readCodeStructureDir 解析目录下所有代码文件的结构，聚合展示。
func readCodeStructureDir(ctx context.Context, dirPath string) (string, error) {
	files, err := resolveDefSearchFiles(dirPath)
	if err != nil {
		return "", fmt.Errorf("解析目录失败: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "📂 目录结构: %s", dirPath)

	if len(files) == 0 {
		sb.WriteString("\n❌ 未找到代码文件")
		return sb.String(), nil
	}

	fmt.Fprintf(&sb, " (%d 个文件)\n\n", len(files))

	totalFuncs := 0
	totalClasses := 0
	var parseErrors []string

	for _, f := range files {
		structure, parseErr := parse.ParseFileStructure(ctx, f)
		if parseErr != nil {
			parseErrors = append(parseErrors,
				fmt.Sprintf("  - %s: %v", filepath.Base(f), parseErr))
			continue
		}

		baseName := filepath.Base(f)
		fmt.Fprintf(&sb, "### %s\n", baseName)
		fmt.Fprintf(&sb, "📝 语言: %s", structure.Language)
		if structure.Package != "" {
			fmt.Fprintf(&sb, " | 📦 包: %s", structure.Package)
		}
		sb.WriteString("\n")

		if len(structure.Functions) > 0 {
			fmt.Fprintf(&sb, "⚙️  函数 (%d):\n", len(structure.Functions))
			for _, fn := range structure.Functions {
				fmt.Fprintf(&sb, "  - %s", fn.Name)
				if fn.Signature != "" {
					fmt.Fprintf(&sb, " %s", fn.Signature)
				}
				fmt.Fprintf(&sb, " (:%d)\n", fn.Line)
			}
			totalFuncs += len(structure.Functions)
		}

		if len(structure.Classes) > 0 {
			fmt.Fprintf(&sb, "🏗️  类/结构体 (%d):\n", len(structure.Classes))
			for _, cls := range structure.Classes {
				fmt.Fprintf(&sb, "  - %s (:%d)\n", cls.Name, cls.Line)
			}
			totalClasses += len(structure.Classes)
		}
		sb.WriteString("\n")
	}

	fmt.Fprintf(&sb, "📊 合计: %d 文件, %d 函数, %d 类/结构体\n",
		len(files), totalFuncs, totalClasses)

	if len(parseErrors) > 0 {
		sb.WriteString("\n⚠️ 解析错误:\n")
		for _, e := range parseErrors {
			sb.WriteString(e + "\n")
		}
	}

	return sb.String(), nil
}

// buildStructureSummary 构建结构摘要
func buildStructureSummary(structure *parse.FileStructure) string {
	var sb strings.Builder

	// 添加搜索图标，表明这是一个读取/搜索操作
	fmt.Fprintf(&sb, "🔍 读取文件结构: %s\n", structure.FilePath)
	fmt.Fprintf(&sb, "📝 语言: %s\n", structure.Language)

	if structure.Package != "" {
		fmt.Fprintf(&sb, "📦 包名: %s\n", structure.Package)
	}

	if len(structure.Imports) > 0 {
		fmt.Fprintf(&sb, "📚 导入: %d 个\n", len(structure.Imports))
		for i, imp := range structure.Imports {
			if i < 3 { // 只显示前3个
				fmt.Fprintf(&sb, "  - %s\n", imp)
			}
		}
		if len(structure.Imports) > 3 {
			fmt.Fprintf(&sb, "  ... 还有 %d 个导入\n", len(structure.Imports)-3)
		}
	}

	if len(structure.Functions) > 0 {
		fmt.Fprintf(&sb, "⚙️  函数: %d 个\n", len(structure.Functions))
		for i, fn := range structure.Functions {
			if i < 5 { // 只显示前5个
				lineInfo := fmt.Sprintf("(第%d行", fn.Line)
				if fn.EndLine > fn.Line {
					lineInfo += fmt.Sprintf("-%d行", fn.EndLine)
				}
				lineInfo += ")"

				fmt.Fprintf(&sb, "  - %s %s\n", fn.Name, lineInfo)
				if fn.Signature != "" {
					fmt.Fprintf(&sb, "    签名: %s\n", fn.Signature)
				}
			}
		}
		if len(structure.Functions) > 5 {
			fmt.Fprintf(&sb, "  ... 还有 %d 个函数\n", len(structure.Functions)-5)
		}
	}

	if len(structure.Classes) > 0 {
		fmt.Fprintf(&sb, "🏗️  类/结构体: %d 个\n", len(structure.Classes))
		for i, cls := range structure.Classes {
			if i < 5 { // 只显示前5个
				lineInfo := fmt.Sprintf("(第%d行", cls.Line)
				if cls.EndLine > cls.Line {
					lineInfo += fmt.Sprintf("-%d行", cls.EndLine)
				}
				lineInfo += ")"

				fmt.Fprintf(&sb, "  - %s %s\n", cls.Name, lineInfo)
			}
		}
		if len(structure.Classes) > 5 {
			fmt.Fprintf(&sb, "  ... 还有 %d 个类/结构体\n", len(structure.Classes)-5)
		}
	}

	if len(structure.Errors) > 0 {
		fmt.Fprintf(&sb, "⚠️  解析警告: %d 个\n", len(structure.Errors))
		for i, err := range structure.Errors {
			if i < 3 {
				fmt.Fprintf(&sb, "  - %s\n", err)
			}
		}
	}

	return sb.String()
}

func init() {
	// 注册 readCodeStructure 工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "read_code_structure",
		Description: code_read_structure_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path (relative to project root)",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleReadCodeStructure,
	})
}

func handleReadCodeStructure(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		result, err = "", fmt.Errorf("参数 'path' 缺失")
		return result, warning, err
	}
	result, err = readCodeStructure(ctx, path)
	return result, warning, err
}
