package code

import (
	_ "embed"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/flycheck"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/parse"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/dscli/dscli/internal/toolcall/file"
)

//go:embed code_write_section.md
var code_write_section_md string

// writeCodeSection 基于代码结构定位并修改特定代码片段
// selector语法：
//
//	function:函数名      - 修改指定函数
//	class:类名          - 修改指定类/结构体
//	method:类名.方法名   - 修改指定方法
//	lines:开始行-结束行  - 修改指定行范围（后备方案）
func writeCodeSection(ctx context.Context, path, selector, newContent string) (result string, err error) {
	// 检查文件是否存在
	if _, err = os.Stat(path); os.IsNotExist(err) {
		err = fmt.Errorf("文件不存在: %s", path)
		return result, err
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		err = fmt.Errorf("读取文件失败: %w", err)
		return result, err
	}

	// 判断是否需要解析文件结构（lines selector 不需要）
	needsParse := !strings.HasPrefix(selector, "lines:")

	var structure *parse.FileStructure
	if needsParse {
		structure, err = parse.ParseFileStructure(ctx, path)
		if err != nil {
			err = fmt.Errorf("解析文件结构失败: %w", err)
			return result, err
		}
	}

	// 根据selector定位代码片段
	lines := strings.Split(string(content), "\n")
	// 记录原始文件是否有尾随换行符
	hadTrailingNewline := len(lines) > 0 && lines[len(lines)-1] == ""
	// 去除文件末尾换行符产生的空元素（与bufio.Scanner行为一致）
	if hadTrailingNewline {
		lines = lines[:len(lines)-1]
	}
	startLine, endLine, err := locateSectionRange(structure, lines, selector)
	if err != nil {
		err = fmt.Errorf("获取区域范围失败: %w", err)
		return result, err
	}

	// 构建结果
	result = buildWriteResult(path, selector, startLine, endLine, lines, newContent)

	if err = writeToFile(path, lines, startLine, endLine, newContent, hadTrailingNewline); err != nil {
		err = fmt.Errorf("写入文件失败: %w", err)
		return result, err
	}
	result += "\n✅ 文件已成功更新"

	return result, err
}

// locateSectionRange 根据selector定位代码片段的范围
func locateSectionRange(structure *parse.FileStructure, lines []string, selector string) (int, int, error) {
	// 解析selector
	parts := strings.SplitN(selector, ":", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("无效的selector格式，应为'类型:名称'，例如'function:main'")
	}

	selectorType := strings.TrimSpace(parts[0])
	selectorValue := strings.TrimSpace(parts[1])

	var startLine, endLine int
	var err error

	switch selectorType {
	case "function":
		startLine, endLine, err = locateFunctionRange(structure, selectorValue)
	case "class", "struct":
		startLine, endLine, err = locateClassRange(structure, selectorValue)
	case "method":
		startLine, endLine, err = locateMethodRange(structure, selectorValue)
	case "lines":
		startLine, endLine, err = locateLinesRange(lines, selectorValue)
	default:
		return 0, 0, fmt.Errorf("不支持的selector类型: %s，支持的类型: function, class, struct, method, lines", selectorType)
	}
	if err != nil {
		return 0, 0, err
	}

	// 防止解析器返回的 endLine 超出文件长度
	// tree-sitter 的 end_lineno 是 exclusive（末行之后的行），
	// 当代码块在文件末尾时会出现 endLine = len(lines) + 1
	if endLine > len(lines) {
		endLine = len(lines)
	}

	return startLine, endLine, nil
}

// locateFunctionRange 定位函数的行范围
func locateFunctionRange(structure *parse.FileStructure, functionName string) (int, int, error) {
	for _, fn := range structure.Functions {
		if fn.Name == functionName {
			return fn.Line, fn.EndLine, nil
		}
	}
	return 0, 0, fmt.Errorf("未找到函数: %s", functionName)
}

// locateClassRange 定位类/结构体的行范围
func locateClassRange(structure *parse.FileStructure, className string) (int, int, error) {
	for _, cls := range structure.Classes {
		if cls.Name == className {
			return cls.Line, cls.EndLine, nil
		}
	}
	return 0, 0, fmt.Errorf("未找到类/结构体: %s", className)
}

