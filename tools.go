package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

var ToolDisplayName = &struct{}{}

// toolRegistry 工具注册表
var toolRegistry = map[string]ToolDef{}

func GetToolDisplayName(name string) string {
	words := strings.Split(name, "_")
	for i, word := range words {
		word = strings.ToUpper(word[0:1]) + word[1:]
		words[i] = word
	}
	return strings.Join(words, "")
}

// RegisterTool 注册工具
func RegisterTool(tool ToolDef) {
	tool.DisplayName = GetToolDisplayName(tool.Name)
	toolRegistry[tool.Name] = tool
}

// GetAllTools 获取所有工具定义（用于API调用）
func GetAllTools() []Tool {
	if ModelID == DeepseekReasoner {
		return nil
	}

	var tools []Tool
	for name, def := range toolRegistry {
		tools = append(tools, Tool{
			Type: "function",
			Function: Function{
				Name:        name,
				Description: def.Description,
				Parameters:  def.Parameters,
			},
		})
	}
	return tools
}

// HandleToolCalls 处理工具调用（带统计）
func HandleToolCalls(ctx context.Context, tcs []ToolCall) []Message {
	inputs := []Message{}
	// 处理每个工具调用
	for _, tc := range tcs {
		// 使用新的工具调用处理器
		result, err := HandleToolCall(ctx, tc.Function.Name, []byte(tc.Function.Arguments))
		if err != nil {
			// But we still need to tell the result to assistant
			result = err.Error()
		}

		inputs = append(inputs, Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    result,
		})
	}
	return inputs
}

// ==================== 工具处理器实现 ====================

// 解析文件路径：如果是相对路径，则拼接项目根目录；否则直接使用
func resolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(ProjectRoot, path)
}

// handleReadFile 读取文件（纯Go实现）
func handleReadFile(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("parameter error: no path specified")
	}

	fullPath := resolvePath(path)

	// 读取文件
	startTime := time.Now()
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// 获取文件信息
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to fetch file information: %w", err)
	}

	// 构建结果
	executionTime := time.Since(startTime)
	result := fmt.Sprintf(`📄 文件内容:
内容:
%s

文件信息:
- 路径: %s
- 大小: %d 字节
- 权限: %s
- 修改时间: %s

📊 执行统计:
执行时间: %v
状态: 成功`,
		string(content),
		fullPath,
		fileInfo.Size(),
		fileInfo.Mode().String(),
		fileInfo.ModTime().Format("2006-01-02 15:04:05"),
		executionTime)
	Notice("读取文件: \"%s\"（%d字节）", fullPath, fileInfo.Size())
	return result, nil
}

func Shuffle(in string) (out string) {
	runes := []rune(in)
	rand.Shuffle(len(runes), func(i, j int) {
		runes[i], runes[j] = runes[j], runes[i]
	})
	out = string(runes)
	return
}

// handleWriteFile 写入文件（纯Go实现）
func handleWriteFile(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok || path == "" {
		return "", fmt.Errorf("参数错误: 缺少path参数")
	}

	fullPath := resolvePath(path)

	// 确保目录存在
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("创建目录%q失败: %w", dir, err)
	}

	content, ok := args["content"]
	if !ok { // no content specified means touch
		content = ""
	}
	// 写入文件
	startTime := time.Now()
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}

	// 获取文件信息用于统计
	fileInfo, err := os.Stat(fullPath)
	if err != nil {
		return "", fmt.Errorf("获取文件信息失败: %w", err)
	}

	// 构建成功响应
	executionTime := time.Since(startTime)
	result := fmt.Sprintf(`✅ 写入成功:
已成功写入文件: \"%s\"
文件大小: %d 字节
权限: %s
路径: %s

📊 执行统计:
执行时间: %v
状态: 成功`,
		path,
		fileInfo.Size(),
		fileInfo.Mode().String(),
		fullPath,
		executionTime)

	Notice("写入文件: \"%s\"（%d字节）", fullPath, fileInfo.Size())
	return result, nil
}

