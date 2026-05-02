// Package flycheck registers the flycheck tool for LLM-driven syntax checking.
package flycheck

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/flycheck"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "flycheck",
		Description: `对指定文件/目录/包运行静态语法/语义检查，类似 Emacs flycheck。

参数：
  path: 路径（必需），相对于项目根目录。支持三种模式：
        - 文件：检查该文件所属的 Go 包（如 internal/flycheck/flycheck.go）
        - 目录：检查该目录及其直接子目录中的所有 Go 包（如 internal/toolcall/）
        - 递归：目录路径后缀 "..." 递归检查所有子包（如 internal/...）

工作原理：
1. 解析路径模式，发现所有待检查的 Go 包
2. 对每个包运行 staticcheck 检查器
3. 汇总所有问题并返回统计信息

返回结果：
- 检查发现问题时：返回统计摘要 + file:line:col: message 格式的诊断信息
- 检查通过时：返回 "✅ flycheck: 检查了 X 个包（Y 个文件），未发现问题"
- 检查器未安装时：返回安装提示

优势：
1. 自动发现 unused import、unused variable 等静态分析问题
2. 支持单文件、目录、递归三种粒度
3. 返回统计信息帮助判断检查覆盖范围

示例：
  # 检查单个文件所在包
  flycheck(path="internal/flycheck/flycheck.go")

  # 检查目录下所有包
  flycheck(path="internal/toolcall/")

  # 递归检查所有子包
  flycheck(path="internal/...")`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件/目录/包路径（相对于项目根目录），支持 '...' 递归模式",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Handler:  handleFlycheck,
	})
}

func handleFlycheck(ctx context.Context, args toolcall.ToolArgs) (result string, suggestion string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("参数 'path' 缺失")
		return
	}

	projectRoot := context.ProjectRoot

	// Normalize path: strip "./" prefix, then trailing slashes (preserve "..." suffix)
	// Use filepath.Clean to resolve ".." and redundant separators.
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimRight(path, "/")
	path = filepath.Clean(path)
	if path == "." {
		// filepath.Clean turns "./" into ".", keep it as "." for root
	}
	// Detect recursive mode: path ends with "..."
	recursive := false
	if strings.HasSuffix(path, "...") {
		recursive = true
		path = strings.TrimSuffix(path, "...")
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "."
		}
	}

	// Resolve to absolute path, check existence
	fullPath := filepath.Join(projectRoot, path)
	fi, statErr := os.Stat(fullPath)
	if statErr != nil {
		err = fmt.Errorf("路径不存在: %s", path)
		return
	}

	// Determine package directories to check
	var pkgDirs []string

	if fi.IsDir() {
		// Directory (or recursive): find all Go packages within
		outfmt.Notice("Flycheck 扫描目录 \"%s\" (recursive=%v) ...", path, recursive)
		pkgDirs = flycheck.FindGoPackages(path, recursive)
		if len(pkgDirs) == 0 {
			return "ℹ️ flycheck: 未找到任何 Go 包", "", nil
		}
	} else {
		// Single file: check its parent package
		outfmt.Notice("Flycheck 检查 \"%s\" ...", path)
		pkgDir := filepath.Dir(path)
		if pkgDir == "." {
			pkgDirs = []string{"."}
		} else {
			pkgDirs = []string{pkgDir}
		}
	}

	// Run flycheck on each package directory
	var allIssues []flycheck.ClassifiedIssue
	var failedPkgs []string
	totalFiles := 0

	for _, pkgDir := range pkgDirs {
		absDir := filepath.Join(projectRoot, pkgDir)
		nFiles := flycheck.CountGoFiles(absDir)
		totalFiles += nFiles

		_, issues, installHint, checkErr := flycheck.FlycheckDir(ctx, pkgDir)
		if checkErr != nil {
			if installHint != "" {
				// Install hint: return immediately
				suggestion = fmt.Sprintf("💡 %s\n\n%s", checkErr.Error(), installHint)
				return "", suggestion, checkErr
			}
			failedPkgs = append(failedPkgs, pkgDir)
			continue
		}

		allIssues = append(allIssues, issues...)
	}

	// Compute overall stats
	stats := flycheck.CountStats(allIssues)

	// Build final response with severity-aware formatting
	pkgWord := "个包"
	fileWord := "个文件"

	if len(allIssues) > 0 {
		var b strings.Builder

		// Prominent header with emoji based on severity
		if stats.Errors > 0 {
			b.WriteString("## ❌ flycheck 发现编译错误 — 必须立即修复！\n\n")
			b.WriteString(fmt.Sprintf("> 检查了 **%d %s**（**%d %s**），发现：\n",
				len(pkgDirs), pkgWord, totalFiles, fileWord))
			b.WriteString(fmt.Sprintf("> ❌ **%d** 个编译错误", stats.Errors))
			if stats.Warnings > 0 {
				b.WriteString(fmt.Sprintf(" / ⚠️ **%d** 个警告", stats.Warnings))
			}
			if stats.Suggestions > 0 {
				b.WriteString(fmt.Sprintf(" / 💡 **%d** 个建议", stats.Suggestions))
			}
			b.WriteString("\n\n")
		} else {
			b.WriteString("## ⚠️ flycheck 发现问题\n\n")
			b.WriteString(fmt.Sprintf("> 检查了 **%d %s**（**%d %s**），发现：\n",
				len(pkgDirs), pkgWord, totalFiles, fileWord))
			b.WriteString(fmt.Sprintf("> ⚠️ **%d** 个警告", stats.Warnings))
			if stats.Suggestions > 0 {
				b.WriteString(fmt.Sprintf(" / 💡 **%d** 个建议", stats.Suggestions))
			}
			b.WriteString("\n\n")
		}

		// Each issue with severity prefix and code block wrapping
		b.WriteString("```\n")
		for _, iss := range allIssues {
			b.WriteString(iss.Severity.String())
			b.WriteString(" ")
			b.WriteString(iss.Line)
			b.WriteString("\n")
		}
		b.WriteString("```\n")

		result = b.String()
	} else if len(failedPkgs) > 0 {
		result = fmt.Sprintf("⚠️ flycheck: 检查了 %d %s（%d %s），%d 个包检查失败: %s",
			len(pkgDirs)-len(failedPkgs), pkgWord, totalFiles, fileWord,
			len(failedPkgs), strings.Join(failedPkgs, ", "))
	} else {
		result = fmt.Sprintf("✅ flycheck: 检查了 %d %s（%d %s），未发现问题",
			len(pkgDirs), pkgWord, totalFiles, fileWord)
	}

	return
}