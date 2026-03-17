package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// writeCodeSection 基于代码结构定位并修改特定代码片段
// selector语法：
//
//	function:函数名      - 修改指定函数
//	class:类名          - 修改指定类/结构体
//	method:类名.方法名   - 修改指定方法
//	lines:开始行-结束行  - 修改指定行范围（后备方案）
//
// writeCodeSection 基于代码结构定位并修改特定代码片段
func writeCodeSection(ctx context.Context, path string, selector string, newContent string) (result string, err error) {
	// 检查文件是否存在
	if _, err = os.Stat(path); os.IsNotExist(err) {
		err = fmt.Errorf("文件不存在: %s", path)
		return
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("读取文件失败: %w", err)
		return
	}

	defer func() {
		output, fmtErr := CodeMakeFormat(ctx)
		if fmtErr != nil {
			if err != nil {
				err = fmt.Errorf("original error: %w, make format error: %w, make format output: %s", err, fmtErr, output)
			} else {
				err = fmt.Errorf("make format error: %w, make format output: %s", fmtErr, output)
			}
		}
		output = strings.TrimSpace(output)
		if output != "" {
			result = fmt.Sprintf("%s\nmake format result:\n%s", result, output)
		}
	}()

	// 解析文件结构
	structure, err := ParseFileStructure(ctx, path)
	if err != nil {
		err = fmt.Errorf("解析文件结构失败: %w", err)
		return
	}

	// 根据selector定位代码片段
	lines := strings.Split(string(content), "\n")
	startLine, endLine, err := locateSectionRange(structure, lines, selector)
	if err != nil {
		err = fmt.Errorf("获取区域范围失败: %w", err)
		return
	}

	// 构建结果
	result = buildWriteResult(path, selector, startLine, endLine, lines, newContent)

	if err = writeToFile(path, lines, startLine, endLine, newContent); err != nil {
		err = fmt.Errorf("写入文件失败: %w", err)
		return
	}
	result += "\n✅ 文件已成功更新"

	return
}

// locateSectionRange 根据selector定位代码片段的范围
func locateSectionRange(structure *FileStructure, lines []string, selector string) (int, int, error) {
	// 解析selector
	parts := strings.SplitN(selector, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("无效的selector格式，应为'类型:名称'，例如'function:main'")
	}

	selectorType := strings.TrimSpace(parts[0])
	selectorValue := strings.TrimSpace(parts[1])

	switch selectorType {
	case "function":
		return locateFunctionRange(structure, selectorValue)
	case "class", "struct":
		return locateClassRange(structure, selectorValue)
	case "method":
		return locateMethodRange(structure, selectorValue)
	case "lines":
		return locateLinesRange(lines, selectorValue)
	default:
		return 0, 0, fmt.Errorf("不支持的selector类型: %s，支持的类型: function, class, struct, method, lines", selectorType)
	}
}

// locateFunctionRange 定位函数的行范围
func locateFunctionRange(structure *FileStructure, functionName string) (int, int, error) {
	for _, fn := range structure.Functions {
		if fn.Name == functionName {
			return fn.Line, fn.EndLine, nil
		}
	}
	return 0, 0, fmt.Errorf("未找到函数: %s", functionName)
}

// locateClassRange 定位类/结构体的行范围
func locateClassRange(structure *FileStructure, className string) (int, int, error) {
	for _, cls := range structure.Classes {
		if cls.Name == className {
			return cls.Line, cls.EndLine, nil
		}
	}
	return 0, 0, fmt.Errorf("未找到类/结构体: %s", className)
}

// locateMethodRange 定位方法的行范围
func locateMethodRange(structure *FileStructure, methodSelector string) (int, int, error) {
	// 方法选择器格式: 类名.方法名
	parts := strings.Split(methodSelector, ".")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("方法选择器格式错误，应为'类名.方法名'")
	}

	className := parts[0]
	methodName := parts[1]

	// 在函数列表中查找方法
	for _, fn := range structure.Functions {
		if fn.Type == "method" && fn.Receiver == className && fn.Name == methodName {
			return fn.Line, fn.EndLine, nil
		}
	}

	return 0, 0, fmt.Errorf("在类 %s 中未找到方法: %s", className, methodName)
}

// locateLinesRange 定位行范围（后备方案）
func locateLinesRange(lines []string, lineSelector string) (int, int, error) {
	// 行选择器格式: 开始行-结束行
	parts := strings.Split(lineSelector, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("行选择器格式错误，应为'开始行-结束行'")
	}

	startLine, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	endLine, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return 0, 0, fmt.Errorf("行号必须为数字")
	}

	if startLine < 1 || endLine > len(lines) || startLine > endLine {
		return 0, 0, fmt.Errorf("行号范围无效: %d-%d (文件总行数: %d)", startLine, endLine, len(lines))
	}

	return startLine, endLine, nil
}

