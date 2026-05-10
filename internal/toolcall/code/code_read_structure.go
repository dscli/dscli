package code

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gitcode.com/dscli/dscli/internal/parse"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed code_read_structure.md
var code_read_structure_md string

// 这个工具让LLM能够获取代码文件的结构信息（函数、类、方法等），
// 为后续的代码操作提供基础。
func readCodeStructure(ctx context.Context, path string) (string, error) {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return "", fmt.Errorf("文件不存在: %s", path)
	}

	// 读取文件内容
	// content, err := os.ReadFile(path)
	// if err != nil {
	// 	return "", fmt.Errorf("读取文件失败: %w", err)
	// }

	// 解析文件结构
	structure, err := parse.ParseFileStructure(ctx, path)
	if err != nil {
		return "", fmt.Errorf("解析文件结构失败: %w", err)
	}

	// 将结构转换为JSON字符串
	jsonBytes, err := json.MarshalIndent(structure, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化结构失败: %w", err)
	}

	// 构建人类可读的摘要
	summary := buildStructureSummary(structure)

	return fmt.Sprintf("%s\n\n完整结构信息（JSON格式）:\n%s", summary, string(jsonBytes)), nil
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
