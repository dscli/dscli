package main

import (
	"fmt"
	"os"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/flycheck"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

var (
	flycheckCmd   *cobra.Command
	flycheckEmacs bool
)

func init() {
	flycheckCmd = AddRootCommand(&cobra.Command{
		Use:   "flycheck <path>",
		Short: "Static code analysis (like Emacs flycheck)",
		Long: `Run static analysis on files/directories/packages.

The <path> argument supports three modes:
  - File: check single file (Go files check their package, Python files directly)
  - Directory: check all Go packages or Python files in directory (e.g. internal/toolcall/)
  - Recursive: append "..." to path for recursive checking (e.g. internal/...)

Supported languages:
  Go     — staticcheck (auto-discovers packages, package-level check)
  Python — ruff (fast linter, supports single file and directory check)

Options:
  --emacs  Use Emacs built-in flycheck (supports 119+ languages)
           instead of dscli native Go/Python checker

Output severity levels:
  ❌ Compile error (must fix immediately)
  ⚠️ Warning
  💡 Improvement suggestion

Examples:
  dscli flycheck internal/flycheck/flycheck.go
  dscli flycheck my_script.py
  dscli flycheck internal/
  dscli flycheck internal/...
  dscli flycheck --emacs internal/`,
		Args: cobra.ExactArgs(1),
		RunE: flycheckRunE,
	})

	flycheckCmd.Flags().BoolVar(&flycheckEmacs, "emacs", false,
		"Use Emacs built-in flycheck (supports 119+ languages)")
}

func flycheckRunE(cmd *cobra.Command, args []string) error {
	return flycheckRunEImpl(args[0])
}

func flycheckRunEImpl(path string) error {
	ctx := context.Background()
	if flycheckEmacs {
		ctx = context.WithValue(ctx, flycheck.EmacsKey, true)
	}
	result, err := flycheck.CheckPath(ctx, path)
	if err != nil {
		outfmt.Error("%v", err)
		if result != nil && result.Suggestion != "" {
			outfmt.Println(result.Suggestion)
		}
		return nil
	}
	if !result.Supported {
		kind := "文件"
		if result.Mode == "package" {
			kind = "目录"
		}
		outfmt.Printf("ℹ️ flycheck 暂不支持 %s 语言（%s: %s）\n",
			langDisplayName(result.Language), kind, result.Path)
		outfmt.Println("   目前支持 Go 和 Python 语言。如需支持其他语言请联系开发者。")
		return nil
	}

	if result.Mode == "file" {
		if result.RawOutput != "" {
			outfmt.Printf("> 检查文件: %s\n\n", result.Path)
			outfmt.Println(result.RawOutput)
		} else {
			outfmt.Printf("✅ flycheck: 检查了 %s，未发现问题\n", result.Path)
		}
		return nil
	}

	printPackageResult(result)

	if result.Stats.Errors > 0 {
		os.Exit(1)
	}
	return nil
}


// printPackageResult 打印 Go 包检查结果。
func printPackageResult(r *flycheck.CheckResult) {
	if len(r.Issues) > 0 {
		if r.Stats.Errors > 0 {
			outfmt.PrintHeader("❌ flycheck 发现编译错误 — 必须立即修复！")
		} else {
			outfmt.PrintHeader("⚠️ flycheck 发现问题")
		}

		sb := fmt.Sprintf("> 检查了 %d 个包（%d 个文件），发现：", r.NPkgs, r.NFiles)
		if r.Stats.Errors > 0 {
			sb += fmt.Sprintf(" ❌ %d 个编译错误", r.Stats.Errors)
			if r.Stats.Warnings > 0 {
				sb += fmt.Sprintf(" / ⚠️ %d 个警告", r.Stats.Warnings)
			}
			if r.Stats.Suggestions > 0 {
				sb += fmt.Sprintf(" / 💡 %d 个建议", r.Stats.Suggestions)
			}
		} else {
			sb += fmt.Sprintf(" ⚠️ %d 个警告", r.Stats.Warnings)
			if r.Stats.Suggestions > 0 {
				sb += fmt.Sprintf(" / 💡 %d 个建议", r.Stats.Suggestions)
			}
		}
		outfmt.Println(sb)

		outfmt.Println("")
		for _, iss := range r.Issues {
			outfmt.Printf("%s %s\n", iss.Severity, iss.Line)
		}
	}

	if len(r.FailedPkgs) > 0 {
		outfmt.Println("")
		outfmt.Warn("[!] %d 个包检查失败:", len(r.FailedPkgs))
		for i, pkg := range r.FailedPkgs {
			info := ""
			if i < len(r.FailedInfos) {
				info = " — " + r.FailedInfos[i]
			}
			outfmt.Printf("    • %s%s\n", pkg, info)
		}
	}

	if len(r.Issues) == 0 && len(r.FailedPkgs) == 0 {
		outfmt.Printf("✅ flycheck: 检查了 %d 个包（%d 个文件），未发现问题\n",
			r.NPkgs, r.NFiles)
	}
}

// langDisplayName 将语言标识转为可读名称。
func langDisplayName(lang string) string {
	switch lang {
	case "python":
		return "Python"
	case "javascript":
		return "JavaScript"
	case "typescript":
		return "TypeScript"
	case "java":
		return "Java"
	case "cpp":
		return "C++"
	case "c":
		return "C"
	case "rust":
		return "Rust"
	case "ruby":
		return "Ruby"
	case "php":
		return "PHP"
	case "swift":
		return "Swift"
	case "kotlin":
		return "Kotlin"
	case "scala":
		return "Scala"
	case "shell":
		return "Shell"
	case "markdown":
		return "Markdown"
	case "json":
		return "JSON"
	case "yaml":
		return "YAML"
	case "toml":
		return "TOML"
	case "xml":
		return "XML"
	case "html":
		return "HTML"
	case "css":
		return "CSS"
	case "vimscript":
		return "VimScript"
	default:
		return lang
	}
}