// buildWriteResult 构建写入结果信息
func buildWriteResult(path, selector string, startLine, endLine int, lines []string, newContent string) string {
	var sb strings.Builder

	fmt.Fprintf(&sb, "📝 文件: %s\n", path)
	fmt.Fprintf(&sb, "🎯 选择器: %s\n", selector)
	fmt.Fprintf(&sb, "📏 范围: 第%d行 - 第%d行\n", startLine, endLine)

	// 显示原内容
	sb.WriteString("\n📄 原内容:\n")
	sb.WriteString("```\n")
	for i := startLine - 1; i < endLine; i++ {
		fmt.Fprintf(&sb, "%d: %s\n", i+1, lines[i])
	}
	sb.WriteString("```\n")

	// 显示新内容
	sb.WriteString("\n🔄 新内容:\n")
	sb.WriteString("```\n")
	newLines := strings.Split(newContent, "\n")
	for i, line := range newLines {
		fmt.Fprintf(&sb, "%d: %s\n", startLine+i, line)
	}
	sb.WriteString("```\n")

	// 显示差异
	sb.WriteString("\n📊 差异:\n")
	oldLineCount := endLine - startLine + 1
	newLineCount := len(newLines)
	fmt.Fprintf(&sb, "  - 原行数: %d\n", oldLineCount)
	fmt.Fprintf(&sb, "  - 新行数: %d\n", newLineCount)
	if oldLineCount != newLineCount {
		fmt.Fprintf(&sb, "  - 行数变化: %+d\n", newLineCount-oldLineCount)
	}

	return sb.String()
}

// writeToFile 将修改写入文件
func writeToFile(path string, lines []string, startLine, endLine int, newContent string) error {
	// 构建新文件内容
	var newLines []string

	// 添加开始行之前的内容
	newLines = append(newLines, lines[:startLine-1]...)

	// 添加新内容
	newLines = append(newLines, strings.Split(newContent, "\n")...)

	// 添加结束行之后的内容
	newLines = append(newLines, lines[endLine:]...)

	// 写入文件
	newContentStr := strings.Join(newLines, "\n")
	return os.WriteFile(path, []byte(newContentStr), 0o644)
}

func init() {
	// 注册 writeCodeSection 工具
	RegisterTool(ToolDef{
		Name: "write_code_section",
		Description: `基于代码结构定位并修改特定代码片段。支持function:函数名、class:类名、method:类名.方法名、lines:开始行-结束行等选择器。

参数：
  path: 文件路径（相对于项目根目录）
  selector: 代码片段选择器，例如：function:main、class:User、method:User.GetName、lines:10-20
  new_content: 要写入的新内容

选择器语法：
  function:函数名      - 修改指定函数
  class:类名          - 修改指定类/结构体
  method:类名.方法名   - 修改指定方法
  lines:开始行-结束行  - 修改指定行范围（后备方案）

优势：
1. 基于代码结构，能理解函数、类、方法的语义
2. 自动定位代码片段，无需手动计算行号

示例：
  # 修改main函数
  write_code_section(path="main.go", selector="function:main", new_content="func main() {\n    fmt.Println(\"Hello\")\n}")
  
  # 修改User类的GetName方法
  write_code_section(path="user.go", selector="method:User.GetName", new_content="func (u *User) GetName() string {\n    return u.Name\n}")`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录）",
					"pattern":     TitleLikePattern(128),
				},
				"selector": map[string]any{
					"type":        "string",
					"description": "代码片段选择器，例如：function:main、class:User、method:User.GetName、lines:10-20",
				},
				"new_content": map[string]any{
					"type":        "string",
					"description": "要写入的新内容, 建议不超过4096字符",
					"pattern":     ContentLikePattern(4096),
				},
			},
			"required":             []string{"path", "selector", "new_content"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleWriteCodeSection,
	})
}

func handleWriteCodeSection(ctx context.Context, args ToolArgs) (string, error) {
	path := ToolArgsValue(args, "path", "")
	if path == "" {
		return "", fmt.Errorf("参数 'path' 缺失")
	}

	// selector, ok := args["selector"]
	selector := ToolArgsValue(args, "selector", "")
	if selector == "" {
		return "", fmt.Errorf("参数 'selector' 缺失")
	}
	newContent := ToolArgsValue(args, "new_content", "")
	if newContent == "" {
		return "", fmt.Errorf("参数 'new_content' 缺失")
	}

	PrintWriteSession(path, selector)

	return writeCodeSection(ctx, path, selector, newContent)
}

func PrintWriteSession(path string, selector string) {
	run := ""
	Printf("修改文件%s代码片段%s%s\n", path, selector, run)
}
