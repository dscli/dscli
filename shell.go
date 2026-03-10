package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"mvdan.cc/sh/v3/syntax"
)

// ShellError 表示shell执行错误
type ShellError struct {
	ExitCode int
	Command  string
	Output   string
	Err      error
	Context  string
}

func (e *ShellError) Error() string {
	if e.ExitCode == -1 {
		return fmt.Sprintf("%s: %v\n命令: %s", e.Context, e.Err, e.Command)
	}
	return fmt.Sprintf("%s (退出码: %d): %v\n命令: %s\n输出: %s",
		e.Context, e.ExitCode, e.Err, e.Command, e.Output)
}

func (e *ShellError) Unwrap() error {
	return e.Err
}

func IsTesting() bool {
	return strings.HasSuffix(os.Args[0], ".test")
}

func Shebang(script string) (name string, arg []string) {
	shebang := []string{"/usr/bin/env", "bash"}
	before, _, ok := strings.Cut(script, "\n")
	if ok {
		line1 := before
		if strings.HasPrefix(line1, "#!") {
			shebang = strings.Fields(line1[2:])
		}
	}
	name = shebang[0]
	arg = shebang[1:]
	return
}

// ShortenShellScript 生成脚本的简短摘要
func ShortenShellScript(script string) string {
	// 处理空字符串
	if script == "" {
		return ""
	}

	// 移除项目根目录路径（只有在ProjectRoot不为空时）
	if ProjectRoot != "" {
		script = strings.ReplaceAll(script, ProjectRoot, ".")
	}

	// 移除非ASCII字符
	script = strings.Map(func(r rune) rune {
		if r > 127 {
			return -1
		}
		return r
	}, script)
	// 使用语法解析生成摘要
	summary := shortenWithSyntaxAnalysis(script)

	// 如果语法解析失败或结果为空，使用简单回退
	if summary == "" {
		summary = shortenSimple(script)
	}

	// 确保长度不超过50字符
	if len(summary) > 50 {
		summary = summary[:50]
	}

	return summary
}

// shortenWithSyntaxAnalysis 使用语法分析生成有意义的摘要
func shortenWithSyntaxAnalysis(script string) string {
	parser := syntax.NewParser()
	reader := strings.NewReader(script)
	sf, err := parser.Parse(reader, "script.sh")
	if err != nil {
		return "" // 解析失败
	}

	// 收集所有命令（排除echo命令）
	var commands []string
	syntax.Walk(sf, func(node syntax.Node) bool {
		switch n := node.(type) {
		case *syntax.CallExpr:
			if len(n.Args) > 0 {
				cmd := n.Args[0].Lit()
				if cmd != "" && !strings.HasPrefix(cmd, "#!") {
					// 跳过echo命令（视为不重要）
					if cmd == "echo" {
						return true
					}
					// 添加命令和最多一个参数
					cmdStr := cmd
					if len(n.Args) > 1 {
						arg := n.Args[1].Lit()
						if arg != "" && len(arg) < 20 {
							cmdStr += " " + arg
						}
					}
					commands = append(commands, cmdStr)
				}
			}
		}
		return true
	})

	if len(commands) == 0 {
		return ""
	}

	// 构建摘要：最多显示3个命令
	maxCommands := 3
	if len(commands) > maxCommands {
		commands = commands[:maxCommands]
		return strings.Join(commands, "; ") + "..."
	}

	return strings.Join(commands, "; ")
}

// shortenSimple 简单的回退方法
func shortenSimple(script string) string {
	lines := []string{}

	for line := range strings.Lines(script) {
		line = strings.TrimSpace(line)

		// 跳过注释和shebang
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		if line == "" {
			continue
		}

		// 跳过echo命令（视为不重要）
		if strings.HasPrefix(line, "echo ") || line == "echo" {
			continue
		}

		lines = append(lines, line)

		// 如果已经收集了足够的内容，停止
		if len(lines) >= 3 {
			break
		}
	}

	if len(lines) == 0 {
		return ""
	}

	return strings.Join(lines, "; ")
}

