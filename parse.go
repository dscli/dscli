package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

//go:embed parse.py
var pythonScript string

// FileStructure 表示文件结构
type FileStructure struct {
	Language  string    `json:"language"`
	FilePath  string    `json:"file_path"`
	Package   string    `json:"package,omitempty"`
	Functions []*Symbol `json:"functions"`
	Classes   []*Symbol `json:"classes"`
	Imports   []string  `json:"imports"`
	Errors    []string  `json:"errors,omitempty"`
}

// Symbol 表示一个代码符号（函数/类）
type Symbol struct {
	Name      string `json:"name"`
	Type      string `json:"type"` // "function", "method", "struct", "interface", "type"
	Line      int    `json:"line"`
	Column    int    `json:"column"`
	EndLine   int    `json:"end_line"`
	EndColumn int    `json:"end_column"`
	Signature string `json:"signature,omitempty"` // 函数签名
	Receiver  string `json:"receiver,omitempty"`  // 方法接收者
}

func init() {
	parseCmd := &cobra.Command{
		Use:   "parse <file>",
		Short: "Parse file structure for LLM editing",
		Long: `Parse file structure (functions, classes, imports) for LLM-assisted editing.
Supports Go files with built-in parser, other languages with Python-based parsing.`,
		Args: cobra.ExactArgs(1),
		RunE: runParse,
	}

	// 添加选项
	parseCmd.Flags().StringP("language", "l", "", "Specify language (auto-detected by default)")
	parseCmd.Flags().BoolP("verbose", "v", false, "Verbose output")
	parseCmd.Flags().BoolP("use-python", "p", false, "Force use Python parser (for non-Go languages)")

	AddRootCommand(parseCmd)
}

// runParse 是 parse 子命令的入口函数
func runParse(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// 获取语言选项
	lang, _ := cmd.Flags().GetString("language")
	if lang == "" {
		lang = guessLanguage(filePath)
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	usePython, _ := cmd.Flags().GetBool("use-python")

	// 解析文件结构
	fs, err := parseFileStructure(cmd.Context(), filePath, lang, verbose, usePython)
	if err != nil {
		return fmt.Errorf("failed to parse file: %w", err)
	}

	// 输出JSON
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(fs)
}

// guessLanguage 根据文件扩展名猜测语言
func guessLanguage(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".js", ".mjs":
		return "javascript"
	case ".ts", ".tsx":
		return "typescript"
	case ".java":
		return "java"
	case ".cpp", ".cc", ".cxx", ".h", ".hpp":
		return "cpp"
	case ".c":
		return "c"
	case ".rs":
		return "rust"
	case ".rb":
		return "ruby"
	case ".php":
		return "php"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".scala":
		return "scala"
	case ".sh", ".bash":
		return "shell"
	case ".md", ".markdown":
		return "markdown"
	case ".json":
		return "json"
	case ".yaml", ".yml":
		return "yaml"
	case ".toml":
		return "toml"
	case ".xml":
		return "xml"
	case ".html", ".htm":
		return "html"
	case ".css":
		return "css"
	default:
		return "unknown"
	}
}

// parseFileStructure 解析文件结构
func parseFileStructure(ctx context.Context, filePath, lang string, verbose, usePython bool) (*FileStructure, error) {
	// 如果是Go语言且不强制使用Python，使用Go内置解析器
	if lang == "go" && !usePython {
		return parseGoStructure(filePath)
	}

	// 其他语言使用Python解析器
	return parseWithPython(ctx, filePath, lang, verbose)
}

