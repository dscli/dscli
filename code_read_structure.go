package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

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
	structure, err := ParseFileStructure(ctx, path)
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
// buildStructureSummary 构建结构摘要
func buildStructureSummary(structure *FileStructure) string {
	var sb strings.Builder

	// 添加搜索图标，表明这是一个读取/搜索操作
	sb.WriteString(fmt.Sprintf("🔍 读取文件结构: %s\n", structure.FilePath))
	sb.WriteString(fmt.Sprintf("📝 语言: %s\n", structure.Language))

	if structure.Package != "" {
		sb.WriteString(fmt.Sprintf("📦 包名: %s\n", structure.Package))
	}

	if len(structure.Imports) > 0 {
		sb.WriteString(fmt.Sprintf("📚 导入: %d 个\n", len(structure.Imports)))
		for i, imp := range structure.Imports {
			if i < 3 { // 只显示前3个
				sb.WriteString(fmt.Sprintf("  - %s\n", imp))
			}
		}
		if len(structure.Imports) > 3 {
			sb.WriteString(fmt.Sprintf("  ... 还有 %d 个导入\n", len(structure.Imports)-3))
		}
	}

	if len(structure.Functions) > 0 {
		sb.WriteString(fmt.Sprintf("⚙️  函数: %d 个\n", len(structure.Functions)))
		for i, fn := range structure.Functions {
			if i < 5 { // 只显示前5个
				lineInfo := fmt.Sprintf("(第%d行", fn.Line)
				if fn.EndLine > fn.Line {
					lineInfo += fmt.Sprintf("-%d行", fn.EndLine)
				}
				lineInfo += ")"

				sb.WriteString(fmt.Sprintf("  - %s %s\n", fn.Name, lineInfo))
				if fn.Signature != "" {
					sb.WriteString(fmt.Sprintf("    签名: %s\n", fn.Signature))
				}
			}
		}
		if len(structure.Functions) > 5 {
			sb.WriteString(fmt.Sprintf("  ... 还有 %d 个函数\n", len(structure.Functions)-5))
		}
	}

	if len(structure.Classes) > 0 {
		sb.WriteString(fmt.Sprintf("🏗️  类/结构体: %d 个\n", len(structure.Classes)))
		for i, cls := range structure.Classes {
			if i < 5 { // 只显示前5个
				lineInfo := fmt.Sprintf("(第%d行", cls.Line)
				if cls.EndLine > cls.Line {
					lineInfo += fmt.Sprintf("-%d行", cls.EndLine)
				}
				lineInfo += ")"

				sb.WriteString(fmt.Sprintf("  - %s %s\n", cls.Name, lineInfo))
			}
		}
		if len(structure.Classes) > 5 {
			sb.WriteString(fmt.Sprintf("  ... 还有 %d 个类/结构体\n", len(structure.Classes)-5))
		}
	}

	if len(structure.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("⚠️  解析警告: %d 个\n", len(structure.Errors)))
		for i, err := range structure.Errors {
			if i < 3 {
				sb.WriteString(fmt.Sprintf("  - %s\n", err))
			}
		}
	}

	return sb.String()
}

func init() {
	// 注册 readCodeStructure 工具
	RegisterTool(ToolDef{
		Name: "read_code_structure",
		Description: `读取代码文件的结构信息（函数、类、方法、导入等）。返回人类可读的摘要和完整的JSON结构信息。
✅ 推荐：这是基于代码结构的新工具，为代码操作提供基础信息。

参数：
  path: 文件路径（相对于项目根目录）

功能：
1. 读取文件的完整结构信息
2. 提供人类可读的摘要
3. 返回详细的JSON格式结构信息
4. 支持多种编程语言（通过文件结构解析）

输出包含：
1. 文件基本信息（路径、语言、包名）
2. 导入列表
3. 函数/方法列表（包含行号、签名等信息）
4. 类/结构体列表
5. 解析警告（如果有）
6. 完整的JSON结构信息

优势：
1. 为 write_code_section 和 read_code_section 提供基础信息
2. 帮助理解代码文件的结构
3. 支持多种编程语言
4. 提供详细的结构信息，便于后续代码操作

示例：
  # 读取main.go文件的结构信息
  read_code_structure(path="main.go")
  
  # 读取user.py文件的结构信息
  read_code_structure(path="user.py")`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录）",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleReadCodeStructure,
	})
}

func handleReadCodeStructure(ctx context.Context, args ToolArgs) (string, error) {
	path := ToolArgsValue(args, "path", "")
	if path == "" {
		return "", fmt.Errorf("参数 'path' 缺失")
	}
	return readCodeStructure(ctx, path)
}