// handleSearchFiles 搜索文件
func handleSearchFiles(ctx context.Context, args map[string]string) (string, error) {
	pattern, ok := args["pattern"]
	if !ok {
		pattern = ""
	}
	content, ok := args["content"]
	if !ok {
		content = ""
	}
	// 使用find和grep命令实现搜索
	// 基础find命令：从当前目录开始，排除.git目录，只搜索文件
	script := `find . -type f -not -path "./.git/*"`

	// 添加文件名模式匹配
	if pattern != "" {
		// 将Go的glob模式转换为find的-name模式
		// 注意：这里简化处理，复杂的glob模式可能需要转换
		// 转义单引号：将'替换为'\''
		escapedPattern := strings.ReplaceAll(pattern, "'", "'\"'\"'")
		script += fmt.Sprintf(` -name '%s'`, escapedPattern)
	}

	// 添加内容匹配
	if content != "" {
		// 使用-exec和grep进行内容搜索
		// -l: 只显示包含匹配内容的文件名
		// -q: 安静模式，只返回退出状态
		// 转义单引号：将'替换为'\''
		escapedContent := strings.ReplaceAll(content, "'", "'\"'\"'")
		script += fmt.Sprintf(` -exec grep -lq '%s' {} \;`, escapedContent)
	}

	// 输出结果并限制数量
	script += ` -print 2>/dev/null | head -50`

	// 处理空结果
	script += ` || echo "未找到匹配的文件"`

	return runBash(ctx, script)
}

// gitCommand 执行git命令（直接使用exec.Command）
func gitCommand(ctx context.Context, args ...string) (string, error) {
	// 检查context是否已经取消
	if ctx.Err() != nil {
		return "", fmt.Errorf("the context has been cancelled: %w", ctx.Err())
	}

	// 创建命令
	cmd := exec.Command("git", args...)
	cmd.Dir = ProjectRoot

	// 设置输出缓冲区
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	// 启动命令
	startTime := time.Now()
	err := cmd.Start()
	if err != nil {
		return "", fmt.Errorf("failed to start git command: %w", err)
	}

	// 创建完成通道
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	// 等待命令完成或context取消
	select {
	case <-ctx.Done():
		// context被取消或超时，终止进程
		cmd.Process.Kill()
		<-done // 等待进程完全终止
		err = ctx.Err()
		if err == context.DeadlineExceeded {
			return stderrBuf.String(), fmt.Errorf("git命令执行超时: %w", err)
		}
		return stderrBuf.String(), fmt.Errorf("git命令被取消: %w", err)
	case err = <-done:
		// 命令执行完成
		stdout := stdoutBuf.String()
		stderr := stderrBuf.String()

		if err != nil {
			// 命令执行失败
			errorMsg := stderr
			if errorMsg == "" {
				errorMsg = err.Error()
			}
			return stdout, fmt.Errorf("failed to execute git command: %s", errorMsg)
		}

		// 命令执行成功
		executionTime := time.Since(startTime)
		if stdout == "" && stderr == "" {
			stdout = "命令执行成功（无输出）"
		}

		// 构建包含执行统计的结果
		result := fmt.Sprintf(`📝 执行结果:
%s

📊 执行统计:
执行时间: %v
状态: 成功`, stdout, executionTime)

		return result, nil
	}
}

// handleGitAdd git添加
func handleGitAdd(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok {
		path = ""
	}
	path = strings.TrimSpace(path)
	names := strings.Fields(path)
	gitArgs := []string{"add"}
	gitArgs = append(gitArgs, names...)
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = fmt.Sprintf("(%s)已添加到暂存区", strings.Join(names, " "))
	}
	return out, nil
}

// handleGitCommit git提交
func handleGitCommit(ctx context.Context, args map[string]string) (string, error) {
	message, ok := args["message"]
	if !ok {
		return "", fmt.Errorf("no message specified")
	}

	options, ok := args["options"]
	if !ok {
		options = ""
	}

	options = strings.TrimSpace(options)

	// 更健壮的-m参数检查
	// 检查 -m、-m[空格]、--message 等变体
	optionWords := strings.FieldsSeq(options)
	for word := range optionWords {
		if word == "-m" || word == "--message" || strings.HasPrefix(word, "-m") {
			return "", fmt.Errorf("message参数已通过message字段提供，不要在options中包含-m或--message")
		}
	}

	gitArgs := []string{"commit", "-m", message}
	if options != "" {
		gitArgs = append(gitArgs, strings.Fields(options)...)
	}

	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "Git has commited"
	}
	return out, nil
}

