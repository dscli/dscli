package toolcall

import (
	"bytes"
	"crypto/md5"
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

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
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

// guessLanguage 根据文件扩展名猜测语言
func GuessLanguage(path string) string {
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
	case ".vim":
		return "vimscript"
	default:
		return "unknown"
	}
}

// ParseFileStructure0 解析文件结构
func ParseFileStructure0(ctx context.Context, filePath, lang string, usePython bool) (*FileStructure, error) {
	// 如果是Go语言且不强制使用Python，使用Go内置解析器
	if lang == "go" && !usePython {
		return parseGoStructure(filePath)
	}

	// 其他语言使用Python解析器
	return parseWithPython(ctx, filePath, lang)
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


// getOrCreatePythonCacheFile 获取或创建 Python 脚本缓存文件
// 根据脚本内容计算 MD5 哈希，在配置目录中创建缓存文件
// getOrCreatePythonCacheFile 为 Python 脚本创建或获取缓存文件
//
// 使用 MD5 哈希（仅用于缓存路径标识，不涉及安全性）生成唯一的缓存文件名，
// 避免重复写入。缓存目录位于 $HOME/.config/dscli/scripts/python。
func getOrCreatePythonCacheFile(script string) (string, error) {
	hash := fmt.Sprintf("%x", md5.Sum([]byte(script)))
	cacheDir := filepath.Join(config.ConfigDir, "scripts", "python")
	cacheFile := filepath.Join(cacheDir, hash+".py")

	if err := os.MkdirAll(cacheDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create cache directory: %w", err)
	}

	if _, err := os.Stat(cacheFile); os.IsNotExist(err) {
		if err := os.WriteFile(cacheFile, []byte(script), 0o644); err != nil {
			return "", fmt.Errorf("failed to write cache file: %w", err)
		}
	} else if err != nil {
		return "", fmt.Errorf("failed to stat cache file: %w", err)
	}

	return cacheFile, nil
}


func runPythonParsePy(ctx context.Context, filePath string, lang string) (output string, err error) {
	// 从上下文中获取verbose标志
	verbose := outfmt.GetVerbose()

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		err = fmt.Errorf("failed to read file: %w", err)
		return
	}

	// 准备输入数据
	inputData := map[string]any{
		"content":  string(content),
		"language": lang,
	}

	jsonInput, err := json.Marshal(inputData)
	if err != nil {
		err = fmt.Errorf("failed to marshal input data: %w", err)
		return
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Using Python parser for language: %s\n", lang)
		fmt.Fprintf(os.Stderr, "Input size: %d bytes\n", len(jsonInput))
	}
	cacheFile, err := getOrCreatePythonCacheFile(pythonScript)
	if err != nil {
		return
	}

	// 执行缓存的Python脚本
	cmd := exec.CommandContext(ctx, "python3", "-u", cacheFile)

	cmd.Stdin = strings.NewReader(string(jsonInput))

	// 捕获输出
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if verbose {
		fmt.Fprintf(os.Stderr, "开始执行Python脚本...\n")
		fmt.Fprintf(os.Stderr, "命令: python3 -u %s <<< jsonInput\n", cacheFile)
	}

	// 执行命令
	if runErr := cmd.Run(); runErr != nil {
		if stderr.Len() > 0 {
			err = fmt.Errorf("python脚本执行失败: %v\nstderr: %s", runErr, stderr.String())
		} else {
			err = fmt.Errorf("python脚本执行失败: %v", runErr)
		}
		return
	}

	output = stdout.String()
	if verbose {
		fmt.Fprintf(os.Stderr, "Python脚本执行完成，输出大小: %d 字节\n", len(output))
		if stderr.Len() > 0 {
			fmt.Fprintf(os.Stderr, "Python脚本stderr: %s\n", stderr.String())
		}
	}

	return
}


