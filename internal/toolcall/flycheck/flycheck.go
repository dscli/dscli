// Package flycheck registers the flycheck tool for LLM-driven syntax checking.
package flycheck

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/flycheck"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed flycheck.md
var flycheckMd string

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "flycheck",
		Description: flycheckMd,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "File/directory/package path (relative to project root), supports '...' for recursion",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (default 120). Set a longer timeout (e.g. 600) for large codebases.",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "code_ops",
		Timeout:  120 * time.Second,
		Handler:  handleFlycheck,
	})
}

func handleFlycheck(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("参数 'path' 缺失")
		return result, warning, err
	}

	checkResult, checkErr := flycheck.CheckPath(ctx, path)

	// 处理错误
	if checkErr != nil {
		if checkResult != nil && checkResult.Suggestion != "" {
			warning = fmt.Sprintf("💡 %s\n\n%s", checkErr.Error(), checkResult.Suggestion)
		}
		err = checkErr
		return result, warning, err
	}

	// 语言不支持
	if !checkResult.Supported {
		kind := "文件"
		if checkResult.Mode == "package" {
			kind = "目录"
		}
		result = fmt.Sprintf("ℹ️ flycheck 暂不支持 %s 语言（%s: %s）\n   目前支持 Go 和 Python 语言。如需支持其他语言请联系开发者。",
			checkResult.Language, kind, checkResult.Path)
		return result, warning, err
	}

	// Emacs flycheck 结果（单文件或目录）
	if checkResult.Mode == "emacs" || checkResult.Mode == "emacs-dir" {
		result = formatEmacsResult(checkResult)
		return result, warning, err
	}

	// 非 Go/Python 目录检查 → 单文件检查
	if checkResult.Mode == "file" {
		if checkResult.RawOutput != "" {
			result = fmt.Sprintf("> 检查文件: %s\n\n%s", checkResult.Path, checkResult.RawOutput)
		} else {
			result = fmt.Sprintf("✅ flycheck: 检查了 %s，未发现问题", checkResult.Path)
		}
		return result, warning, err
	}

	// 包/目录检查 → Markdown 格式化
	result = formatPackageResult(checkResult)
	return result, warning, err
}

