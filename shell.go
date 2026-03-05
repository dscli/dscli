package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"slices"
	"strings"
	"time"

	"mvdan.cc/sh/v3/syntax"
)

func init() {
	// 注册shell工具
	RegisterTool(ToolDef{
		Name: "shell",
		Description: `在项目根目录执行Shell脚本。
支持shebang指定解释器（如bash、sh等）。
脚本通过标准输入传递，避免命令行长度限制。

输出格式：
- 成功时：返回包含执行结果和执行统计的格式化文本
- 失败时：返回包含错误信息、输出内容和执行统计的格式化文本

示例：
1. Bash脚本：echo "Hello"
2. Shell脚本：ls -la
3. 文件操作：cat file.txt
4. Git操作：git status

注意：谨慎使用，避免破坏性操作。确保脚本在项目目录内执行。`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type": "string",
					"description": `要执行的Shell脚本内容。
脚本执行结果会以格式化文本返回，包含执行统计信息。

示例：
1. Bash脚本：echo "Hello"
2. Shell脚本：ls -la
3. 文件操作：cat file.txt
4. Git操作：git status
`,
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		},
		Category: "system",
		Handler:  handleShell,
	})

	// 注册python工具
	RegisterTool(ToolDef{
		Name: "python",
		Description: `在项目根目录执行Python脚本。
脚本通过标准输入传递，避免命令行长度限制。

输出格式：
- 成功时：返回包含执行结果和执行统计的格式化文本
- 失败时：返回包含错误信息、输出内容和执行统计的格式化文本

示例：
1. Python脚本：print("Hello")
2. 数据处理：import json; print(json.dumps({"key": "value"}))
3. 文件操作：with open("file.txt", "r") as f: print(f.read())

注意：谨慎使用，避免破坏性操作。确保脚本在项目目录内执行。`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type": "string",
					"description": `要执行的Python脚本内容。
支持shebang指定解释器（如#!/usr/bin/env python, #!/usr/bin/env python3）。
脚本执行结果会以格式化文本返回，包含执行统计信息。

示例：
1. Python脚本：print("Hello")
2. 数据处理：import json; print(json.dumps({"key": "value"}))
3. 文件操作：with open("file.txt", "r") as f: print(f.read())
`,
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		},
		Category: "system",
		Handler:  handlePython,
	})
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

func parseCommands(script string) (commands []string, err error) {
	parser := syntax.NewParser()
	reader := strings.NewReader(script)
	sf, err := parser.Parse(reader, "script.sh")
	if err != nil {
		return
	}
	syntax.Walk(sf, func(node syntax.Node) bool {
		// 我们关心的是 *syntax.CallExpr 节点，它代表一个命令调用
		if call, ok := node.(*syntax.CallExpr); ok {
			// 命令名通常是 Args 切片中的第一个参数
			if len(call.Args) > 0 {
				// Args[0] 是一个 *syntax.Word，我们需要获取它的字面值
				// Lit() 方法可以返回单词的字面字符串
				cmdName := call.Args[0].Lit()
				if cmdName != "" {
					commands = append(commands, cmdName)
				}
			}
		}
		// 返回 true 表示继续遍历子节点
		return true
	})
	return
}

func validateShell(script string) (err error) {
	commands, err := parseCommands(script)
	if err != nil {
		return
	}

	if slices.Contains(commands, "./dscli") {
		return fmt.Errorf("do not run dscli in dscli")
	}
	return
}

// handleShell 执行Shell脚本
func handleShell(ctx context.Context, args map[string]string) (out string, err error) {
	script, ok := args["script"]
	if !ok {
		script = ""
	}

	if err = validateShell(script); err != nil {
		return
	}

	Notice("Shell: %s", ShortenShellScript(script))
	out, err = runShell(ctx, script)
	return
}

// handlePython 执行Python脚本
func handlePython(ctx context.Context, args map[string]string) (out string, err error) {
	script, ok := args["script"]
	if !ok {
		script = ""
	}

	// 如果没有shebang，添加默认的python shebang
	if !strings.HasPrefix(strings.TrimSpace(script), "#!") {
		script = "#!/usr/bin/env python3\n" + script
	}

	out, err = runShell(ctx, script)
	return
}

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

func ShellExec(ctx context.Context, script string) (out string, err error) {
	name, arg := Shebang(script)
	out, err = shellExec(ctx, script, name, arg)
	return
}

func shellExec(cxt context.Context, script string, name string, arg []string) (out string, err error) {
	buf := bytes.NewBuffer([]byte{})
	subproc := exec.Command(name, arg...)
	subproc.Dir = ProjectRoot
	subproc.Stdout = buf
	subproc.Stderr = buf
	stdin, err := subproc.StdinPipe()
	if err != nil {
		err = fmt.Errorf("failed to get stdin pipe: %w", err)
		return
	}
	err = subproc.Start()
	if err != nil {
		err = fmt.Errorf("failed to start %s: %w", name, err)
		return
	}
	n, err := io.WriteString(stdin, fmt.Sprintf("%s\n", script))
	if err != nil {
		err = fmt.Errorf("failed to write string at %d: %w", n, err)
		return
	}
	err = stdin.Close()
	if err != nil {
		err = fmt.Errorf("failed to close stdin: %w", err)
		return
	}

	err = subproc.Wait()
	out = buf.String()
	if err != nil {
		return out, err
	}
	return out, nil
}

func runShell(ctx context.Context, script string) (result string, err error) {
	startTime := time.Now()
	name, arg := Shebang(script)

	out, err := shellExec(ctx, script, name, arg)
	executionTime := time.Since(startTime)
	if err != nil {
		// 构建包含执行统计的失败结果
		result := fmt.Sprintf(`❌ 执行失败:
错误: %v

输出内容:
%s

📊 执行统计:
执行时间: %v
状态: 失败`,
			err, out, executionTime)
		return result, nil
	}

	// 构建包含执行统计的成功结果
	result = fmt.Sprintf(`📝 执行结果:
%s

📊 执行统计:
执行时间: %v
状态: 成功`,
		out, executionTime)

	return
}