// handleGitLog git日志
func handleGitLog(ctx context.Context, args map[string]string) (string, error) {
	maxCountRaw, ok := args["max_count"]
	if !ok || maxCountRaw == "" {
		maxCountRaw = "10"
	}
	_, err := strconv.Atoi(maxCountRaw)
	if err != nil {
		err = fmt.Errorf("%q is not integer in string: %w", maxCountRaw, err)
		return "", err
	}

	out, err := gitCommand(ctx, "log", "-n", maxCountRaw, "--oneline")
	if err != nil {
		return "", err
	}

	if out == "" {
		out = "git log succeed without output"
	}
	return out, nil
}

// handleGitDiff git差异
func handleGitDiff(ctx context.Context, args map[string]string) (string, error) {
	path, ok := args["path"]
	if !ok {
		path = ""
	}
	path = strings.TrimSpace(path)
	gitArgs := []string{"diff"}
	if path != "" {
		names := strings.Fields(path)
		gitArgs = append(gitArgs, "HEAD", "--")
		gitArgs = append(gitArgs, names...)
	}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
}

// handleGitStatus git状态
func handleGitStatus(ctx context.Context, args map[string]string) (string, error) {
	out, err := gitCommand(ctx, "status", "--short")
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "工作区干净，无变更"
	}
	return out, nil
}

