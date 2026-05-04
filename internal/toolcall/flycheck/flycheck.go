// Package flycheck registers the flycheck tool for LLM-driven syntax checking.
package flycheck

import (
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/flycheck"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "flycheck",
		Description: `对指定文件/目录/包运行静态语法/语义检查，类似 Emacs flycheck。

参数：
  path: 路径（必需），相对于项目根目录。支持三种模式：
        - 文件：检查该文件（Go 文件检查所属包，Python 文件直接检查）
        - 目录：检查该目录中的所有 Go 包或 Python 文件（如 internal/toolcall/）
        - 递归：目录路径后缀 "..." 递归检查所有子包/文件（如 internal/...）

支持语言：
  Go     — staticcheck（自动发现包，包级检查）
  Python — ruff（快速 linter，支持单文件和目录检查）

工作原理：
1. 解析路径模式，发现所有待检查的 Go 包或 Python 文件
2. 对每个包/文件运行对应的检查器（staticcheck / ruff）
3. 汇总所有问题并返回统计信息

返回结果：
- 检查发现问题时：返回统计摘要 + file:line:col: message 格式的诊断信息
- 检查通过时：返回 "✅ flycheck: 检查了 X 个包（Y 个文件），未发现问题"
- 检查器未安装时：返回安装提示

优势：
1. 自动发现 unused import、unused variable 等静态分析问题
2. 支持 Go 和 Python 语言
3. 支持单文件、目录、递归三种粒度
4. 返回统计信息帮助判断检查覆盖范围

示例：
  # 检查 Go 文件所在包
  flycheck(path="internal/flycheck/flycheck.go")

  # 检查 Python 文件
  flycheck(path="my_script.py")

  # 检查目录下所有包/文件
  flycheck(path="internal/toolcall/")

  # 递归检查所有子包/文件
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

func handleFlycheck(ctx context.Context, args toolcall.ToolArgs) (result string, warning string, err error) {
	path := toolcall.ToolArgsValue(args, "path", "")
	if path == "" {
		err = fmt.Errorf("参数 'path' 缺失")
		return
	}

	checkResult, checkErr := flycheck.CheckPath(ctx, path)

	// 处理错误
	if checkErr != nil {
		if checkResult != nil && checkResult.Suggestion != "" {
			warning = fmt.Sprintf("💡 %s\n\n%s", checkErr.Error(), checkResult.Suggestion)
		}
		err = checkErr
		return
	}

	// 语言不支持
	if !checkResult.Supported {
		kind := "文件"
		if checkResult.Mode == "package" {
			kind = "目录"
		}
		result = fmt.Sprintf("ℹ️ flycheck 暂不支持 %s 语言（%s: %s）\n   目前支持 Go 和 Python 语言。如需支持其他语言请联系开发者。",
			checkResult.Language, kind, checkResult.Path)
		return
	}

	// 非 Go/Python 目录检查 → 单文件检查
	if checkResult.Mode == "file" {
		if checkResult.RawOutput != "" {
			result = fmt.Sprintf("> 检查文件: %s\n\n%s", checkResult.Path, checkResult.RawOutput)
		} else {
			result = fmt.Sprintf("✅ flycheck: 检查了 %s，未发现问题", checkResult.Path)
		}
		return
	}


	// 包/目录检查 → Markdown 格式化
	result = formatPackageResult(checkResult)
	return
}

// formatPackageResult outputs package/directory check results in Markdown.
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
			b.WriteString(fmt.Sprintf("## ❌ flycheck 发现%s — 必须立即修复！\n\n", errKindWord))
			b.WriteString(fmt.Sprintf("> 检查了 **%d %s**（**%d %s**），发现：\n",
				r.NPkgs, unitWord, r.NFiles, fileWord))
			b.WriteString(fmt.Sprintf("> ❌ **%d** %s", r.Stats.Errors, errKindWord))
			if r.Stats.Warnings > 0 {
				b.WriteString(fmt.Sprintf(" / ⚠️ **%d** 个警告", r.Stats.Warnings))
			}
			if r.Stats.Suggestions > 0 {
				b.WriteString(fmt.Sprintf(" / 💡 **%d** 个建议", r.Stats.Suggestions))
			}
			b.WriteString("\n\n")
		} else {
			b.WriteString("## ⚠️ flycheck 发现问题\n\n")
			b.WriteString(fmt.Sprintf("> 检查了 **%d %s**（**%d %s**），发现：\n",
				r.NPkgs, unitWord, r.NFiles, fileWord))
			b.WriteString(fmt.Sprintf("> ⚠️ **%d** 个警告", r.Stats.Warnings))
			if r.Stats.Suggestions > 0 {
				b.WriteString(fmt.Sprintf(" / 💡 **%d** 个建议", r.Stats.Suggestions))
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
		b.WriteString(fmt.Sprintf("> ⚠️ **%d %s检查失败：**\n", len(r.FailedPkgs), failWord))
		for i, pkg := range r.FailedPkgs {
			info := ""
			if i < len(r.FailedInfos) {
				info = " — " + r.FailedInfos[i]
			}
			b.WriteString(fmt.Sprintf("> - `%s`%s\n", pkg, info))
		}
	}

	// 全部成功
	if b.Len() == 0 {
		b.WriteString(fmt.Sprintf("✅ flycheck: 检查了 %d %s（%d %s），未发现问题",
			r.NPkgs, unitWord, r.NFiles, fileWord))
	}

	return b.String()
}