// parseGoStructure 使用Go内置AST解析器解析Go文件结构
func parseGoStructure(path string) (*FileStructure, error) {
	fset := token.NewFileSet()
	node, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}

	fs := &FileStructure{
		Language: "go",
		FilePath: path,
		Package:  node.Name.Name,
	}

	ast.Inspect(node, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.FuncDecl:
			pos := fset.Position(x.Pos())
			end := fset.Position(x.End())
			symbol := &Symbol{
				Name:      x.Name.Name,
				Type:      "function",
				Line:      pos.Line,
				Column:    pos.Column,
				EndLine:   end.Line,
				EndColumn: end.Column,
			}

			// 如果是方法，添加接收者信息
			if x.Recv != nil && len(x.Recv.List) > 0 {
				symbol.Type = "method"
				// 提取接收者类型
				if ident, ok := x.Recv.List[0].Type.(*ast.Ident); ok {
					symbol.Receiver = ident.Name
				}
			}

			fs.Functions = append(fs.Functions, symbol)

		case *ast.GenDecl:
			switch x.Tok {
			case token.IMPORT:
				for _, spec := range x.Specs {
					if imp, ok := spec.(*ast.ImportSpec); ok {
						fs.Imports = append(fs.Imports, imp.Path.Value)
					}
				}
			case token.TYPE:
				for _, spec := range x.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						pos := fset.Position(typeSpec.Pos())
						end := fset.Position(typeSpec.End())

						var symbolType string
						switch typeSpec.Type.(type) {
						case *ast.StructType:
							symbolType = "struct"
						case *ast.InterfaceType:
							symbolType = "interface"
						default:
							symbolType = "type"
						}

						fs.Classes = append(fs.Classes, &Symbol{
							Name:      typeSpec.Name.Name,
							Type:      symbolType,
							Line:      pos.Line,
							Column:    pos.Column,
							EndLine:   end.Line,
							EndColumn: end.Column,
						})
					}
				}
			}
		}
		return true
	})
	return fs, nil
}

// parseWithPython 使用Python脚本解析文件结构
func parseWithPython(ctx context.Context, filePath, lang string, verbose bool) (structure *FileStructure, err error) {
	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 准备输入数据
	inputData := map[string]any{
		"content":  string(content),
		"language": lang,
	}

	jsonInput, err := json.Marshal(inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input data: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Using Python parser for language: %s\n", lang)
		fmt.Fprintf(os.Stderr, "Input size: %d bytes\n", len(jsonInput))
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	_, err = w.Write(jsonInput)
	if err != nil {
		return nil, err
	}
	w.Close()
	ctx = context.WithValue(ctx, ShellStdin, r)
	output, err := ShellExec(ctx, pythonScript)
	r.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to execute Python script: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Python output size: %d bytes\n", len(output))
	}

	// 解析Python输出
	var pythonResult map[string]any
	if err := json.Unmarshal([]byte(output), &pythonResult); err != nil {
		return nil, fmt.Errorf("failed to parse Python output: %w", err)
	}

	// 检查Python解析是否成功
	if success, ok := pythonResult["success"].(bool); ok && !success {
		if errMsg, ok := pythonResult["error"].(string); ok {
			return nil, fmt.Errorf("Python parser error: %s", errMsg)
		}
		return nil, fmt.Errorf("Python parser failed without error message")
	}

	// 转换为FileStructure格式
	fs := &FileStructure{
		Language: lang,
		FilePath: filePath,
	}

	// 对于Markdown等非编程语言，处理不同的结构
	if lang == "markdown" || lang == "org" {
		// 处理Markdown标题
		if headings, ok := pythonResult["headings"].([]interface{}); ok {
			for _, h := range headings {
				if headingMap, ok := h.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(headingMap, "name"),
						Type: getString(headingMap, "type"),
					}
					if line, ok := headingMap["lineno"].(float64); ok {
						symbol.Line = int(line)
					}
					if endLine, ok := headingMap["end_lineno"].(float64); ok {
						symbol.EndLine = int(endLine)
					} else {
						symbol.EndLine = symbol.Line
					}
					fs.Classes = append(fs.Classes, symbol)
				}
			}
		}

		// 处理代码块
		if codeBlocks, ok := pythonResult["code_blocks"].([]interface{}); ok {
			for _, cb := range codeBlocks {
				if cbMap, ok := cb.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(cbMap, "name"),
						Type: getString(cbMap, "type"),
					}
					if line, ok := cbMap["lineno"].(float64); ok {
						symbol.Line = int(line)
					}
					if endLine, ok := cbMap["end_lineno"].(float64); ok {
						symbol.EndLine = int(endLine)
					} else {
						symbol.EndLine = symbol.Line
					}
					fs.Functions = append(fs.Functions, symbol)
				}
			}
		}

		// 处理列表项
		if lists, ok := pythonResult["lists"].([]interface{}); ok {
			for _, l := range lists {
				if listMap, ok := l.(map[string]interface{}); ok {
					// 将列表项添加到Imports字段
					listItem := getString(listMap, "name")
					if listItem != "" {
						fs.Imports = append(fs.Imports, listItem)
					}
				}
			}
		}

		// 处理链接
		if links, ok := pythonResult["links"].([]interface{}); ok {
			for _, l := range links {
				if linkMap, ok := l.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(linkMap, "name"),
						Type: getString(linkMap, "type"),
					}
					if line, ok := linkMap["lineno"].(float64); ok {
						symbol.Line = int(line)
					}
					if endLine, ok := linkMap["end_lineno"].(float64); ok {
						symbol.EndLine = int(endLine)
					} else {
						symbol.EndLine = symbol.Line
					}
					fs.Functions = append(fs.Functions, symbol)
				}
			}
		}
	} else {
		// 对于编程语言，处理函数和类
		// 解析函数
		if functions, ok := pythonResult["functions"].([]interface{}); ok {
			for _, f := range functions {
				if funcMap, ok := f.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(funcMap, "name"),
						Type: getString(funcMap, "type"),
					}
					if line, ok := funcMap["lineno"].(float64); ok {
						symbol.Line = int(line)
					}
					if endLine, ok := funcMap["end_lineno"].(float64); ok {
						symbol.EndLine = int(endLine)
					} else {
						// 如果没有end_lineno，使用lineno作为默认值
						symbol.EndLine = symbol.Line
					}
					fs.Functions = append(fs.Functions, symbol)
				}
			}
		}

		// 解析类
		if classes, ok := pythonResult["classes"].([]interface{}); ok {
			for _, c := range classes {
				if classMap, ok := c.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(classMap, "name"),
						Type: getString(classMap, "type"),
					}
					if line, ok := classMap["lineno"].(float64); ok {
						symbol.Line = int(line)
						symbol.EndLine = int(line)
					}
					fs.Classes = append(fs.Classes, symbol)
				}
			}
		}

		// 解析导入
		if imports, ok := pythonResult["imports"].([]interface{}); ok {
			for _, imp := range imports {
				if impStr, ok := imp.(string); ok {
					fs.Imports = append(fs.Imports, impStr)
				}
			}
		}
	}

	// 解析错误
	if errors, ok := pythonResult["errors"].([]interface{}); ok {
		for _, err := range errors {
			if errStr, ok := err.(string); ok {
				fs.Errors = append(fs.Errors, errStr)
			}
		}
	}

	return fs, nil
}

