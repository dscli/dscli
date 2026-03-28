package git

import (
	"fmt"

	"gitcode.com/dscli/dscli/internal/context"
)

func init() {
	subCommands := SubCommands()
	if len(subCommands) == 0 {
		return
	}

	RegisterTool(ToolDef{
		Name: "git",
		Description: `在当前目录运行Git命令。其中 command="git" 是为了Git运行在其他目录，例如
 - git(command="git", args=["-C", "/other/dir", "status"]) :  在 /other/dir 运行 git
 - git(command="commit" args=["-m", "commit message"]) : 在当前目录运行 git

help 子命令可以用来查看 git 帮助，例如
- git(command="help") : 查看 git 常用子命令列表，如常用子命令忘记，可以使用
- git(command="help", args=["commit"]): 查看 Git commit 子命令详细用法，如果记不清Git子命令用法，可使用
- git(command="help", args=["faq"]): 关于使用 Git 的常见问题，如果日常使用 Git 有问题，可使用
- git(command="help", args=["everyday"]): 日常 Git 的一组有用的最小命令集合, 如想提高可使用
总之 git tool 呈现 git 所有能力，有含糊就去查帮助。
`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": `Git命令或子命令`,
					"enum":        subCommands,
				},
				"args": map[string]any{
					"type":        "array",
					"description": `Git子命令或选项列表`,
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

func handleGit(ctx context.Context, toolArgs ToolArgs) (result string, suggestion string, err error) {
	name := "command"
	command := ToolArgsValue(toolArgs, name, "")
	if command == "" {
		err = fmt.Errorf("no %q property specified", name)
		suggestion = `例如，git(command="status")`
		return
	}

	args := ToolArgsValue(toolArgs, "args", []string{})
	result, suggestion, err = runGitCommand(ctx, command, args...)
	return
}
