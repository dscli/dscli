package main

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

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
Supports Go files with built-in parser, other languages with fallback regex parsing.`,
		Args: cobra.ExactArgs(1),
		RunE: runParse,
	}

	// 添加选项
	parseCmd.Flags().StringP("language", "l", "", "Specify language (auto-detected by default)")
	parseCmd.Flags().BoolP("verbose", "v", false, "Verbose output")

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

	// 解析文件结构
	fs, err := parseFileStructure(filePath, lang, verbose)
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
func parseFileStructure(filePath, lang string, verbose bool) (*FileStructure, error) {
	switch lang {
	case "go":
		return parseGoStructure(filePath)
	default:
		// 对于非Go语言，使用备用解析器
		return fallbackParse(filePath, lang)
	}
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
			if x.Tok == token.IMPORT {
				for _, spec := range x.Specs {
					if imp, ok := spec.(*ast.ImportSpec); ok {
						fs.Imports = append(fs.Imports, imp.Path.Value)
					}
				}
			} else if x.Tok == token.TYPE {
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

// fallbackParse 使用正则表达式进行简单解析（用于非Go语言）
func fallbackParse(filePath, lang string) (*FileStructure, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	fs := &FileStructure{
		Language: lang,
		FilePath: filePath,
	}

	lines := strings.Split(string(content), "\n")

	switch lang {
	case "python":
		parsePython(lines, fs)
	case "javascript", "typescript":
		parseJavaScript(lines, fs)
	case "java":
		parseJava(lines, fs)
	case "cpp", "c":
		parseCpp(lines, fs)
	default:
		// 对于未知语言，只提供基本信息
		fs.Errors = append(fs.Errors, fmt.Sprintf("No specific parser for language: %s", lang))
	}

	return fs, nil
}

// parsePython 解析Python文件
func parsePython(lines []string, fs *FileStructure) {
	funcRegex := regexp.MustCompile(`^def\s+(\w+)`)
	classRegex := regexp.MustCompile(`^class\s+(\w+)`)
	importRegex := regexp.MustCompile(`^import\s+(.+)|^from\s+(\S+)\s+import`)

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过注释
		if strings.HasPrefix(line, "#") {
			continue
		}

		// 匹配函数
		if matches := funcRegex.FindStringSubmatch(line); matches != nil {
			fs.Functions = append(fs.Functions, &Symbol{
				Name:    matches[1],
				Type:    "function",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配类
		if matches := classRegex.FindStringSubmatch(line); matches != nil {
			fs.Classes = append(fs.Classes, &Symbol{
				Name:    matches[1],
				Type:    "class",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配导入
		if matches := importRegex.FindStringSubmatch(line); matches != nil {
			if matches[1] != "" {
				fs.Imports = append(fs.Imports, matches[1])
			} else if matches[2] != "" {
				fs.Imports = append(fs.Imports, matches[2])
			}
		}
	}
}

// parseJavaScript 解析JavaScript/TypeScript文件
func parseJavaScript(lines []string, fs *FileStructure) {
	funcRegex := regexp.MustCompile(`^(?:export\s+)?(?:async\s+)?function\s+(\w+)`)
	classRegex := regexp.MustCompile(`^(?:export\s+)?class\s+(\w+)`)
	arrowFuncRegex := regexp.MustCompile(`^(?:export\s+)?(?:const|let|var)\s+(\w+)\s*=\s*(?:async\s*)?\([^)]*\)\s*=>`)
	importRegex := regexp.MustCompile(`^import\s+(?:.+from\s+)?['"]([^'"]+)['"]`)

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过注释
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}

		// 匹配函数
		if matches := funcRegex.FindStringSubmatch(line); matches != nil {
			fs.Functions = append(fs.Functions, &Symbol{
				Name:    matches[1],
				Type:    "function",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配箭头函数
		if matches := arrowFuncRegex.FindStringSubmatch(line); matches != nil {
			fs.Functions = append(fs.Functions, &Symbol{
				Name:    matches[1],
				Type:    "function",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配类
		if matches := classRegex.FindStringSubmatch(line); matches != nil {
			fs.Classes = append(fs.Classes, &Symbol{
				Name:    matches[1],
				Type:    "class",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配导入
		if matches := importRegex.FindStringSubmatch(line); matches != nil {
			fs.Imports = append(fs.Imports, matches[1])
		}
	}
}

// parseJava 解析Java文件
func parseJava(lines []string, fs *FileStructure) {
	classRegex := regexp.MustCompile(`^(?:public\s+|private\s+|protected\s+)?(?:abstract\s+)?(?:final\s+)?class\s+(\w+)`)
	methodRegex := regexp.MustCompile(`^(?:public\s+|private\s+|protected\s+)?(?:static\s+)?(?:final\s+)?(?:synchronized\s+)?(?:[\w<>\[\]]+\s+)?(\w+)\s*\([^)]*\)\s*(?:throws\s+[\w,.\s]+)?\s*[{]`)
	importRegex := regexp.MustCompile(`^import\s+(.+);`)

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过注释
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}

		// 匹配类
		if matches := classRegex.FindStringSubmatch(line); matches != nil {
			fs.Classes = append(fs.Classes, &Symbol{
				Name:    matches[1],
				Type:    "class",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配方法（简化版）
		if matches := methodRegex.FindStringSubmatch(line); matches != nil {
			// 排除Java关键字
			keywords := map[string]bool{
				"if": true, "for": true, "while": true, "switch": true,
				"case": true, "try": true, "catch": true, "finally": true,
			}
			if keywords[matches[1]] {
				continue
			}

			// 排除构造函数（与类名相同）
			if len(fs.Classes) > 0 && matches[1] == fs.Classes[len(fs.Classes)-1].Name {
				continue
			}
			fs.Functions = append(fs.Functions, &Symbol{
				Name:    matches[1],
				Type:    "method",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配导入
		if matches := importRegex.FindStringSubmatch(line); matches != nil {
			fs.Imports = append(fs.Imports, matches[1])
		}
	}
}

// parseCpp 解析C/C++文件
func parseCpp(lines []string, fs *FileStructure) {
	funcRegex := regexp.MustCompile(`^(?:[\w:<>\[\]\*&]+\s+)?(\w+)\s*\([^)]*\)\s*(?:const\s*)?[{;]`)
	classRegex := regexp.MustCompile(`^(?:class|struct)\s+(\w+)`)
	includeRegex := regexp.MustCompile(`^#include\s+[<"]([^>"]+)[>"]`)

	for i, line := range lines {
		line = strings.TrimSpace(line)

		// 跳过注释和预处理指令（除了#include）
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") ||
			(strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "#include")) {
			continue
		}

		// 匹配函数
		if matches := funcRegex.FindStringSubmatch(line); matches != nil {
			fs.Functions = append(fs.Functions, &Symbol{
				Name:    matches[1],
				Type:    "function",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配类/结构体
		if matches := classRegex.FindStringSubmatch(line); matches != nil {
			fs.Classes = append(fs.Classes, &Symbol{
				Name:    matches[1],
				Type:    "class",
				Line:    i + 1,
				EndLine: i + 1,
			})
		}

		// 匹配包含
		if matches := includeRegex.FindStringSubmatch(line); matches != nil {
			fs.Imports = append(fs.Imports, matches[1])
		}
	}
}