// getString 安全地从map中获取字符串
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// ParseFileStructure 公共接口：解析文件结构
func ParseFileStructure(filePath, content string) (*FileStructure, error) {
	// 猜测语言
	lang := guessLanguage(filePath)

	// 如果是Go语言，使用Go解析器
	if lang == "go" {
		// 创建临时文件
		tmpFile, err := os.CreateTemp("", "parse_*.go")
		if err != nil {
			return nil, fmt.Errorf("failed to create temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		// 写入内容
		if _, err := tmpFile.WriteString(content); err != nil {
			return nil, fmt.Errorf("failed to write temp file: %w", err)
		}
		tmpFile.Close()

		// 解析Go结构
		return parseGoStructure(tmpFile.Name())
	}

	// 其他语言使用Python解析器
	ctx := context.Background()
	inputData := map[string]any{
		"content":  content,
		"language": lang,
	}

	jsonInput, err := json.Marshal(inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal input data: %w", err)
	}

	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	_, err = w.Write(jsonInput)
	if err != nil {
		return nil, err
	}
	w.Close()
	ctx = context.WithValue(ctx, ShellStdin, r)
	output, err := ShellExec(ctx, pythonScript)
	r.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to execute Python script: %w", err)
	}

	// 解析Python输出
	var pythonResult map[string]any
	if err := json.Unmarshal([]byte(output), &pythonResult); err != nil {
		return nil, fmt.Errorf("failed to parse Python output: %w", err)
	}

	// 检查Python解析是否成功
	if success, ok := pythonResult["success"].(bool); ok && !success {
		if errMsg, ok := pythonResult["error"].(string); ok {
			return nil, fmt.Errorf("Python parser error: %s", errMsg)
		}
		return nil, fmt.Errorf("Python parser failed without error message")
	}
	// 转换为FileStructure格式
	fs := &FileStructure{
		Language: lang,
		FilePath: filePath,
	}

	// 对于Markdown等非编程语言，处理不同的结构
	if lang == "markdown" || lang == "org" {
		// 处理标题（映射到Classes）
		if headings, ok := pythonResult["headings"].([]interface{}); ok {
			for _, h := range headings {
				if headingMap, ok := h.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(headingMap, "name"),
						Type: getString(headingMap, "type"),
					}
					if line, ok := headingMap["lineno"].(float64); ok {
						symbol.Line = int(line)
						symbol.EndLine = int(line)
					}
					fs.Classes = append(fs.Classes, symbol)
				}
			}
		}

		// 处理代码块（映射到Functions）
		if codeBlocks, ok := pythonResult["code_blocks"].([]interface{}); ok {
			for _, cb := range codeBlocks {
				if cbMap, ok := cb.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(cbMap, "name"),
						Type: getString(cbMap, "type"),
					}
					if line, ok := cbMap["lineno"].(float64); ok {
						symbol.Line = int(line)
					}
					if endLine, ok := cbMap["end_lineno"].(float64); ok {
						symbol.EndLine = int(endLine)
					} else {
						symbol.EndLine = symbol.Line
					}
					fs.Functions = append(fs.Functions, symbol)
				}
			}
		}

		// 处理列表项（映射到Imports）
		if lists, ok := pythonResult["lists"].([]interface{}); ok {
			for _, l := range lists {
				if listMap, ok := l.(map[string]interface{}); ok {
					fs.Imports = append(fs.Imports, getString(listMap, "name"))
				}
			}
		}

		// 处理链接（映射到Functions）
		if links, ok := pythonResult["links"].([]interface{}); ok {
			for _, l := range links {
				if linkMap, ok := l.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(linkMap, "name"),
						Type: getString(linkMap, "type"),
					}
					if line, ok := linkMap["lineno"].(float64); ok {
						symbol.Line = int(line)
						symbol.EndLine = int(line)
					}
					fs.Functions = append(fs.Functions, symbol)
				}
			}
		}
	} else {
		// 原有的编程语言处理逻辑
		// 解析函数
		if functions, ok := pythonResult["functions"].([]interface{}); ok {
			for _, f := range functions {
				if funcMap, ok := f.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(funcMap, "name"),
						Type: getString(funcMap, "type"),
					}
					if line, ok := funcMap["lineno"].(float64); ok {
						symbol.Line = int(line)
					}
					if endLine, ok := funcMap["end_lineno"].(float64); ok {
						symbol.EndLine = int(endLine)
					} else {
						// 如果没有end_lineno，使用lineno作为默认值
						symbol.EndLine = symbol.Line
					}
					fs.Functions = append(fs.Functions, symbol)
				}
			}
		}

		// 解析类
		if classes, ok := pythonResult["classes"].([]interface{}); ok {
			for _, c := range classes {
				if classMap, ok := c.(map[string]interface{}); ok {
					symbol := &Symbol{
						Name: getString(classMap, "name"),
						Type: getString(classMap, "type"),
					}
					if line, ok := classMap["lineno"].(float64); ok {
						symbol.Line = int(line)
						symbol.EndLine = int(line)
					}
					fs.Classes = append(fs.Classes, symbol)
				}
			}
		}

		// 解析导入
		if imports, ok := pythonResult["imports"].([]interface{}); ok {
			for _, imp := range imports {
				if impStr, ok := imp.(string); ok {
					fs.Imports = append(fs.Imports, impStr)
				}
			}
		}
	}

	// 解析错误
	if errors, ok := pythonResult["errors"].([]interface{}); ok {
		for _, err := range errors {
			if errStr, ok := err.(string); ok {
				fs.Errors = append(fs.Errors, errStr)
			}
		}
	}

	return fs, nil
}
