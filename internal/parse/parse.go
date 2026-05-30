package parse

import (
	"bytes"
	"context"
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
	"runtime"
	"strings"
	"unsafe"

	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/treesitter"
	"gitcode.com/dscli/treesitter/clang"
	"modernc.org/libc"
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

// GuessLanguage 根据文件扩展名猜测语言
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
	case ".org":
		return "org"
	case ".el":
		return "elisp"
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

	// 如果是C语言且不强制使用Python，使用tree-sitter原生解析器
	if lang == "c" && !usePython {
		return parseCStructure(filePath)
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

func runPythonParsePy(ctx context.Context, filePath, lang string) (output string, err error) {
	// 从上下文中获取verbose标志
	verbose := outfmt.GetVerbose()

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		err = fmt.Errorf("failed to read file: %w", err)
		return output, err
	}

	// 准备输入数据
	inputData := map[string]any{
		"content":  string(content),
		"language": lang,
	}

	jsonInput, err := json.Marshal(inputData)
	if err != nil {
		err = fmt.Errorf("failed to marshal input data: %w", err)
		return output, err
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "Using Python parser for language: %s\n", lang)
		fmt.Fprintf(os.Stderr, "Input size: %d bytes\n", len(jsonInput))
	}
	cacheFile, err := getOrCreatePythonCacheFile(pythonScript)
	if err != nil {
		return output, err
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
		return output, err
	}

	output = stdout.String()
	if verbose {
		fmt.Fprintf(os.Stderr, "Python脚本执行完成，输出大小: %d 字节\n", len(output))
		if stderr.Len() > 0 {
			fmt.Fprintf(os.Stderr, "Python脚本stderr: %s\n", stderr.String())
		}
	}

	return output, err
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

	switch lang {
	case "markdown", "org":
		fs.Classes = extractSymbols(pythonResult, "headings")
		fs.Functions = extractSymbols(pythonResult, "code_blocks")
		fs.Imports = extractNames(pythonResult, "lists")
		fs.Functions = append(fs.Functions, extractSymbols(pythonResult, "links")...)
	case "vimscript":
		fs.Functions = extractSymbols(pythonResult, "functions")
		fs.Classes = extractSymbols(pythonResult, "commands")
		appendFormatted(pythonResult, "variables", &fs.Imports, "%s (%s)", "name", "type")
		appendFormatted(pythonResult, "mappings", &fs.Imports, "mapping: %s", "name")
		appendFormatted(pythonResult, "augroups", &fs.Imports, "augroup: %s", "name")
	case "elisp":
		fs.Functions = extractSymbols(pythonResult, "functions")
		fs.Functions = append(fs.Functions, extractSymbols(pythonResult, "macros")...)
		fs.Classes = extractSymbols(pythonResult, "variables")
		fs.Classes = append(fs.Classes, extractSymbols(pythonResult, "custom_variables")...)
		appendFormatted(pythonResult, "provides", &fs.Imports, "provide: %s", "name")
	default:
		fs.Functions = extractSymbols(pythonResult, "functions")
		fs.Classes = extractSymbols(pythonResult, "classes")
		fs.Imports = extractStrings(pythonResult, "imports")
	}

	// 解析错误
	fs.Errors = extractStrings(pythonResult, "errors")

	return fs, nil
}

// symbolFromMap 从map中构建Symbol，自动处理lineno/end_lineno默认值
func symbolFromMap(m map[string]any) *Symbol {
	s := &Symbol{
		Name: getString(m, "name"),
		Type: getString(m, "type"),
	}
	if line, ok := m["lineno"].(float64); ok {
		s.Line = int(line)
	}
	if endLine, ok := m["end_lineno"].(float64); ok {
		s.EndLine = int(endLine)
	} else {
		s.EndLine = s.Line
	}
	return s
}

// extractSymbols 从result中提取指定key的Symbol列表
func extractSymbols(result map[string]any, key string) []*Symbol {
	items, ok := result[key].([]any)
	if !ok {
		return nil
	}
	symbols := make([]*Symbol, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			symbols = append(symbols, symbolFromMap(m))
		}
	}
	return symbols
}

// extractStrings 从result中提取指定key的字符串列表（元素为string类型）
func extractStrings(result map[string]any, key string) []string {
	items, ok := result[key].([]any)
	if !ok {
		return nil
	}
	strs := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok {
			strs = append(strs, s)
		}
	}
	return strs
}

