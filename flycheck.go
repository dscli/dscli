package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/flycheck"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	_ = AddRootCommand(&cobra.Command{
		Use:   "flycheck <path>",
		Short: "静态检查 Go 代码（类似 Emacs flycheck）",
		Long: `对指定文件/目录/包运行 staticcheck 静态检查。

参数 <path> 支持三种模式：
  - 文件：检查该文件所属的 Go 包（如 internal/flycheck/flycheck.go）
  - 目录：检查该目录及其直接子目录中的所有 Go 包（如 internal/toolcall/）
  - 递归：目录路径后缀 "..." 递归检查所有子包（如 internal/...）

输出按严重程度分级：
  ❌ 编译错误（必须立即修复）
  ⚠️ 警告
  💡 改进建议

示例：
  dscli flycheck internal/flycheck/flycheck.go
  dscli flycheck internal/
  dscli flycheck internal/...`,
		Args: cobra.ExactArgs(1),
		RunE: flycheckRunE,
	})
}

func flycheckRunE(cmd *cobra.Command, args []string) error {
	path := args[0]
	return flycheckRunEImpl(path)
}

func flycheckRunEImpl(path string) error {
	// Normalize path: strip "./" prefix, clean
	path = strings.TrimPrefix(path, "./")
	path = filepath.Clean(path)

	// Detect recursive mode
	recursive := false
	if strings.HasSuffix(path, "...") {
		recursive = true
		path = strings.TrimSuffix(path, "...")
		path = strings.TrimRight(path, "/")
		if path == "" {
			path = "."
		}
	}

	// Determine package directories to check
	var pkgDirs []string
	fi, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("路径不存在: %s", path)
	}

	if fi.IsDir() {
		pkgDirs = flycheck.FindGoPackages(path, recursive)
		if len(pkgDirs) == 0 {
			outfmt.Println("ℹ️ 未找到任何 Go 包")
			return nil
		}
	} else {
		pkgDir := filepath.Dir(path)
		if pkgDir == "." {
			pkgDirs = []string{"."}
		} else {
			pkgDirs = []string{pkgDir}
		}
	}

	// Run flycheck on each package
	var allIssues []flycheck.ClassifiedIssue
	var failedPkgs []string
	totalFiles := 0
	ctx := context.Background()

	for _, pkgDir := range pkgDirs {
		absDir := filepath.Join(context.ProjectRoot, pkgDir)
		nFiles := flycheck.CountGoFiles(absDir)
		totalFiles += nFiles

		_, issues, installHint, checkErr := flycheck.FlycheckDir(ctx, pkgDir)
		if checkErr != nil {
			if installHint != "" {
				outfmt.Error("%v", checkErr)
				outfmt.Println(installHint)
				return checkErr
			}
			failedPkgs = append(failedPkgs, pkgDir)
			continue
		}
		allIssues = append(allIssues, issues...)
	}

	// Print results
	stats := flycheck.CountStats(allIssues)

	if len(allIssues) > 0 {
		// Print header
		if stats.Errors > 0 {
			outfmt.PrintHeader("❌ flycheck 发现编译错误 — 必须立即修复！")
		} else {
			outfmt.PrintHeader("⚠️ flycheck 发现问题")
		}

		// Print summary line
		sb := fmt.Sprintf("> 检查了 %d 个包（%d 个文件），发现：", len(pkgDirs), totalFiles)
		if stats.Errors > 0 {
			sb += fmt.Sprintf(" ❌ %d 个编译错误", stats.Errors)
			if stats.Warnings > 0 {
				sb += fmt.Sprintf(" / ⚠️ %d 个警告", stats.Warnings)
			}
			if stats.Suggestions > 0 {
				sb += fmt.Sprintf(" / 💡 %d 个建议", stats.Suggestions)
			}
		} else {
			sb += fmt.Sprintf(" ⚠️ %d 个警告", stats.Warnings)
			if stats.Suggestions > 0 {
				sb += fmt.Sprintf(" / 💡 %d 个建议", stats.Suggestions)
			}
		}
		outfmt.Println(sb)

		// Print each issue
		outfmt.Println("")
		for _, iss := range allIssues {
			outfmt.Printf("%s %s\n", iss.Severity, iss.Line)
		}
	} else if len(failedPkgs) > 0 {
		outfmt.Warn("检查了 %d 个包（%d 个文件），%d 个包检查失败: %s",
			len(pkgDirs)-len(failedPkgs), totalFiles,
			len(failedPkgs), strings.Join(failedPkgs, ", "))
	} else {
		outfmt.Printf("✅ flycheck: 检查了 %d 个包（%d 个文件），未发现问题\n",
			len(pkgDirs), totalFiles)
	}

	if stats.Errors > 0 {
		return fmt.Errorf("发现 %d 个编译错误", stats.Errors)
	}
	return nil
}
