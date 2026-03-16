package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/shell"
	"mvdan.cc/sh/v3/syntax"
)

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

// removeShebang 移除脚本中的shebang行
func removeShebang(script string) string {
	lines := strings.Split(script, "\n")
	if len(lines) > 0 && strings.HasPrefix(lines[0], "#!") {
		// 移除shebang行
		return strings.Join(lines[1:], "\n")
	}
	return script
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

	// 确保长度不超过50字符（使用rune操作避免乱码）
	runes := []rune(summary)
	if len(runes) > 50 {
		summary = string(runes[:50])
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
						if arg != "" && len([]rune(arg)) < 20 {
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

func ArrangeArgs(ctx context.Context, cacheFile string) (context.Context, bool) {
	name := ContextValue(ctx, ShellNameKey, "")
	args := ContextValue(ctx, ShellArgsKey, []string{})
	verbose := ContextValue(ctx, VerboseKey, false)

	if verbose {
		fmt.Fprintf(os.Stderr, "ArrangeArgs: name=%s, args=%v, cacheFile=%s\n", name, args, cacheFile)
	}

	if strings.HasSuffix(name, "env") {
		if len(args) == 0 {
			return ctx, false
		}
		arg := args[0]
		switch arg {
		case "bash": // support bash
			args = append([]string{cacheFile}, args[1:]...)
			ctx = context.WithValue(ctx, ShellNameKey, "bash")
			ctx = context.WithValue(ctx, ShellArgsKey, args)
			if verbose {
				fmt.Fprintf(os.Stderr, "ArrangeArgs处理后: name=bash, args=%#v\n", args)
			}
			return ctx, true
		case "python", "python3": // support python
			args = append([]string{"-u", cacheFile}, args[1:]...)
			ctx = context.WithValue(ctx, ShellNameKey, arg)
			ctx = context.WithValue(ctx, ShellArgsKey, args)
			if verbose {
				fmt.Fprintf(os.Stderr, "ArrangeArgs处理后: name=%s, args=%#v\n", arg, args)
			}
			return ctx, true
		default:
			return ctx, false
		}
	}
	if strings.HasSuffix(name, "bash") {
		args = append([]string{cacheFile}, args...)
		ctx = context.WithValue(ctx, ShellArgsKey, args)
		if verbose {
			fmt.Fprintf(os.Stderr, "ArrangeArgs处理后: name=%s, args=%#v\n", name, args)
		}
		return ctx, true
	}
	if strings.HasSuffix(name, "python") || strings.HasSuffix(name, "python3") {
		args = append([]string{"-u", cacheFile}, args...)
		ctx = context.WithValue(ctx, ShellArgsKey, args)
		if verbose {
			fmt.Fprintf(os.Stderr, "ArrangeArgs处理后: name=%s, args=%#v\n", name, args)
		}
		return ctx, true
	}
	return ctx, false
}

// ShellExec 是混合实现的 Shell 执行函数
// 对于 shell 命令使用新的 internal/shell 包
// 对于非 shell 命令回退到原始的 os/exec 实现
// ShellExec 执行 shell 脚本
func ShellExec(ctx context.Context, script string) (out string, err error) {
	verbose := ContextValue(ctx, VerboseKey, false)
	// 解析 shebang
	name, shellArgs := Shebang(script)
	if verbose {
		fmt.Fprintf(os.Stderr, "解析到的命令: %s, 参数: %v\n", name, shellArgs)
	}

	// 移除 shebang 行
	script = removeShebang(script)
	name, shellArgs = ContextValue(ctx, ShellNameKey, name), ContextValue(ctx, ShellArgsKey, shellArgs)
	// 设置上下文值
	ctx = context.WithValue(ctx, ShellNameKey, name)
	ctx = context.WithValue(ctx, ShellArgsKey, shellArgs)

	// 判断是否是 Python 命令
	isPythonCommand := strings.Contains(name, "python")
	// 检查shellArgs是否包含python
	if len(shellArgs) > 0 && strings.Contains(shellArgs[0], "python") {
		isPythonCommand = true
	}

	// 使用IsShellScript判断是否是有效的Shell脚本
	isShellScript, shellScriptErr := shell.IsShellScript(ctx, script)

	// 判断是否是 shell 命令
	// 逻辑优先级：
	// 1. 如果是Python命令，肯定不是shell命令
	// 2. 如果不是Python命令，且是有效的Shell脚本，则是shell命令
	// 3. 如果不是Python命令，但不是有效的Shell脚本，则使用原来的简单逻辑（不是Python就是Shell）
	//    这是为了向后兼容，因为有些简单的命令可能被误判
	isShellCommand := false
	if isPythonCommand {
		isShellCommand = false
	} else if isShellScript {
		isShellCommand = true
	} else {
		// 使用原来的简单逻辑：不是Python就是Shell
		isShellCommand = true
		if verbose && shellScriptErr != nil {
			fmt.Fprintf(os.Stderr, "警告：脚本不是有效的Shell语法，但为了兼容性仍然按Shell处理: %v\n", shellScriptErr)
		}
	}

	// 检查是否需要stdin
	hasStdin := ContextValue(ctx, ShellStdinKey, io.Reader(nil)) != nil

	if verbose {
		fmt.Fprintf(os.Stderr, "命令: %s, 参数: %v, 是shell命令: %v, 是Python命令: %v, 需要stdin: %v\n", name, shellArgs, isShellCommand, isPythonCommand, hasStdin)
		if shellScriptErr != nil {
			fmt.Fprintf(os.Stderr, "Shell脚本检查错误: %v\n", shellScriptErr)
		}
	}

	// 测试模式下不允许执行dscli，避免引起递归调用
	if IsTesting() && isShellCommand {
		script = strings.ReplaceAll(script, "dscli", "echo dscli")
	}

	if isShellCommand && !hasStdin {
		if verbose {
			fmt.Fprintf(os.Stderr, "使用 internal/shell 包执行\n")
		}
		// 使用新的 shell 包执行（仅当不需要stdin时）
		return executeWithShellPackage(ctx, script)
	} else {
		if verbose {
			fmt.Fprintf(os.Stderr, "使用原始的 os/exec 执行\n")
		}
		// 对于非shell命令、Python命令或需要stdin的shell命令，使用缓存文件机制
		cacheFile, err := GetOrCreateCacheFile(ctx, script)
		if err != nil {
			return "", err
		}
		// 使用 ArrangeArgs 处理参数
		ctx, ok := ArrangeArgs(ctx, cacheFile)
		if !ok {
			return "", fmt.Errorf("do not support %s %v", name, shellArgs)
		}

		name = ContextValue(ctx, ShellNameKey, "")
		shellArgs = ContextValue(ctx, ShellArgsKey, []string{})

		// 获取stdin
		stdin := ContextValue(ctx, ShellStdinKey, io.Reader(nil))
		if stdin == nil {
			stdin = strings.NewReader("")
		}

		// 回退到原始的 os/exec 实现
		return executeWithOSExec(ctx, name, shellArgs, script, stdin)
	}
}

// executeWithShellPackage 使用新的 shell 包执行命令
func executeWithShellPackage(ctx context.Context, script string) (out string, err error) {
	// 创建 shell 配置
	config := &shell.Config{
		WorkingDir:  ProjectRoot,
		Timeout:     60 * time.Second,
		StrictMode:  true,
		SandboxMode: !IsTesting(),
		EnvVars:     append(os.Environ(), "InsideShellExec=1"),
		SandboxConfig: &shell.SandboxConfig{
			AllowedCommands: []string{
				"bash", "sh", "zsh",
				"echo", "ls", "cat", "git", "find", "grep",
				"mkdir", "rm", "cp", "mv", "chmod", "chown",
				"curl", "wget", "tar", "gzip", "unzip",
				"/usr/bin/env", "/bin/bash", "/bin/sh",
			},
			AllowedPaths: []string{ProjectRoot},
		},
	}

	// 创建执行器
	executor := shell.NewExecutor(config)

	// 执行脚本
	result, execErr := executor.Execute(ctx, script)
	if execErr != nil {
		return "", fmt.Errorf("failed to create executor: %w", execErr)
	}

	if result.Err != nil {
		// 检查是否超时
		if ctx.Err() != nil {
			if ctx.Err() == context.DeadlineExceeded {
				return result.Stdout, fmt.Errorf("命令执行超时")
			}
			return result.Stdout, fmt.Errorf("命令被取消: %w", ctx.Err())
		}

		return result.Stdout, fmt.Errorf("命令执行失败: %w", result.Err)
	}

	return result.Stdout, nil
}

// executeWithOSExec 使用原始的 os/exec 实现执行命令
// executeWithOSExec 使用原始的 os/exec 实现执行命令
func executeWithOSExec(ctx context.Context, name string, args []string, script string, stdin io.Reader) (out string, err error) {
	// 这是原始的 ShellExec 实现的核心部分
	name = ContextValue(ctx, ShellNameKey, name)
	args = ContextValue(ctx, ShellArgsKey, args)
	buf := bytes.NewBuffer([]byte{})
	subproc := exec.CommandContext(ctx, name, args...)
	subproc.Dir = ProjectRoot
	subproc.Stdout = buf
	subproc.Stderr = buf
	subproc.Stdin = stdin
	subproc.Env = append(os.Environ(), "InsideShellExec=1")
	err = subproc.Start()
	if err != nil {
		err = fmt.Errorf("failed to start %s: %w", name, err)
		return
	}
	err = subproc.Wait()
	out = buf.String()

	// 检查是否被取消或超时
	if ctx.Err() != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return out, fmt.Errorf("命令执行超时")
		}
		return out, fmt.Errorf("命令被取消: %w", ctx.Err())
	}

	if err != nil {
		// 提供更详细的错误信息
		exitErr, ok := err.(*exec.ExitError)
		if ok {
			return out, fmt.Errorf("命令执行失败 (退出码: %d): %s", exitErr.ExitCode(), exitErr.String())
		}
		return out, fmt.Errorf("命令执行失败: %w", err)
	}

	return out, nil
}
