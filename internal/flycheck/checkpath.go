package flycheck

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	dsctx "gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/parse"
)

// ---------------------------------------------------------------------------
// Context key: 允许调用方通过 context 指定目录检查的目标语言。
// 文件检查始终通过扩展名自动识别，不受此值影响。
// ---------------------------------------------------------------------------

// LanguageKey 用于 context.WithValue 传递目标语言。
// 示例：ctx = context.WithValue(ctx, flycheck.LanguageKey, "python")
var LanguageKey = dsctx.ContextKeyType[string]{}

// EmacsKey 用于 context.WithValue 传递 --emacs 选项。
// 当设置为 true 时，强制使用 Emacs flycheck（而非 dscli 内置实现）。
var EmacsKey = dsctx.ContextKeyType[bool]{}

// LanguageFromContext 从 context 中读取目标语言，若未设置返回空字符串。
func LanguageFromContext(ctx context.Context) string {
	return dsctx.ContextValue(ctx, LanguageKey, "")
}

// ---------------------------------------------------------------------------
// CheckResult: 统一返回结构
// ---------------------------------------------------------------------------

// CheckResult 封装一次 flycheck 检查的完整结果。
// 调用方根据 Mode / Language / Supported 字段区别处理。
type CheckResult struct {
	Path string // 规范化后的路径

	Language  string // 检测到的语言（"go", "python", …）
	Mode      string // "package"（Go 包检查）或 "file"（单文件检查）
	Supported bool   // 该语言是否有注册的检查器

	// Go 包检查结果
	Issues      []ClassifiedIssue // 分类后的问题
	Stats       IssueStats        // 问题统计
	NPkgs       int               // 检查的包数
	NFiles      int               // 检查的文件数
	FailedPkgs  []string          // 检查失败的包（相对路径）
	FailedInfos []string          // 失败包的简要错误信息（与 FailedPkgs 一一对应）

	// 非 Go 文件检查结果
	RawOutput string // 原始检查器输出

	// 通用
	Suggestion string // 安装提示等建议信息（仅当 err != nil 时有意义）
}

// ---------------------------------------------------------------------------
// NormalizePath: 路径规范化（供外部复用）
// ---------------------------------------------------------------------------

// NormalizePath 规范化 flycheck 路径，同时检测递归模式。
// 返回 (cleanPath, recursive)。
func NormalizePath(path string) (string, bool) {
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimRight(path, "/")
	path = filepath.Clean(path)

	recursive := false
	if strings.HasSuffix(path, "...") {
		recursive = true
		path = strings.TrimSuffix(path, "...")
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "."
		}
	}

	return path, recursive
}

// ---------------------------------------------------------------------------
// IsLanguageSupported: 查询语言是否有注册的检查器
// ---------------------------------------------------------------------------

// IsLanguageSupported 返回该语言是否有已注册的检查器。
func IsLanguageSupported(lang string) bool {
	checkers, ok := Registry[lang]
	return ok && len(checkers) > 0
}

// ---------------------------------------------------------------------------
// CheckPath: 统一入口
// ---------------------------------------------------------------------------

// CheckPath 智能检查任意路径（文件或目录），自动识别语言并选择合适的检查器。
//
// 路径处理：
//   - 自动规范化（去除 "./" 前缀、清理冗余分隔符）
//   - 识别递归模式（路径后缀 "..."）
//   - 自动判断文件 vs 目录
//
// 语言识别：
//   - 文件：从扩展名自动识别（parse.GuessLanguage）
//   - 目录：默认查找 Go 包；可通过 context.WithValue(LanguageKey, lang) 指定语言
//
// 选项：
//   - --emacs: 通过 context.WithValue(EmacsKey, true) 强制使用 Emacs flycheck
//   - 未设置时，自动检测 INSIDE_EMACS/EMACS 环境变量
//
// 返回 CheckResult，调用方根据 Mode / Language / Supported 字段区别处理。
func CheckPath(ctx context.Context, path string) (*CheckResult, error) {
	// --emacs 选项：强制使用 Emacs flycheck（支持 119+ 语言）
	if dsctx.ContextValue(ctx, EmacsKey, false) {
		return checkPathEmacs(ctx, path)
	}
	// Emacs 环境：自动使用 Emacs flycheck（支持 119+ 语言）
	if isEmacsEnv() {
		return checkPathEmacs(ctx, path)
	}

	// 原有逻辑：Go/Python 静态检查
	// 1. 规范化路径
	path, recursive := NormalizePath(path)

	// 2. 确认路径存在
	fullPath := filepath.Join(dsctx.ProjectRoot, path)
	fi, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	// 3. 分派
	var result *CheckResult
	if fi.IsDir() {
		result, err = checkPathDir(ctx, path, recursive)
	} else {
		result, err = checkPathFile(ctx, path)
	}

	if result != nil {
		result.Path = path
	}
	return result, err
}

