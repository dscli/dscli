package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

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

// ShortenShellScript for display
func ShortenShellScript(script string) string {
	script = strings.ReplaceAll(script, ProjectRoot, ".")
	// 处理空字符串
	if script == "" {
		return ""
	}

	lines := []string{}
	n := 0
	for line := range strings.Lines(script) {
		line = strings.TrimSpace(line)
		line = strings.Map(func(r rune) rune {
			if r > 127 {
				return -1
			}
			return r
		}, line)
		if strings.HasPrefix(line, "#") ||
			strings.HasPrefix(line, "//") {
			continue
		}
		lines = append(lines, line)
		n += len(line)
		if n > 50 { // we need 50 most
			break
		}
	}

	script = strings.Join(lines, "; ")
	if len(script) > 50 {
		return script[0:50]
	}
	return script
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
		return "", fmt.Errorf("do not support %s %v", name, arg)
	}

	shellStdin := ContextValue(ctx, ShellStdin, io.Reader(os.Stdin))
	r, w, err := os.Pipe()
	if err != nil {
		return "", err
	}

	defer func() {
		if r != nil {
			r.Close()
		}
		if w != nil {
			w.Close()
		}
	}()

	buf := bytes.NewBuffer([]byte{})
	subproc := exec.CommandContext(ctx, name, arg...)
	Println(subproc)
	subproc.Dir = ProjectRoot
	subproc.Stdout = buf
	subproc.Stderr = buf
	subproc.Stdin = shellStdin
	subproc.ExtraFiles = []*os.File{r}
	err = subproc.Start()
	if err != nil {
		err = fmt.Errorf("failed to start %s: %w", name, err)
		return
	}
	_ = r.Close()
	r = nil
	_, err = io.WriteString(w, script)
	if err != nil {
		err = fmt.Errorf("failed to write stdin: %w", err)
		return
	}
	_ = w.Close()
	w = nil
	if err != nil {
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
