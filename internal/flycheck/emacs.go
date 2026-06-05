package flycheck

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/shell"
)

//go:embed dscli-flycheck.sh
var flycheckScript string

// isEmacsEnv 检查是否在 Emacs 环境中运行。
//
// Emacs (dscli.el) 在启动 dscli 时会设置 INSIDE_EMACS 和 EMACS 环境变量。
func isEmacsEnv() bool {
	return os.Getenv("INSIDE_EMACS") != "" || os.Getenv("EMACS") != ""
}

// emacsFlycheckResult 是 Emacs flycheck 返回的 JSON 结构。
//
// 与 dscli-flycheck.sh 输出格式严格对应。
type emacsFlycheckResult struct {
	File     string   `json:"file"`
	Language string   `json:"language"`
	Checkers []string `json:"checkers"`
	NErrors  int      `json:"n_errors"`
	Stats    struct {
		Errors      int `json:"errors"`
		Warnings    int `json:"warnings"`
		Suggestions int `json:"suggestions"`
	} `json:"stats"`
	Errors []struct {
		Filename string `json:"filename"`
		Line     int    `json:"line"`
		Column   int    `json:"column"`
		Message  string `json:"message"`
		Severity string `json:"severity"`
		Checker  string `json:"checker"`
		ID       string `json:"id"`
	} `json:"errors"`
	// 错误响应
	ErrorStr string `json:"error"`
}

// runEmacsFlycheck 调用内嵌的 dscli-flycheck.sh 对单个文件执行 Emacs flycheck。
//
// 脚本通过 bash -s 执行，脚本内容经 stdin 传入。
// filePath 是相对于项目根目录的路径。
// timeoutSecs 是整体超时秒数（0 表示不设置超时）。
func runEmacsFlycheck(ctx context.Context, filePath string, timeoutSecs int) (*emacsFlycheckResult, error) {
	absPath := filepath.Join(context.ProjectRoot, filePath)

	// 外层超时控制：通过 context 传递给 mvdan/sh，超时后自动杀死子进程
	if timeoutSecs > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
		defer cancel()
	}

	// 通过 heredoc 将内嵌脚本传给 bash 执行
	script := fmt.Sprintf(flycheckScript, absPath)

	output, err := shell.SimpleExecute(ctx, script)
	if err != nil {
		// 脚本返回非零退出码时尝试解析 JSON（可能包含 error 字段）
		if len(output) > 0 {
			var result emacsFlycheckResult
			if jsonErr := json.Unmarshal([]byte(output), &result); jsonErr == nil && result.ErrorStr != "" {
				return nil, fmt.Errorf("flycheck 错误: %s", result.ErrorStr)
			}
		}
		return nil, fmt.Errorf("emacs flycheck 执行失败: %w\n输出: %s", err, output)
	}

	unquoted, err := strconv.Unquote(strings.TrimSpace(output))
	if err == nil {
		output = unquoted
	}

	var result emacsFlycheckResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		return nil, fmt.Errorf("解析 flycheck JSON 失败: %w\n原始输出: %s", err, output)
	}

	// 检查 JSON 中的错误字段
	if result.ErrorStr != "" {
		return nil, fmt.Errorf("flycheck 错误: %s", result.ErrorStr)
	}

	return &result, nil
}

// convertEmacsResult 将 Emacs flycheck 结果转换为统一的 CheckResult 结构。
func convertEmacsResult(emacsResult *emacsFlycheckResult, path string) *CheckResult {
	cr := &CheckResult{
		Path:      path,
		Language:  emacsResult.Language,
		Mode:      "emacs", // 标识为 Emacs flycheck 结果
		Supported: true,
		NFiles:    1,
		Stats: IssueStats{
			Errors:      emacsResult.Stats.Errors,
			Warnings:    emacsResult.Stats.Warnings,
			Suggestions: emacsResult.Stats.Suggestions,
		},
	}

	// 转换每个诊断信息为 ClassifiedIssue
	for _, e := range emacsResult.Errors {
		severity := classifyEmacsSeverity(e.Severity)

		// 构建人类可读的行信息
		line := fmt.Sprintf("%s:%d:%d: %s",
			e.Filename, e.Line, e.Column, e.Message)
		if e.ID != "" {
			line += fmt.Sprintf(" (%s)", e.ID)
		}
		if e.Checker != "" {
			line += fmt.Sprintf(" [%s]", e.Checker)
		}

		cr.Issues = append(cr.Issues, ClassifiedIssue{
			Severity: severity,
			Line:     line,
		})
	}

	return cr
}

