package main

import (
	"fmt"

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
		Short: "静态检查代码（类似 Emacs flycheck）",
		Long: `对指定文件/目录/包运行静态检查。

参数 <path> 支持三种模式：
  - 文件：检查该文件（Go 文件检查所属包，Python 文件直接检查）
  - 目录：检查该目录中的所有 Go 包或 Python 文件（如 internal/toolcall/）
  - 递归：目录路径后缀 "..." 递归检查所有子包/文件（如 internal/...）

支持语言：
  Go     — staticcheck（自动发现包，包级检查）
  Python — ruff（快速 linter，支持单文件和目录检查）

选项：
  --emacs  使用 Emacs 内置 flycheck 实现（支持 119+ 语言），
           而非 dscli 内置的 Go/Python 检查器

输出按严重程度分级：
  ❌ 编译错误（必须立即修复）
  ⚠️ 警告
  💡 改进建议

示例：
  dscli flycheck internal/flycheck/flycheck.go
  dscli flycheck my_script.py
  dscli flycheck internal/
  dscli flycheck internal/...
  dscli flycheck --emacs internal/`,
		Args: cobra.ExactArgs(1),
		RunE: flycheckRunE,
	})

	flycheckCmd.Flags().BoolVar(&flycheckEmacs, "emacs", false,
		"使用 Emacs 内置 flycheck（支持 119+ 语言）")
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
	// 处理错误
	if err != nil {
		outfmt.Error("%v", err)
		if result != nil && result.Suggestion != "" {
			outfmt.Println(result.Suggestion)
		}
		return err
	}
	// 语言不支持
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

	// 非 Go/Python 目录检查 → 单文件检查
	if result.Mode == "file" {
		if result.RawOutput != "" {
			outfmt.Printf("> 检查文件: %s\n\n", result.Path)
			outfmt.Println(result.RawOutput)
		} else {
			outfmt.Printf("✅ flycheck: 检查了 %s，未发现问题\n", result.Path)
		}
		return nil
	}

	// 包/目录检查
	printPackageResult(result)

	if result.Stats.Errors > 0 {
		return fmt.Errorf("发现 %d 个编译错误", result.Stats.Errors)
	}
	return nil
}

// printPackageResult 打印 Go 包检查结果。
func printPackageResult(r *flycheck.CheckResult) {
	if len(r.Issues) > 0 {
		// Header
		if r.Stats.Errors > 0 {
			outfmt.PrintHeader("❌ flycheck 发现编译错误 — 必须立即修复！")
		} else {
			outfmt.PrintHeader("⚠️ flycheck 发现问题")
		}

		// Summary line
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

		// Each issue
		outfmt.Println("")
		for _, iss := range r.Issues {
			outfmt.Printf("%s %s\n", iss.Severity, iss.Line)
		}
	}

	// 报告失败的包（即使已有问题也显示）
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

	// 全部成功
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