// extractNames 从result中提取指定key的name字段列表（跳过空字符串）
func extractNames(result map[string]any, key string) []string {
	items, ok := result[key].([]any)
	if !ok {
		return nil
	}
	names := make([]string, 0, len(items))
	for _, item := range items {
		if m, ok := item.(map[string]any); ok {
			if name := getString(m, "name"); name != "" {
				names = append(names, name)
			}
		}
	}
	return names
}

// appendFormatted 从result中提取指定key的map，按格式追加到target
func appendFormatted(result map[string]any, key string, target *[]string, format string, mapKeys ...string) {
	items, ok := result[key].([]any)
	if !ok {
		return
	}
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		args := make([]any, len(mapKeys))
		for i, k := range mapKeys {
			args[i] = getString(m, k)
		}
		*target = append(*target, fmt.Sprintf(format, args...))
	}
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

	// 如果是C语言，使用tree-sitter原生解析器
	if lang == "c" {
		return parseCStructure(filePath)
	}

	return parseWithPython(ctx, filePath, lang)
}

// parseCStructure uses tree-sitter to parse C file structure natively in Go.
func parseCStructure(filePath string) (*FileStructure, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	tls := libc.NewTLS()
	defer tls.Close()

	parser := treesitter.Xts_parser_new(tls)
	defer treesitter.Xts_parser_delete(tls, parser)

	lang := clang.Xtree_sitter_c(tls)
	if treesitter.Xts_parser_set_language(tls, parser, lang) == 0 {
		return nil, fmt.Errorf("tree-sitter: failed to set C language")
	}
	// content must remain valid for the duration of parse + traversal;
	// tree-sitter returns byte offsets that tsNodeText slices from content.
	srcPtr := uintptr(unsafe.Pointer(unsafe.SliceData(content)))
	tree := treesitter.Xts_parser_parse_string(tls, parser, 0, srcPtr, uint32(len(content)))
	defer treesitter.Xts_tree_delete(tls, tree)
	defer runtime.KeepAlive(content) // prevent GC of content backing array until tree is freed
	root := treesitter.Xts_tree_root_node(tls, tree)

	fs := &FileStructure{
		Language: "c",
		FilePath: filePath,
	}

	// Walk top-level named children of the translation_unit
	count := treesitter.Xts_node_named_child_count(tls, root)
	for i := uint32(0); i < count; i++ {
		child := treesitter.Xts_node_named_child(tls, root, i)
		collectCSymbols(tls, child, content, fs)
	}

	return fs, nil
}

// tsNodeType returns the node type name as a Go string.
func tsNodeType(tls *libc.TLS, node treesitter.TTSNode) string {
	p := treesitter.Xts_node_type(tls, node)
	if p == 0 { // NULL pointer from tree-sitter (defensive)
		return ""
	}
	return libc.GoString(p)
}

// collectCSymbols recursively walks a tree-sitter node and collects symbols.
func collectCSymbols(tls *libc.TLS, node treesitter.TTSNode, content []byte, fs *FileStructure) {
	typeName := tsNodeType(tls, node)

	switch typeName {
	case "function_definition":
		if name := findCName(tls, node, content); name != "" {
			s := tsMakeSymbol(tls, node, name, "function")
			fs.Functions = append(fs.Functions, s)
		}
		return // don't recurse into function bodies

	case "struct_specifier":
		if name := findCName(tls, node, content); name != "" {
			s := tsMakeSymbol(tls, node, name, "struct")
			fs.Classes = append(fs.Classes, s)
		}
		return // don't recurse into struct body

	case "union_specifier":
		if name := findCName(tls, node, content); name != "" {
			s := tsMakeSymbol(tls, node, name, "union")
			fs.Classes = append(fs.Classes, s)
		}
		return

	case "enum_specifier":
		if name := findCName(tls, node, content); name != "" {
			s := tsMakeSymbol(tls, node, name, "enum")
			fs.Classes = append(fs.Classes, s)
		}
		return

	case "type_definition":
		if name := findCName(tls, node, content); name != "" {
			s := tsMakeSymbol(tls, node, name, "typedef")
			fs.Classes = append(fs.Classes, s)
		}
		return // don't recurse — avoids duplicate struct/enum inside typedef

	case "preproc_include":
		// Fall through to recursion: includes can be nested inside
		// preproc_if blocks, so we must continue traversing children.
		if path := findIncludePath(tls, node, content); path != "" {
			fs.Imports = append(fs.Imports, path)
		}

	case "preproc_def":
		if name := findCDefineName(tls, node, content); name != "" {
			s := tsMakeSymbol(tls, node, name, "macro")
			fs.Classes = append(fs.Classes, s)
		}
	}

	// Recurse into children for nested constructs (e.g. preproc_if → preproc_include)
	childCount := treesitter.Xts_node_named_child_count(tls, node)
	for i := uint32(0); i < childCount; i++ {
		child := treesitter.Xts_node_named_child(tls, node, i)
		collectCSymbols(tls, child, content, fs)
	}
}