// classifyEmacsSeverity 将 Emacs flycheck 的严重级别字符串映射到 IssueSeverity。
func classifyEmacsSeverity(s string) IssueSeverity {
	switch s {
	case "error":
		return SevError
	case "warning":
		return SevWarning
	case "suggestion":
		return SevSuggestion
	default:
		return SevWarning
	}
}

// checkPathEmacs 使用 Emacs flycheck 检查任意路径（文件或目录）。
//
// 文件：直接调用 Emacs flycheck。
// 目录：遍历目录中的文件，逐个调用 Emacs flycheck 并聚合结果。
func checkPathEmacs(ctx context.Context, path string) (*CheckResult, error) {
	// 规范化路径（去除 "./", "..." 等）
	path, _ = NormalizePath(path)

	fullPath := filepath.Join(context.ProjectRoot, path)
	fi, err := os.Stat(fullPath)
	if err != nil {
		return nil, fmt.Errorf("路径不存在: %s", path)
	}

	if fi.IsDir() {
		return checkPathEmacsDir(ctx, path)
	}
	return checkPathEmacsFile(ctx, path)
}

// checkPathEmacsFile 对单个文件执行 Emacs flycheck。
func checkPathEmacsFile(ctx context.Context, path string) (*CheckResult, error) {
	emacsResult, err := runEmacsFlycheck(ctx, path, 30)
	if err != nil {
		return nil, err
	}
	return convertEmacsResult(emacsResult, path), nil
}

// checkPathEmacsDir 对目录执行 Emacs flycheck（遍历文件）。
func checkPathEmacsDir(ctx context.Context, dir string) (*CheckResult, error) {
	absDir := filepath.Join(context.ProjectRoot, dir)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	cr := &CheckResult{
		Path:      dir,
		Mode:      "emacs-dir",
		Supported: true,
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		// 跳过常见的非代码文件
		if ext == "" || isBinaryExt(ext) {
			continue
		}

		relPath := filepath.Join(dir, name)
		cr.NFiles++

		emacsResult, err := runEmacsFlycheck(ctx, relPath, 30)
		if err != nil {
			cr.FailedPkgs = append(cr.FailedPkgs, relPath)
			cr.FailedInfos = append(cr.FailedInfos, err.Error())
			continue
		}

		// 聚合结果
		cr.Stats.Errors += emacsResult.Stats.Errors
		cr.Stats.Warnings += emacsResult.Stats.Warnings
		cr.Stats.Suggestions += emacsResult.Stats.Suggestions

		for _, e := range emacsResult.Errors {
			severity := classifyEmacsSeverity(e.Severity)
			line := fmt.Sprintf("%s:%d:%d: %s",
				e.Filename, e.Line, e.Column, e.Message)
			if e.ID != "" {
				line += fmt.Sprintf(" (%s)", e.ID)
			}
			if e.Checker != "" {
				line += fmt.Sprintf(" [%s]", e.Checker)
			}
			cr.Issues = append(cr.Issues, ClassifiedIssue{
				Severity: severity,
				Line:     line,
			})
		}
	}

	return cr, nil
}

// isBinaryExt 判断扩展名是否属于二进制/非代码文件类型。
func isBinaryExt(ext string) bool {
	switch ext {
	case ".o", ".so", ".a", ".exe", ".dll", ".dylib",
		".jpg", ".jpeg", ".png", ".gif", ".bmp", ".ico", ".svg",
		".mp3", ".mp4", ".avi", ".mov", ".wmv",
		".zip", ".tar", ".gz", ".bz2", ".xz", ".7z",
		".pdf", ".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".ttf", ".otf", ".woff", ".woff2",
		".db", ".sqlite", ".sqlite3",
		".pyc", ".pyo", ".class",
		".min.js", ".min.css":
		return true
	}
	return false
}