// handleGitPush git push [options...]
func handleGitPush(ctx context.Context, args map[string]string) (string, error) {
	options, ok := args["options"]
	if !ok {
		options = ""
	}
	options = strings.TrimSpace(options)
	names := strings.Fields(options)
	gitArgs := []string{"push"}
	if options != "" {
		gitArgs = append(gitArgs, names...)
	}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
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

// handleExecuteScript 执行脚本（支持多种解释器，通过shebang指定）
func handleExecuteScript(ctx context.Context, args map[string]string) (out string, err error) {
	script, ok := args["script"]
	if !ok {
		script = ""
	}
	out, err = runBash(ctx, script)
	return
}

func runScript(ctx context.Context, script string, name string, arg []string) (out string, err error) {
	Notice("执行脚本: %s", ShortenScript(script))
	return shellExec(script, name, arg)
}

func ShortenScript(script string) string {
	// 处理空字符串
	if script == "" {
		return ""
	}

	// 跳过 shebang 行
	if strings.HasPrefix(script, "#!") {
		// 找到第一个换行符
		if idx := strings.Index(script, "\n"); idx != -1 {
			script = strings.TrimSpace(script[idx+1:])
		} else {
			// 如果只有 shebang 没有内容
			return ""
		}
	}

	// 处理长度
	r := []rune(script)
	n := len(r)
	if n > 72 {
		first := strings.Fields(string(r[0:36]))
		last := strings.Fields(string(r[n-36 : n]))
		return strings.Join(first, " ") + ".." + strings.Join(last, " ")
	}
	return script
}

func ShellExec(script string) (out string, err error) {
	name, arg := Shebang(script)
	out, err = shellExec(script, name, arg)
	return
}

func shellExec(script string, name string, arg []string) (out string, err error) {
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

func runBash(ctx context.Context, script string) (result string, err error) {
	startTime := time.Now()
	name, arg := Shebang(script)

	out, err := runScript(ctx, script, name, arg)
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

// handleSqlite 执行SQLite数据库查询和操作
func handleSqlite(ctx context.Context, args map[string]string) (string, error) {
	script, ok := args["script"]
	if !ok || script == "" {
		return "", fmt.Errorf("sql script can not be empty")
	}
	// 构建完整的shebang脚本
	fullScript := fmt.Sprintf("#!/usr/bin/env sqlite3 %s\n%s", DBPath, script)

	// 使用现有的runBash执行
	return runBash(ctx, fullScript)
}

// handleDscliChatReload 重新加载运行 dscli chat --reload
func handleDscliChatReload(ctx context.Context, args map[string]string) (result string, err error) {
	// 检查是否是重载命令
	isReload := false
	if v, ok := ctx.Value(IsReload).(bool); ok {
		isReload = v
	}

	// 检查 confirm 参数
	confirm, ok := args["confirm"]
	if !ok || confirm != "yes" {
		return "", fmt.Errorf("必须提供 confirm=yes 参数来确认重载")
	}

	if isReload {
		// 如果是重载进程，返回简单确认信息
		result = "重载进程已启动"
		return
	}

	Info("🔄 检测到重载命令，正在重启进程...")

	// 获取命令行参数
	var cmdArgs []string
	if v, ok := ctx.Value(CommandLineArgs).([]string); ok && len(v) > 0 {
		// 使用原始命令行参数
		cmdArgs = make([]string, len(v))
		copy(cmdArgs, v)

		// 确保有 --reload 标志
		hasReload := false
		for _, arg := range cmdArgs {
			if arg == "--reload" {
				hasReload = true
				break
			}
		}
		if !hasReload {
			cmdArgs = append(cmdArgs, "--reload")
		}
	} else {
		// 如果没有命令行参数，使用默认参数
		cmdArgs = []string{"chat", "--reload"}
	}

	// 构建 exec 命令 - 使用绝对路径避免递归
	dscliPath, err := exec.LookPath("dscli")
	if err != nil {
		Error("找不到 dscli 命令: %v", err)
		return "", fmt.Errorf("找不到 dscli 命令: %v", err)
	}

	// 使用绝对路径执行，避免递归
	cmd := exec.Command(dscliPath, cmdArgs...)
	cmd.Dir = ProjectRoot
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// 执行 exec（这会替换当前进程）
	if err = cmd.Run(); err != nil {
		Error("重载失败: %v", err)
		// 如果 exec 失败，返回错误信息
		err = fmt.Errorf("重载失败: %v", err)
		return "", err
	} else {
		// exec 成功，进程已被替换，这里不会执行
		os.Exit(0)
	}
	return
}

func init() {
	// 注册文件操作工具
	RegisterTool(ToolDef{
		Name:        "read_file",
		Description: "读取项目内指定文件的内容",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleReadFile,
	})

	RegisterTool(ToolDef{
		Name:        "write_file",
		Description: "将内容写入文件（覆盖或新建）",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径，如main.go",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要写入的内容",
				},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleWriteFile,
	})

	RegisterTool(ToolDef{
		Name:        "search_files",
		Description: "在项目中搜索文件（按文件名模式或文件内容）",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "文件名模式，如 '*.go'，为空则匹配所有文件",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "要搜索的内容（如果提供则搜索文件内容）",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleSearchFiles,
	})

	// 注册Git操作工具
	RegisterTool(ToolDef{
		Name:        "git_add",
		Description: "将文件添加到 Git 暂存区",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录），多个文件用空格分隔",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitAdd,
	})

	RegisterTool(ToolDef{
		Name:        "git_commit",
		Description: "提交暂存区更改",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"message": map[string]any{
					"type":        "string",
					"description": "提交信息",
				},
				"options": map[string]any{
					"type": "string",
					"description": `其他git commit选项，例如：-a（提交所有更改）、
--amend（修改上次提交）、--no-edit（使用原提交信息）、
--allow-empty（允许空提交）。
多个选项用空格分隔，例如：-a --amend --no-edit`,
				},
			},
			"required":             []string{"message"},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitCommit,
	})

	RegisterTool(ToolDef{
		Name:        "git_log",
		Description: "查看提交历史",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"max_count": map[string]any{
					"type":        "string",
					"description": `最大显示数量，默认"10"`,
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitLog,
	})

	RegisterTool(ToolDef{
		Name:        "git_diff",
		Description: "查看文件或暂存区的差异",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "文件路径（相对于项目根目录），多个文件用空格分隔",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitDiff,
	})

	RegisterTool(ToolDef{
		Name:        "git_status",
		Description: "查看 Git 仓库状态",
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitStatus,
	})

	RegisterTool(ToolDef{
		Name:        "git_push",
		Description: "推送 Git 分支到远程",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"options": map[string]any{
					"type":        "string",
					"description": "选项，例如：--force-with-lease，多个选项用空格分隔，例如：origin main --force，可为空",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		},
		Category: "git",
		Handler:  handleGitPush,
	})

	// 注册脚本执行工具
	RegisterTool(ToolDef{
		Name: "execute_script",
		Description: `在项目根目录执行脚本。
支持shebang指定解释器（如bash、python等）。
脚本通过标准输入传递，避免命令行长度限制。

输出格式：
- 成功时：返回包含执行结果和执行统计的格式化文本
- 失败时：返回包含错误信息、输出内容和执行统计的格式化文本

示例：
1. Bash脚本：echo "Hello"
2. Python脚本：
#!/usr/bin/env python
print("Hello")
3. 文件操作：cat file.txt
4. Git操作：git status

注意：谨慎使用，避免破坏性操作。确保脚本在项目目录内执行。`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type": "string",
					"description": `要执行的脚本内容。
支持shebang指定解释器（如#!/usr/bin/env bash, #!/usr/bin/env python）。
脚本执行结果会以格式化文本返回，包含执行统计信息。

示例：
1. Bash脚本：echo "Hello"
2. Python脚本：
#!/usr/bin/env python
print("Hello")
3. 文件操作：cat file.txt
4. Git操作：git status
`,
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		},
		Category: "system",
		Handler:  handleExecuteScript,
	})

	// 注册SQLite数据库工具
	RegisterTool(ToolDef{
		Name:        "sqlite",
		Description: "执行SQLite数据库查询和操作。脚本内容为SQL语句。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type": "string",
					"description": `sqlite SQL脚本内容。
例如：
1. .schema messages      Show the CREATE statements matching PATTERN
2. select id, role from messages where id > 1000 order by created_at desc;`,
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		},
		Category: "database",
		Handler:  handleSqlite,
	})

	// 注册SQLite数据库工具
	RegisterTool(ToolDef{
		Name:        "dscli_chat_reload",
		Description: `perform dscli chat reload.`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"confirm": map[string]any{
					"type":        "string",
					"description": `must be ` + "`yes`",
				},
			},
			"required":             []string{"confirm"},
			"additionalProperties": false,
		},
		Category: "system",
		Handler:  handleDscliChatReload,
	})
}