// checkPathDir 检查目录（含递归模式）。
func checkPathDir(ctx context.Context, dir string, recursive bool) (*CheckResult, error) {
	// 从 context 获取语言；未指定时自动检测目录内容
	lang := LanguageFromContext(ctx)
	if lang == "" {
		lang = detectDirLanguage(dir, recursive)
	}

	result := &CheckResult{
		Language:  lang,
		Mode:      "package",
		Supported: IsLanguageSupported(lang),
	}

	if !result.Supported {
		return result, nil
	}

	switch lang {
	case "go":
		return checkGoDir(ctx, dir, recursive, result)
	case "python":
		return checkPythonDir(ctx, dir, recursive, result)
	default:
		return result, nil
	}
}

// detectDirLanguage 根据目录中的文件扩展名自动检测语言。
// 优先级：Go > Python > 其他。如果都不匹配返回 "go"。
func detectDirLanguage(dir string, recursive bool) string {
	absDir := filepath.Join(dsctx.ProjectRoot, dir)

	hasGo := false
	hasPy := false

	if recursive {
		filepath.Walk(absDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil
			}
			if info.IsDir() {
				return nil
			}
			switch {
			case strings.HasSuffix(info.Name(), ".go"):
				hasGo = true
			case strings.HasSuffix(info.Name(), ".py"):
				hasPy = true
			}
			if hasGo && hasPy {
				return filepath.SkipAll
			}
			return nil
		})
	} else {
		entries, err := os.ReadDir(absDir)
		if err != nil {
			return "go"
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			switch {
			case strings.HasSuffix(e.Name(), ".go"):
				hasGo = true
			case strings.HasSuffix(e.Name(), ".py"):
				hasPy = true
			}
		}
	}

	if hasGo {
		return "go"
	}
	if hasPy {
		return "python"
	}
	return "go"
}

// checkGoDir checks a Go directory by finding Go packages and running staticcheck on each.
func checkGoDir(ctx context.Context, dir string, recursive bool, result *CheckResult) (*CheckResult, error) {
	pkgDirs := FindGoPackages(dir, recursive)
	if len(pkgDirs) == 0 {
		return result, nil
	}

	var allIssues []ClassifiedIssue

	for _, pkgDir := range pkgDirs {
		absDir := filepath.Join(dsctx.ProjectRoot, pkgDir)
		result.NFiles += CountGoFiles(absDir)

		_, issues, installHint, checkErr := FlycheckDir(ctx, "go", pkgDir)
		if checkErr != nil {
			if installHint != "" {
				result.Suggestion = installHint
				return result, checkErr
			}
			result.FailedPkgs = append(result.FailedPkgs, pkgDir)
			result.FailedInfos = append(result.FailedInfos, checkErr.Error())
			continue
		}

		allIssues = append(allIssues, issues...)
	}

	result.Issues = allIssues
	result.Stats = CountStats(allIssues)
	result.NPkgs = len(pkgDirs)

	return result, nil
}

// checkPythonDir checks a Python directory using ruff.
// ruff handles recursion natively when given a directory path.
func checkPythonDir(ctx context.Context, dir string, recursive bool, result *CheckResult) (*CheckResult, error) {
	// Count Python files for stats
	pyFiles := FindPyFiles(dir, recursive)
	result.NFiles = len(pyFiles)
	result.NPkgs = 1 // Python treats the whole directory as one check unit

	if result.NFiles == 0 {
		return result, nil
	}

	_, issues, installHint, checkErr := FlycheckDir(ctx, "python", dir)
	if checkErr != nil {
		if installHint != "" {
			result.Suggestion = installHint
			return result, checkErr
		}
		result.FailedPkgs = append(result.FailedPkgs, dir)
		result.FailedInfos = append(result.FailedInfos, checkErr.Error())
		return result, nil
	}

	result.Issues = issues
	result.Stats = CountStats(issues)

	return result, nil
}

// checkPathFile 检查单个文件。
func checkPathFile(ctx context.Context, path string) (*CheckResult, error) {
	ext := strings.ToLower(filepath.Ext(path))
	lang := parse.GuessLanguage(path)

	result := &CheckResult{
		Language:  lang,
		Mode:      "file",
		Supported: IsLanguageSupported(lang),
		NFiles:    1,
	}

	if ext == ".go" {
		// Go 文件：检查其所属的包目录
		pkgDir := filepath.Dir(path)
		if pkgDir == "." {
			// 文件在项目根目录，就用 "." 作为包目录
		}

		absDir := filepath.Join(dsctx.ProjectRoot, pkgDir)
		result.NFiles = CountGoFiles(absDir)
		result.NPkgs = 1
		result.Mode = "package" // Go 文件走包级检查

		_, issues, installHint, checkErr := FlycheckDir(ctx, "go", pkgDir)
		if checkErr != nil {
			if installHint != "" {
				result.Suggestion = installHint
			}
			return result, checkErr
		}

		result.Issues = issues
		result.Stats = CountStats(issues)
		return result, nil
	}

	// 非 Go 文件：使用多语言 Flycheck
	if !result.Supported {
		return result, nil
	}

	rawOutput, suggestion, flyErr := Flycheck(ctx, path)
	if flyErr != nil {
		if suggestion != "" {
			result.Suggestion = suggestion
		}
		return result, flyErr
	}

	result.RawOutput = rawOutput
	return result, nil
}
