package toolcall

import (
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

// GitOutput 封装Git命令的输出功能
type GitOutput struct {
	command string
	args    []string
	start   time.Time
}

// NewGitOutput 创建新的Git输出实例
func NewGitOutput(command string, args ...string) *GitOutput {
	return &GitOutput{
		command: command,
		args:    args,
		start:   time.Now(),
	}
}

// PrintCommand 打印正在执行的Git命令
func (g *GitOutput) PrintCommand() {
	fullCommand := fmt.Sprintf("git %s", g.command)
	if len(g.args) > 0 {
		fullCommand = fmt.Sprintf("%s %s", fullCommand, strings.Join(g.args, " "))
	}

	outfmt.Notice("执行Git命令: %s", fullCommand)

	// 显示详细参数信息（如果verbose模式）
	if len(g.args) > 0 {
		for i, arg := range g.args {
			outfmt.Debug("  参数[%d]: %s", i, arg)
		}
	}
}

// PrintResult 打印Git命令执行结果
func (g *GitOutput) PrintResult(output string, err error) {
	executionTime := time.Since(g.start)

	if err != nil {
		outfmt.Error("Git命令执行失败: %v", err)
		outfmt.Debug("执行时间: %v", executionTime)
		return
	}

	// 如果输出为空，显示成功消息
	if strings.TrimSpace(output) == "" {
		outfmt.Success("Git命令执行成功")
	} else {
		// 格式化输出
		outfmt.PrintSection("Git命令输出")
		outfmt.Println(output)
	}

	outfmt.Info("执行时间: %v", executionTime)
}

// PrintError 打印错误消息
func (g *GitOutput) PrintError(err error) {
	outfmt.Error("Git命令执行失败: %v", err)
	outfmt.Debug("执行时间: %v", time.Since(g.start))
}

// PrintInfo 打印信息消息
func (g *GitOutput) PrintInfo(format string, args ...any) {
	outfmt.Info(format, args...)
}

// PrintDebug 打印调试消息
func (g *GitOutput) PrintDebug(format string, args ...any) {
	outfmt.Debug(format, args...)
}

// PrintWarning 打印警告消息
func (g *GitOutput) PrintWarning(format string, args ...any) {
	outfmt.Warn(format, args...)
}

// PrintGitHeader 打印Git操作标题
func PrintGitHeader(operation string) {
	outfmt.PrintHeader(fmt.Sprintf("Git %s", strings.ToUpper(operation)))
}

// PrintGitSection 打印Git操作章节
func PrintGitSection(operation string) {
	outfmt.PrintSection(fmt.Sprintf("Git %s", operation))
}

// PrintGitSubSection 打印Git操作子章节
func PrintGitSubSection(operation string) {
	outfmt.PrintSubSection(fmt.Sprintf("Git %s", operation))
}

// FormatGitOutput 格式化Git输出
func FormatGitOutput(output string, operation string) string {
	if strings.TrimSpace(output) == "" {
		return fmt.Sprintf("Git %s 执行成功（无输出）", operation)
	}

	// 添加Git操作前缀
	lines := strings.Split(output, "\n")
	var formattedLines []string
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			formattedLines = append(formattedLines, fmt.Sprintf("  %s", line))
		}
	}

	return fmt.Sprintf("Git %s 输出:\n%s", operation, strings.Join(formattedLines, "\n"))
}

// PrintGitCommand 简化版：直接打印Git命令
func PrintGitCommand(args ...string) {
	if len(args) == 0 {
		return
	}

	command := args[0]
	commandArgs := args[1:]

	fullCommand := fmt.Sprintf("git %s", command)
	if len(commandArgs) > 0 {
		fullCommand = fmt.Sprintf("%s %s", fullCommand, strings.Join(commandArgs, " "))
	}

	outfmt.Notice("执行: %s", fullCommand)
}