// locateMethodRange 定位方法的行范围
func locateMethodRange(structure *parse.FileStructure, methodSelector string) (int, int, error) {
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

	if startLine < 1 || startLine > endLine {
		return 0, 0, fmt.Errorf("行号范围无效: %d-%d", startLine, endLine)
	}

	if startLine > len(lines) {
		return 0, 0, fmt.Errorf("起始行 %d 超出文件总行数 %d", startLine, len(lines))
	}

	// 如果结束行超出文件范围，截断到文件末尾
	// LLM 无法预知文件总行数，请求超出范围是正常行为，不应报错
	if endLine > len(lines) {
		endLine = len(lines)
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
	safeEnd := endLine
	if safeEnd > len(lines) {
		safeEnd = len(lines)
	}
	for i := startLine - 1; i < safeEnd; i++ {
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
func writeToFile(path string, lines []string, startLine, endLine int, newContent string, hadTrailingNewline bool) error {
	// 防御性边界检查：防止因解析器返回异常值导致的切片越界 panic
	if startLine < 1 || startLine > len(lines) {
		return fmt.Errorf("startLine %d 越界，文件共 %d 行", startLine, len(lines))
	}
	if endLine < startLine || endLine > len(lines) {
		return fmt.Errorf("endLine %d 越界，有效范围 [%d, %d]", endLine, startLine, len(lines))
	}

	// 构建新文件内容
	var newLines []string

	// 添加开始行之前的内容
	newLines = append(newLines, lines[:startLine-1]...)

	// 添加新内容
	newLines = append(newLines, strings.Split(newContent, "\n")...)

	// 添加结束行之后的内容
	newLines = append(newLines, lines[endLine:]...)

	// 写入文件，保留原始文件的尾随换行符状态
	newContentStr := strings.Join(newLines, "\n")
	if hadTrailingNewline {
		newContentStr += "\n"
	}
	return os.WriteFile(path, []byte(newContentStr), 0o644)
}

func init() {
	// 注册 writeCodeSection 工具
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "write_code_section",
		Description: code_write_section_md,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File path (relative to project root)",
				},
				"selector": map[string]any{
					"type":        "string",
					"description": "Code selector, e.g. function:main, class:User, method:User.GetName, lines:10-20",
				},
				"new_content": map[string]any{
					"type":        "string",
					"description": "New content to write, max 4096 chars recommended",
				},
				"context": map[string]any{
					"type":        "boolean",
					"description": "After editing, return a context window around the edit. Default true. Set false to suppress.",
				},
			},
			"required":             []string{"path", "selector", "new_content"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleWriteCodeSection,
	})
}

func handleWriteCodeSection(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("参数 'path' 缺失")
		return result, warning, err
	}

	// selector, ok := args["selector"]
	selector := toolcall.ToolArgsValue(args, "selector", "")
	if selector == "" {
		err = fmt.Errorf("参数 'selector' 缺失")
		return result, warning, err
	}
	newContent := toolcall.ToolArgsValue(args, "new_content", "")
	if newContent == "" {
		err = fmt.Errorf("参数 'new_content' 缺失")
		return result, warning, err
	}
	showContext := toolcall.ToolArgsValue(args, "context", true)

	PrintWriteSection(path, selector)

	// 编辑前解析行范围，用于上下文偏移计算
	var oldStart, oldEnd int
	if showContext {
		oldStart, oldEnd, _ = getSectionRange(ctx, path, selector)
	}

	result, err = writeCodeSection(ctx, path, selector, newContent)
	if err != nil {
		return result, warning, err
	}
	// 编辑后上下文窗口
	if showContext && oldStart > 0 {
		if ctxStr := codeEditContext(path, oldStart, oldEnd, newContent); ctxStr != "" {
			result += ctxStr
		}
	}

	// Run flycheck on the written file and append issues as suggestion
	if flyResult, _, flyErr := flycheck.Flycheck(ctx, path); flyErr == nil && flyResult != "" {
		if warning != "" {
			warning += "\n\n"
		}
		warning += flyResult
	}

	return result, warning, err
}

func PrintWriteSection(path, selector string) {
	run := ""
	outfmt.Printf("修改文件%s代码片段%s%s\n", path, selector, run)
}

// getSectionRange 在编辑前解析 selector 对应的行范围（1-based 包含）。
func getSectionRange(ctx context.Context, path, selector string) (int, int, error) {
	structure, err := parse.ParseFileStructure(ctx, path)
	if err != nil {
		return 0, 0, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, err
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	return locateSectionRange(structure, lines, selector)
}

// codeEditContext 为 write_code_section 生成编辑后上下文窗口。
// oldStart/oldEnd 是编辑前的行范围（1-based 包含）。
// codeEditContext 为 write_code_section 生成编辑后上下文窗口。
// oldStart/oldEnd 是编辑前的行范围（1-based 包含）。
func codeEditContext(path string, oldStart, oldEnd int, newContent string) string {
	oldReplaced := oldEnd - oldStart + 1
	newLineCount := strings.Count(newContent, "\n") + 1
	if newContent == "" || strings.HasSuffix(newContent, "\n") {
		newLineCount = strings.Count(newContent, "\n")
	}

	return file.AppendEditContext(path, oldStart, oldEnd, oldReplaced, newLineCount)
}