func ArrangeArgs(name string, args []string) ([]string, bool) {
	if strings.HasSuffix(name, "env") {
		if len(args) == 0 {
			return args, false
		}
		arg := args[0]
		switch arg {
		case "bash": // support bash
			args = append([]string{"bash", "/dev/fd/3"}, args[1:]...)
			return args, true
		case "python", "python3": // support python
			args = append([]string{args[0], "-u", "/dev/fd/3"}, args[1:]...)
			return args, true
		default:
			return args, false
		}
	}
	if strings.HasSuffix(name, "bash") {
		args = append([]string{"/dev/fd/3"}, args...)
		return args, true
	}
	if strings.HasSuffix(name, "python") || strings.HasSuffix(name, "python3") {
		args = append([]string{"-u", "/dev/fd/3"}, args...)
		return args, true
	}
	return args, false
}

func ShellExec(ctx context.Context, script string) (out string, err error) {
	name := ContextValue(ctx, ShellName, "")
	arg := ContextValue(ctx, ShellArgs, []string{})
	if name == "" {
		name, arg = Shebang(script)
	}
	arg, ok := ArrangeArgs(name, arg)
	if !ok {
		return "", fmt.Errorf("不支持的命令: %s %v", name, arg)
	}

	shellStdin := ContextValue(ctx, ShellStdin, io.Reader(os.Stdin))
	r, w, err := os.Pipe()
	if err != nil {
		return "", fmt.Errorf("创建管道失败: %w", err)
	}

	// 使用defer确保资源清理
	defer func() {
		cleanupResources(r, w)
	}()

	if IsTesting() {
		script = strings.ReplaceAll(script, "dscli", "echo dscli")
	}

	buf := bytes.NewBuffer([]byte{})
	subproc := exec.CommandContext(ctx, name, arg...)
	subproc.Dir = ProjectRoot
	subproc.Stdout = buf
	subproc.Stderr = buf
	subproc.Stdin = shellStdin
	subproc.Env = append(os.Environ(), "InsideShellExec=1")
	subproc.ExtraFiles = []*os.File{r}

	// 记录命令摘要用于错误信息
	cmdSummary := ShortenShellScript(script)
	if cmdSummary == "" {
		cmdSummary = script
		if len(cmdSummary) > 50 {
			cmdSummary = cmdSummary[:50] + "..."
		}
	}

	err = subproc.Start()
	if err != nil {
		return "", &ShellError{
			ExitCode: -1,
			Command:  cmdSummary,
			Output:   "",
			Err:      fmt.Errorf("启动进程失败: %w", err),
			Context:  "进程启动失败",
		}
	}

	// 确保进程在函数结束时被清理
	defer cleanupProcess(subproc)

	_ = r.Close()
	r = nil
	_, err = io.WriteString(w, script)
	if err != nil {
		return "", &ShellError{
			ExitCode: -1,
			Command:  cmdSummary,
			Output:   "",
			Err:      fmt.Errorf("写入脚本失败: %w", err),
			Context:  "脚本输入失败",
		}
	}
	_ = w.Close()
	w = nil

	err = subproc.Wait()
	out = buf.String()

	// 检查是否被取消或超时
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out, &ShellError{
				ExitCode: -1,
				Command:  cmdSummary,
				Output:   out,
				Err:      fmt.Errorf("命令执行超时"),
				Context:  "执行超时",
			}
		}
		return out, &ShellError{
			ExitCode: -1,
			Command:  cmdSummary,
			Output:   out,
			Err:      fmt.Errorf("命令被取消: %w", ctx.Err()),
			Context:  "执行被取消",
		}
	}

	if err != nil {
		exitCode := 1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}

		return out, &ShellError{
			ExitCode: exitCode,
			Command:  cmdSummary,
			Output:   out,
			Err:      err,
			Context:  "命令执行失败",
		}
	}

	return out, nil
}

// cleanupResources 清理管道资源
func cleanupResources(r, w *os.File) {
	if r != nil {
		r.Close()
	}
	if w != nil {
		w.Close()
	}
}

// cleanupProcess 清理进程资源
func cleanupProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}

	// 如果进程还在运行，尝试终止
	if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
		cmd.Process.Kill()
	}
}
