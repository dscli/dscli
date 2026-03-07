package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
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

	// 添加依赖检查命令
	depsCmd := &cobra.Command{
		Use:   "deps",
		Short: "Check Python parser dependencies",
		Long:  "Check if required Python dependencies are available for the parser",
		RunE:  runCheckDeps,
	}
	depsCmd.Flags().BoolP("verbose", "v", false, "Verbose output")
	depsCmd.Flags().BoolP("install", "i", false, "Install missing dependencies")
	depsCmd.Flags().BoolP("setup", "s", false, "Run full setup (check and install if needed)")

	AddRootCommand(depsCmd)
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
	fs, err := parseFileStructure(filePath, lang, verbose, usePython)
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
func parseFileStructure(filePath, lang string, verbose, usePython bool) (*FileStructure, error) {
	// 如果是Go语言且不强制使用Python，使用Go内置解析器
	if lang == "go" && !usePython {
		return parseGoStructure(filePath)
	}

	// 其他语言使用Python解析器
	return parseWithPython(filePath, lang, verbose)
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
func parseWithPython(filePath, lang string, verbose bool) (*FileStructure, error) {
	// 首先检查Python依赖
	if err := checkPythonDependencies(verbose); err != nil {
		return nil, fmt.Errorf("Python dependency check failed: %w", err)
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	// 准备输入数据
	inputData := map[string]interface{}{
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

	// 执行Python脚本
	cmd := exec.Command("python3", "-c", pythonScript)
	cmd.Stdin = bytes.NewReader(jsonInput)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		if verbose {
			fmt.Fprintf(os.Stderr, "Python stderr: %s\n", stderr.String())
		}
		return nil, fmt.Errorf("failed to execute Python script: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Python output size: %d bytes\n", len(output))
	}
	var pythonResult map[string]interface{}
	if err := json.Unmarshal(output, &pythonResult); err != nil {
		return nil, fmt.Errorf("failed to parse Python output: %w", err)
	}

	// 检查Python解析是否成功
	if success, ok := pythonResult["success"].(bool); ok && !success {
		if errMsg, ok := pythonResult["error"].(string); ok {
			return nil, fmt.Errorf("Python parser error: %s", errMsg)
		}
		return nil, fmt.Errorf("Python parser failed without error message")
	}

	// 检查依赖信息
	if depsInfo, ok := pythonResult["dependency_info"].(map[string]interface{}); ok {
		if depsOk, ok := depsInfo["dependencies_ok"].(bool); ok && !depsOk {
			return nil, fmt.Errorf("Python parser dependencies are not satisfied")
		}

		if verbose {
			if enhancedCaps, ok := depsInfo["enhanced_capabilities"].([]interface{}); ok && len(enhancedCaps) > 0 {
				fmt.Fprintf(os.Stderr, "Enhanced capabilities available: %v\n", enhancedCaps)
			}
		}
	}

	// 转换为FileStructure格式
	fs := &FileStructure{
		Language: lang,
		FilePath: filePath,
	}

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
					symbol.EndLine = int(line)
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

// runCheckDeps 是 deps 子命令的入口函数
func runCheckDeps(cmd *cobra.Command, args []string) error {
	verbose, _ := cmd.Flags().GetBool("verbose")
	install, _ := cmd.Flags().GetBool("install")
	setup, _ := cmd.Flags().GetBool("setup")

	// 检查Python依赖
	if err := checkPythonDependencies(verbose); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Python dependencies check failed: %v\n", err)

		if install || setup {
			fmt.Fprintf(os.Stderr, "Attempting to install dependencies...\n")
			if err := installPythonDependencies(verbose); err != nil {
				return fmt.Errorf("failed to install dependencies: %w", err)
			}

			// 重新检查依赖
			fmt.Fprintf(os.Stderr, "Re-checking dependencies after installation...\n")
			if err := checkPythonDependencies(verbose); err != nil {
				return fmt.Errorf("dependencies still not satisfied after installation: %w", err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "\nTo install missing dependencies, run:\n")
			fmt.Fprintf(os.Stderr, "  dscli deps --install\n")
			fmt.Fprintf(os.Stderr, "  dscli deps --setup\n")
			return err
		}
	}

	fmt.Fprintf(os.Stderr, "✅ Python dependencies are OK\n")
	return nil
}

// installPythonDependencies 安装Python依赖
func installPythonDependencies(verbose bool) error {
	// 创建临时requirements.txt文件
	requirementsContent := `# dscli Python Parser Requirements
# Generated by dscli deps command

# Required dependencies (built-in):
# json, re, ast, typing, traceback, importlib.util

# Optional dependencies for enhanced parsing:
astroid>=3.0.0  # Enhanced Python AST parsing and type inference
javalang>=0.13.0  # Java language parsing
pycparser>=2.21  # C/C++ parsing
`

	tmpFile, err := os.CreateTemp("", "dscli-requirements-*.txt")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(requirementsContent); err != nil {
		return fmt.Errorf("failed to write requirements file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	// 执行pip install
	cmd := exec.Command("python3", "-m", "pip", "install", "-r", tmpFile.Name())
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr

	if verbose {
		fmt.Fprintf(os.Stderr, "Running: %s\n", strings.Join(cmd.Args, " "))
	}

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("pip install failed: %w", err)
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "✅ Dependencies installed successfully\n")
	}

	return nil
}

// checkPythonDependencies 检查Python依赖
func checkPythonDependencies(verbose bool) error {
	// 准备依赖检查输入
	inputData := map[string]interface{}{
		"action": "check_deps",
	}

	jsonInput, err := json.Marshal(inputData)
	if err != nil {
		return fmt.Errorf("failed to marshal dependency check input: %w", err)
	}

	// 执行Python脚本进行依赖检查
	cmd := exec.Command("python3", "-c", pythonScript)
	cmd.Stdin = bytes.NewReader(jsonInput)
	cmd.Stderr = os.Stderr

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to execute Python dependency check: %w", err)
	}

	// 解析依赖检查结果
	var result map[string]interface{}
	if err := json.Unmarshal(output, &result); err != nil {
		return fmt.Errorf("failed to parse dependency check result: %w", err)
	}

	// 检查依赖是否OK
	if depsOk, ok := result["dependencies_ok"].(bool); ok {
		if !depsOk {
			return fmt.Errorf("Python parser dependencies are not satisfied")
		}
	} else {
		return fmt.Errorf("dependency check result missing 'dependencies_ok' field")
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "✓ Python dependencies are OK\n")

		if enhancedCaps, ok := result["enhanced_capabilities"].([]interface{}); ok && len(enhancedCaps) > 0 {
			fmt.Fprintf(os.Stderr, "Enhanced capabilities: %v\n", enhancedCaps)
		}

		if pythonVersion, ok := result["python_version"].(string); ok {
			fmt.Fprintf(os.Stderr, "Python version: %s\n", pythonVersion)
		}
	}

	return nil
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
