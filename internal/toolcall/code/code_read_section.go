package code

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

// readCodeSection 基于代码结构定位并读取特定代码片段
// selector语法：
//
//	function:函数名      - 读取指定函数
//	class:类名          - 读取指定类/结构体
//	method:类名.方法名   - 读取指定方法
//	lines:开始行-结束行  - 读取指定行范围（后备方案）
func readCodeSection(ctx context.Context, path string, selector string) (string, error) {
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
	structure, err := toolcall.ParseFileStructure(ctx, path)
	if err != nil {
		return "", fmt.Errorf("解析文件结构失败: %w", err)
	}

	// 根据selector定位代码片段
	lines := strings.Split(string(content), "\n")
	// 去除文件末尾换行符产生的空元素（与bufio.Scanner行为一致）
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	result, err := locateCodeSection(structure, lines, selector)
	if err != nil {
		return "", err
	}

	return result, nil
}

// locateCodeSection 根据selector定位代码片段
func locateCodeSection(structure *toolcall.FileStructure, lines []string, selector string) (string, error) {
	// 解析selector
	parts := strings.SplitN(selector, ":", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("无效的selector格式，应为'类型:名称'，例如'function:main'")
	}

	selectorType := strings.TrimSpace(parts[0])
	selectorValue := strings.TrimSpace(parts[1])

	switch selectorType {
	case "function":
		return locateFunction(structure, lines, selectorValue)
	case "class", "struct":
		return locateClass(structure, lines, selectorValue)
	case "method":
		return locateMethod(structure, lines, selectorValue)
	case "lines":
		return locateLines(lines, selectorValue)
	default:
		return "", fmt.Errorf("不支持的selector类型: %s，支持的类型: function, class, struct, method, lines", selectorType)
	}
}

// locateFunction 定位函数
func locateFunction(structure *toolcall.FileStructure, lines []string, functionName string) (string, error) {
	for _, fn := range structure.Functions {
		if fn.Name == functionName {
			return extractLines(lines, fn.Line, fn.EndLine), nil
		}
	}
	return "", fmt.Errorf("未找到函数: %s", functionName)
}

// locateClass 定位类/结构体
func locateClass(structure *toolcall.FileStructure, lines []string, className string) (string, error) {
	for _, cls := range structure.Classes {
		if cls.Name == className {
			return extractLines(lines, cls.Line, cls.EndLine), nil
		}
	}
	return "", fmt.Errorf("未找到类/结构体: %s", className)
}

// locateMethod 定位方法
func locateMethod(structure *toolcall.FileStructure, lines []string, methodSelector string) (string, error) {
	// 方法选择器格式: 类名.方法名
	parts := strings.Split(methodSelector, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("方法选择器格式错误，应为'类名.方法名'")
	}

	className := parts[0]
	methodName := parts[1]

	// 在函数列表中查找方法
	// 注意：当前解析器将方法也放在Functions列表中，通过Receiver字段标识
	for _, fn := range structure.Functions {
		if fn.Type == "method" && fn.Receiver == className && fn.Name == methodName {
			return extractLines(lines, fn.Line, fn.EndLine), nil
		}
	}

	return "", fmt.Errorf("在类 %s 中未找到方法: %s", className, methodName)
}

// locateLines 定位行范围（后备方案）
func locateLines(lines []string, lineSelector string) (string, error) {
	// 行选择器格式: 开始行-结束行
	parts := strings.Split(lineSelector, "-")
	if len(parts) != 2 {
		return "", fmt.Errorf("行选择器格式错误，应为'开始行-结束行'")
	}

	startLine, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	endLine, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil {
		return "", fmt.Errorf("行号必须为数字")
	}

	if startLine < 1 || endLine > len(lines) || startLine > endLine {
		return "", fmt.Errorf("行号范围无效: %d-%d (文件总行数: %d)", startLine, endLine, len(lines))
	}

	return extractLines(lines, startLine, endLine), nil
}

// extractLines 提取指定行范围的代码
func extractLines(lines []string, startLine, endLine int) string {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	if startLine > endLine {
		return ""
	}

	var result []string
	for i := startLine - 1; i < endLine; i++ {
		result = append(result, lines[i])
	}
	return strings.Join(result, "\n")
}

func init() {
	// 注册 readCodeSection 工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "read_code_section",
		Description: `基于代码结构定位并读取特定代码片段。支持function:函数名、class:类名、method:类名.方法名、lines:开始行-结束行等选择器。
✅ 推荐：这是基于代码结构的新工具，比基于行号的操作更智能、更准确。

参数：
  path: 文件路径（相对于项目根目录）
  selector: 代码片段选择器，例如：function:main、class:User、method:User.GetName、lines:10-20

选择器语法：
  function:函数名      - 读取指定函数
  class:类名          - 读取指定类/结构体
  method:类名.方法名   - 读取指定方法
  lines:开始行-结束行  - 读取指定行范围（后备方案）

优势：
1. 基于代码结构，能理解函数、类、方法的语义
2. 自动定位代码片段，无需手动计算行号
3. 支持多种编程语言（通过文件结构解析）
4. 比 read_file_with_line_range 更智能、更准确

示例：
  # 读取main函数
  read_code_section(path="main.go", selector="function:main")
  
  # 读取User类
  read_code_section(path="user.go", selector="class:User")
  
  # 读取User类的GetName方法
  read_code_section(path="user.go", selector="method:User.GetName")
  
  # 读取第10-20行（后备方案）
  read_code_section(path="config.yaml", selector="lines:10-20")`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录）",
				},
				"selector": map[string]any{
					"type":        "string",
					"description": "代码片段选择器，例如：function:main、class:User、method:User.GetName、lines:10-20",
				},
			},
			"required":             []string{"path", "selector"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleReadCodeSection,
	})
}

func handleReadCodeSection(ctx context.Context, args toolcall.ToolArgs) (result string, user string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		result, err = "", fmt.Errorf("参数 'path' 缺失")
		return
	}
	selector := toolcall.ToolArgsValue(args, "selector", "")
	if selector == "" {
		result, err = "", fmt.Errorf("参数 'selector' 缺失")
		return
	}
	outfmt.Printf("读取%s文件代码片段%s\n", path, selector)
	result, err = readCodeSection(ctx, path, selector)
	return
}
