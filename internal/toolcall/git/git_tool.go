package git

import (
	"fmt"
	"slices"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
)

func init() {
	subCommands := SubCommands()
	if len(subCommands) == 0 {
		return
	}

	RegisterTool(ToolDef{
		Name: "git",
		Description: `在当前目录运行Git命令。例如
 - git(command="commit" args=["-m", "commit message"]) : 在当前目录运行 git

help 子命令可以用来查看 git 帮助，例如
- git(command="help") : 查看 git 常用子命令列表
- git(command="help", args=["commit"]): 查看 Git 子命令详细用法
- git(command="help", args=["faq"]): 关于使用 Git 的常见问题
- git(command="help", args=["everyday"]): 日常 Git 的一组有用的最小命令集合
总之 git tool 呈现 git 所有能力，不清楚可查帮助。
`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": `Git子命令`,
					"enum":        subCommands,
				},
				"-C": map[string]any{
					"type": "string",
					"description": `Git 工作目录，对应 git -C <路径> ，默认当前目录。`,
				},
				"args": map[string]any{
					"type":        "array",
					"description": `Git子命令选项列表`,
					"items": map[string]string{
						"type":        "string",
						"description": "Git子命令或选项",
					},
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGit,
	})
}

func SliceString(args []string) string {
	return strings.Join(func() []string {
		qargs := make([]string, len(args))
		for i, arg := range args {
			qargs[i] = fmt.Sprintf("%q", arg)
		}
		return qargs
	}(), ",")
}

func handleGit(ctx context.Context, toolArgs ToolArgs) (result string, suggestion string, err error) {
	name := "command"
	command := ToolArgsValue(toolArgs, name, "")
	if command == "" {
		err = fmt.Errorf("no %q property specified", name)
		suggestion = `例如，git(command="status")`
		return
	}

	commands := SubCommands()
	if !slices.Contains(commands, command) {
		err = fmt.Errorf("command %q is not supported", command)
		suggestion = fmt.Sprintf(`应该在%s中选择命令`, SliceString(commands))
		return
	}

	args := ToolArgsValue(toolArgs, "args",
		ToolArgsValue(toolArgs, "arguments", []string{}))

	gitWorkingDir := ToolArgsValue(toolArgs, "-C", context.ProjectRoot)
	ctx = context.WithValue(ctx, context.GitWorkingDirKey, gitWorkingDir)
	result, suggestion, err = runGitCommand(ctx, command, args...)
	if err != nil {
		if suggestion == "" && result != "" {
			suggestion = result
			result = fmt.Sprintf("git(command=%q, args=[%s]) 失败", command, SliceString(args))
		}
	}
	return
}
