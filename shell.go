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
		Timeout:  60 * time.Second, // 设置60秒超时
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
		Timeout:  60 * time.Second, // 设置60秒超时
		Handler:  handlePython,
	})
}

// 危险命令列表
var dangerousCommands = []string{
	// 系统破坏性命令
	"rm -rf /",
	"rm -rf /*",
	"rm -rf /etc",
	"rm -rf /bin",
	"rm -rf /usr",
	"rm -rf /var",
	"rm -rf /lib",
	"rm -rf /opt",
	"rm -rf /sbin",
	"rm -rf /boot",
	"rm -rf /dev",
	"rm -rf /proc",
	"rm -rf /sys",
	"rm -rf ~",
	"rm -rf $HOME",

	// 磁盘操作命令
	"dd if=/dev/zero",
	"dd of=/dev/sd",
	"mkfs",
	"fdisk",
	"parted",
	"wipefs",

	// 系统关键文件
	"chmod 000 /etc",
	"chmod 777 /etc/passwd",
	"chmod 777 /etc/shadow",

	// 网络破坏
	"iptables -F",
	"iptables -X",
	"iptables -t nat -F",

	// 进程管理
	"kill -9 -1",
	"killall",
	"pkill",

	// 内存/CPU破坏
	":(){ :|:& };:", // fork炸弹
	"cat /dev/urandom",

	// 服务管理
	"systemctl stop",
	"service stop",
	"init 0",
	"init 6",
	"shutdown",
	"halt",
	"poweroff",
	"reboot",
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

// checkDangerousCommands 检查脚本中是否包含危险命令
func checkDangerousCommands(script string) error {
	// 转换为小写进行不区分大小写的检查
	lowerScript := strings.ToLower(script)

	for _, dangerousCmd := range dangerousCommands {
		if strings.Contains(lowerScript, strings.ToLower(dangerousCmd)) {
			return fmt.Errorf("检测到危险命令: %s", dangerousCmd)
		}
	}

	return nil
}

func validateShell(script string) (err error) {
	// 检查危险命令
	if err := checkDangerousCommands(script); err != nil {
		return err
	}

	// 检查是否运行 dscli
	commands, err := parseCommands(script)
	if err != nil {
		return err
	}

	if slices.Contains(commands, "./dscli") {
		return fmt.Errorf("do not run dscli in dscli")
	}

	return nil
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

	// 检查危险命令（Python脚本也可能包含shell命令）
	if err := checkDangerousCommands(script); err != nil {
		return "", err
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

func shellExec(ctx context.Context, script string, name string, arg []string) (out string, err error) {
	buf := bytes.NewBuffer([]byte{})
	subproc := exec.CommandContext(ctx, name, arg...)
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

func runShell(ctx context.Context, script string) (result string, err error) {
	startTime := time.Now()
	name, arg := Shebang(script)

	out, err := shellExec(ctx, script, name, arg)
	executionTime := time.Since(startTime)

	if err != nil {
		// 构建包含执行统计的失败结果
		result := fmt.Sprintf("❌ 执行失败:\n错误: %v\n\n输出内容:\n%s\n\n📊 执行统计:\n执行时间: %v\n状态: 失败",
			err, out, executionTime)
		return result, nil
	}

	// 构建包含执行统计的成功结果
	result = fmt.Sprintf("📝 执行结果:\n%s\n\n📊 执行统计:\n执行时间: %v\n状态: 成功",
		out, executionTime)

	return
}