// tsMakeSymbol creates a Symbol from a tree-sitter node.
// Tree-sitter uses 0-based rows and columns; we convert to 1-based
// to match editor/LSP conventions.
func tsMakeSymbol(tls *libc.TLS, node treesitter.TTSNode, name, kind string) *Symbol {
	start := treesitter.Xts_node_start_point(tls, node)
	end := treesitter.Xts_node_end_point(tls, node)
	return &Symbol{
		Name:      name,
		Type:      kind,
		Line:      int(start.Frow) + 1,
		Column:    int(start.Fcolumn) + 1,
		EndLine:   int(end.Frow) + 1,
		EndColumn: int(end.Fcolumn) + 1,
	}
}

// findCName finds the last meaningful identifier name within a tree-sitter node.
// It avoids descending into compound_statements and field_declaration_lists.
// Uses the last identifier (not first) so that type_definition correctly
// returns the alias name rather than a nested struct/enum tag.
func findCName(tls *libc.TLS, node treesitter.TTSNode, content []byte) string {
	typeName := tsNodeType(tls, node)

	// Terminal identifier nodes: return their text.
	switch typeName {
	case "identifier", "field_identifier", "type_identifier", "statement_identifier":
		return tsNodeText(tls, node, content)
	}

	// Don't descend into bodies.
	switch typeName {
	case "compound_statement", "field_declaration_list", "enumerator_list",
		"parameter_list", "argument_list", "initializer_list":
		return ""
	}

	// Recurse into children; take the LAST identifier found.
	// For most nodes (function, struct, enum) there is only one meaningful
	// identifier. For type_definition, the alias name is always the last
	// type_identifier — earlier ones may be struct/enum tags that we must skip.
	var lastName string
	childCount := treesitter.Xts_node_named_child_count(tls, node)
	for i := uint32(0); i < childCount; i++ {
		child := treesitter.Xts_node_named_child(tls, node, i)
		if n := findCName(tls, child, content); n != "" {
			lastName = n
		}
	}
	return lastName
}

// findIncludePath extracts the include path from a preproc_include node.
func findIncludePath(tls *libc.TLS, node treesitter.TTSNode, content []byte) string {
	childCount := treesitter.Xts_node_named_child_count(tls, node)
	for i := uint32(0); i < childCount; i++ {
		child := treesitter.Xts_node_named_child(tls, node, i)
		typeName := tsNodeType(tls, child)
		switch typeName {
		case "system_lib_string":
			return tsNodeText(tls, child, content)
		case "string_content":
			text := tsNodeText(tls, child, content)
			return `"` + text + `"`
		}
	}
	return ""
}

// findCDefineName extracts the macro name from a preproc_def node.
func findCDefineName(tls *libc.TLS, node treesitter.TTSNode, content []byte) string {
	childCount := treesitter.Xts_node_named_child_count(tls, node)
	for i := uint32(0); i < childCount; i++ {
		child := treesitter.Xts_node_named_child(tls, node, i)
		typeName := tsNodeType(tls, child)
		if typeName == "identifier" {
			return tsNodeText(tls, child, content)
		}
	}
	return ""
}

// tsNodeText extracts the source text for a tree-sitter node.
func tsNodeText(tls *libc.TLS, node treesitter.TTSNode, content []byte) string {
	start := treesitter.Xts_node_start_byte(tls, node)
	end := treesitter.Xts_node_end_byte(tls, node)
	startI, endI := int(start), int(end)
	if startI >= 0 && startI <= endI && endI <= len(content) {
		return string(content[startI:endI])
	}
	return ""
}