// formatPackageResult outputs package/directory check results in Markdown.
// Adapts terminology based on language: Go says "个包" and "编译错误",
// Python says "个目录" and "静态错误".
func formatPackageResult(r *flycheck.CheckResult) string {
	// Choose terminology based on language
	unitWord := "个包"
	errKindWord := "编译错误"
	if r.Language == "python" {
		unitWord = "个文件"
		errKindWord = "静态错误"
	}
	fileWord := "个文件"
	var b strings.Builder

	if len(r.Issues) > 0 {
		// Header
		if r.Stats.Errors > 0 {
			fmt.Fprintf(&b, "## ❌ flycheck 发现%s — 必须立即修复！\n\n", errKindWord)
			fmt.Fprintf(&b, "> 检查了 **%d %s**（**%d %s**），发现：\n",
				r.NPkgs, unitWord, r.NFiles, fileWord)
			fmt.Fprintf(&b, "> ❌ **%d** %s", r.Stats.Errors, errKindWord)
			if r.Stats.Warnings > 0 {
				fmt.Fprintf(&b, " / ⚠️ **%d** 个警告", r.Stats.Warnings)
			}
			if r.Stats.Suggestions > 0 {
				fmt.Fprintf(&b, " / 💡 **%d** 个建议", r.Stats.Suggestions)
			}
			b.WriteString("\n\n")
		} else {
			b.WriteString("## ⚠️ flycheck 发现问题\n\n")
			fmt.Fprintf(&b, "> 检查了 **%d %s**（**%d %s**），发现：\n",
				r.NPkgs, unitWord, r.NFiles, fileWord)
			fmt.Fprintf(&b, "> ⚠️ **%d** 个警告", r.Stats.Warnings)
			if r.Stats.Suggestions > 0 {
				fmt.Fprintf(&b, " / 💡 **%d** 个建议", r.Stats.Suggestions)
			}
			b.WriteString("\n\n")
		}

		// Issues in code block
		b.WriteString("```\n")
		for _, iss := range r.Issues {
			b.WriteString(iss.Severity.String())
			b.WriteString(" ")
			b.WriteString(iss.Line)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	// 报告失败的包（即使已有问题也显示）
	if len(r.FailedPkgs) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		failWord := "个包"
		if r.Language == "python" {
			failWord = "个目录"
		}
		fmt.Fprintf(&b, "> ⚠️ **%d %s检查失败：**\n", len(r.FailedPkgs), failWord)
		for i, pkg := range r.FailedPkgs {
			info := ""
			if i < len(r.FailedInfos) {
				info = " — " + r.FailedInfos[i]
			}
			fmt.Fprintf(&b, "> - `%s`%s\n", pkg, info)
		}
	}

	// 全部成功
	if b.Len() == 0 {
		fmt.Fprintf(&b, "✅ flycheck: 检查了 %d %s（%d %s），未发现问题",
			r.NPkgs, unitWord, r.NFiles, fileWord)
	}

	return b.String()
}

// formatEmacsResult formats Emacs flycheck results (single file or directory) in Markdown.
//
// Unlike formatPackageResult, this uses generic terminology (not Go/Python specific).
func formatEmacsResult(r *flycheck.CheckResult) string {
	kind := "文件"
	if r.Mode == "emacs-dir" {
		kind = "目录"
	}

	var b strings.Builder

	if len(r.Issues) > 0 {
		if r.Stats.Errors > 0 {
			b.WriteString("## ❌ flycheck 发现静态错误 — 必须立即修复！\n\n")
			fmt.Fprintf(&b, "> 检查了 %s `%s`（**%d** 个文件），发现：\n",
				kind, r.Path, r.NFiles)
			fmt.Fprintf(&b, "> ❌ **%d** 个错误", r.Stats.Errors)
			if r.Stats.Warnings > 0 {
				fmt.Fprintf(&b, " / ⚠️ **%d** 个警告", r.Stats.Warnings)
			}
			if r.Stats.Suggestions > 0 {
				fmt.Fprintf(&b, " / 💡 **%d** 个建议", r.Stats.Suggestions)
			}
			b.WriteString("\n\n")
		} else {
			b.WriteString("## ⚠️ flycheck 发现问题\n\n")
			fmt.Fprintf(&b, "> 检查了 %s `%s`（**%d** 个文件），发现：\n",
				kind, r.Path, r.NFiles)
			fmt.Fprintf(&b, "> ⚠️ **%d** 个警告", r.Stats.Warnings)
			if r.Stats.Suggestions > 0 {
				fmt.Fprintf(&b, " / 💡 **%d** 个建议", r.Stats.Suggestions)
			}
			b.WriteString("\n\n")
		}

		b.WriteString("```\n")
		for _, iss := range r.Issues {
			b.WriteString(iss.Severity.String())
			b.WriteString(" ")
			b.WriteString(iss.Line)
			b.WriteString("\n")
		}
		b.WriteString("```\n")
	}

	// 报告失败的文件
	if len(r.FailedPkgs) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "> ⚠️ **%d 个文件检查失败：**\n", len(r.FailedPkgs))
		for i, pkg := range r.FailedPkgs {
			info := ""
			if i < len(r.FailedInfos) {
				info = " — " + r.FailedInfos[i]
			}
			fmt.Fprintf(&b, "> - `%s`%s\n", pkg, info)
		}
	}

	// 全部成功
	if b.Len() == 0 {
		fmt.Fprintf(&b, "✅ flycheck: 检查了 %s `%s`（%d 个文件），未发现问题",
			kind, r.Path, r.NFiles)
	}

	return b.String()
}