// HandleToolCall 处理工具调用（带统计和超时）
func HandleToolCall(ctx context.Context, toolName string, argsRaw json.RawMessage) (string, error) {
	// 获取工具处理器
	tool, ok := toolRegistry[toolName]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", toolName)
	}
	args := map[string]string{}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		n := len(argsRaw)
		if n > 80 {
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about raw argument tool %q received`+
				` which lead error:
- the length of the argument string: %d
- the last 40 bytes of the argument string: %q
- the first 40 bytes of the argument string: %q`, err, toolName, n,
				string(argsRaw[n-40:]), string(argsRaw[0:40]))
		} else {
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about the raw argument tool %q received, 
which lead to the error:
- the length of the argument string：%d
- the argument raw：%q`, err, toolName, n, string(argsRaw))
		}
		return "", err
	}

	// 创建带超时的context（如果工具设置了超时）
	var cancel context.CancelFunc
	if tool.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, tool.Timeout)
		defer cancel()
	}

	ctx = context.WithValue(ctx, ToolDisplayName, tool.DisplayName)
	toolID, err := GetOrCreateTool(tool.Name, tool.Description, tool.Category)
	if err != nil {
		Error(err.Error(), "name", tool.Name)
		// 继续执行工具，但不记录统计
		return tool.Handler(ctx, args)
	}

	// 执行工具
	result, err := tool.Handler(ctx, args)

	// 检查是否超时
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("工具执行超时（%v）", tool.Timeout)
	}

	// 记录使用情况
	success := err == nil
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	if err := RecordToolUsage(toolID, success, errorMsg); err != nil {
		return "", err
	}

	return result, err
}