// parseWithPython 使用Python脚本解析文件结构
func parseWithPython(ctx context.Context, filePath, lang string) (structure *FileStructure, err error) {
	// 从上下文中获取verbose标志
	verbose := outfmt.GetVerbose()

	output, err := runPythonParsePy(ctx, filePath, lang)
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
			return nil, fmt.Errorf("python parser error: %s", errMsg)
		}
		return nil, fmt.Errorf("python parser failed without error message")
	}

	// 转换为FileStructure格式
	fs := &FileStructure{
		Language: lang,
		FilePath: filePath,
	}

	// 对于Markdown等非编程语言，处理不同的结构
	switch lang {
	case "markdown", "org":
		// 处理Markdown标题
		if headings, ok := pythonResult["headings"].([]any); ok {
			for _, h := range headings {
				if headingMap, ok := h.(map[string]any); ok {
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
		if codeBlocks, ok := pythonResult["code_blocks"].([]any); ok {
			for _, cb := range codeBlocks {
				if cbMap, ok := cb.(map[string]any); ok {
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
		if lists, ok := pythonResult["lists"].([]any); ok {
			for _, l := range lists {
				if listMap, ok := l.(map[string]any); ok {
					// 将列表项添加到Imports字段
					listItem := getString(listMap, "name")
					if listItem != "" {
						fs.Imports = append(fs.Imports, listItem)
					}
				}
			}
		}

		// 处理链接
		if links, ok := pythonResult["links"].([]any); ok {
			for _, l := range links {
				if linkMap, ok := l.(map[string]any); ok {
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
	case "vimscript":
		// 特殊处理vimscript
		// 解析函数
		if functions, ok := pythonResult["functions"].([]any); ok {
			for _, f := range functions {
				if funcMap, ok := f.(map[string]any); ok {
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
						symbol.EndLine = symbol.Line
					}
					fs.Functions = append(fs.Functions, symbol)
				}
			}
		}

		// 解析命令（映射到Classes）
		if commands, ok := pythonResult["commands"].([]any); ok {
			for _, c := range commands {
				if cmdMap, ok := c.(map[string]any); ok {
					symbol := &Symbol{
						Name: getString(cmdMap, "name"),
						Type: getString(cmdMap, "type"),
					}
					if line, ok := cmdMap["lineno"].(float64); ok {
						symbol.Line = int(line)
						symbol.EndLine = int(line)
					}
					fs.Classes = append(fs.Classes, symbol)
				}
			}
		}

		// 解析变量（映射到Imports）
		if variables, ok := pythonResult["variables"].([]any); ok {
			for _, v := range variables {
				if varMap, ok := v.(map[string]any); ok {
					varName := getString(varMap, "name")
					varType := getString(varMap, "type")
					fs.Imports = append(fs.Imports, fmt.Sprintf("%s (%s)", varName, varType))
				}
			}
		}

		// 解析映射（也映射到Imports）
		if mappings, ok := pythonResult["mappings"].([]any); ok {
			for _, m := range mappings {
				if mapMap, ok := m.(map[string]any); ok {
					mapName := getString(mapMap, "name")
					fs.Imports = append(fs.Imports, fmt.Sprintf("mapping: %s", mapName))
				}
			}
		}

		// 解析自动命令组（也映射到Imports）
		if augroups, ok := pythonResult["augroups"].([]any); ok {
			for _, a := range augroups {
				if augroupMap, ok := a.(map[string]any); ok {
					augroupName := getString(augroupMap, "name")
					fs.Imports = append(fs.Imports, fmt.Sprintf("augroup: %s", augroupName))
				}
			}
		}
	default:
		// 对于其他编程语言，处理函数和类
		// 解析函数
		if functions, ok := pythonResult["functions"].([]any); ok {
			for _, f := range functions {
				if funcMap, ok := f.(map[string]any); ok {
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
		if classes, ok := pythonResult["classes"].([]any); ok {
			for _, c := range classes {
				if classMap, ok := c.(map[string]any); ok {
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
		if imports, ok := pythonResult["imports"].([]any); ok {
			for _, imp := range imports {
				if impStr, ok := imp.(string); ok {
					fs.Imports = append(fs.Imports, impStr)
				}
			}
		}
	}

	// 解析错误
	if errors, ok := pythonResult["errors"].([]any); ok {
		for _, err := range errors {
			if errStr, ok := err.(string); ok {
				fs.Errors = append(fs.Errors, errStr)
			}
		}
	}

	return fs, nil
}

// getString 安全地从map中获取字符串
func getString(m map[string]any, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// ParseFileStructure 公共接口：解析文件结构
func ParseFileStructure(ctx context.Context, filePath string) (*FileStructure, error) {
	// 猜测语言
	lang := GuessLanguage(filePath)

	// 如果是Go语言，使用Go解析器
	if lang == "go" {
		return parseGoStructure(filePath)
	}

	return parseWithPython(ctx, filePath, lang)
}