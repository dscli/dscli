// Package flycheck provides on-the-fly syntax checking for code files,
// inspired by Emacs flycheck. It detects language from file extension,
// finds the appropriate checker, runs it, and returns results.
package flycheck

import (
	"path/filepath"
	"strings"
)

// GuessLanguage 根据文件扩展名猜测语言。
// 从 internal/parse 迁移而来，是 flycheck 的语言检测引擎。
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
