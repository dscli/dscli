package git

import (
	"bytes"
	"os/exec"
	"strings"
	"sync"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
)

var (
	subcommands     []string
	subcommandsOnce sync.Once
)

func runGitCommand(ctx context.Context, command string, args ...string) (result string, suggestion string, err error) {
	args = append([]string{command}, args...)

	startTime := time.Now()
	outfmt.Notice("运行 git %s 命令\n", command)

	result, suggestion, err = GitCommand(ctx, args...)
	executionTime := time.Since(startTime)
	if err != nil {
		return
	}
	outfmt.Success("Git命令成功(%s)\n", executionTime.String())
	return
}

func GitCommand(ctx context.Context, args ...string) (result string, suggestion string, err error) {
	workDir := context.ContextValue(ctx, context.GitWorkingDirKey, context.ProjectRoot)
	// make sure no pager and no color
	args = append([]string{"--no-pager", "-c", "color.ui=false"}, args...)

	// 创建命令
	cmd := exec.CommandContext(ctx, "git", args...)

	// 设置工作目录（仅当非空时）
	if workDir != "" {
		cmd.Dir = workDir
	}

	// 捕获输出
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// 执行命令
	err = cmd.Run()
	result = stdoutBuf.String()
	suggestion = stderrBuf.String()
	if err != nil {
		outfmt.Error("%q 失败", cmd.String())
	}
	return
}

func gitHelp(args ...string) string {
	args = append([]string{"help"}, args...)
	ctx := context.Background()
	output, errput, err := GitCommand(ctx, args...)
	if err != nil {
		outfmt.Error("failed to run %s:%v", errput, err)
	}
	lines := []string{}
	for line := range strings.Lines(output) {
		if strings.Contains(line, "--") {
			continue
		}
		line = strings.TrimRight(line, "\n")
		lines = append(lines, line)
	}
	output = strings.Join(lines, "\n")
	return output
}

func SubCommands() []string {
	subcommandsOnce.Do(func() {
		commands := []string{}
		output := gitHelp("-a")
		if output == "" {
			return
		}

		for line := range strings.Lines(output) {
			if strings.HasPrefix(line, "  ") {
				fields := strings.Fields(line)
				command := fields[0]
				if strings.HasPrefix(command, "[") || strings.HasPrefix(command, "-") {
					continue
				}
				commands = append(commands, command)
			}
		}
		subcommands = commands
	})
	return subcommands
}
