package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"math/rand"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"
)

var ToolDisplayName = &struct{}{}

// toolRegistry 工具注册表
var toolRegistry = map[string]ToolDef{}

// RegisterTool 注册工具
func RegisterTool(tool ToolDef) {
	displayName := func() string {
		name := tool.Name
		words := strings.Split(name, "_")
		for i, word := range words {
			word = strings.ToUpper(word[0:1]) + word[1:]
			words[i] = word
		}
		return strings.Join(words, "")
	}
	tool.DisplayName = displayName()
	toolRegistry[tool.Name] = tool
}

// GetAllTools 获取所有工具定义（用于API调用）
func GetAllTools() []Tool {
	if ModelID == DEEPSEEK_REASONER {
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

// HandleToolCall 处理工具调用（带统计）
func HandleToolCall(ctx context.Context, toolName string, args json.RawMessage) (string, error) {
	// 获取工具处理器
	tool, ok := toolRegistry[toolName]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", toolName)
	}
	ctx = context.WithValue(ctx, ToolDisplayName, tool.DisplayName)
	toolID, err := GetOrCreateTool(tool.Name, tool.Description, tool.Category)
	if err != nil {
		slog.Error(err.Error(), "name", tool.Name)
		// 继续执行工具，但不记录统计
		return tool.Handler(ctx, args)
	}

	// 执行工具
	result, err := tool.Handler(ctx, args)

	// 记录使用情况
	success := err == nil
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	if err := RecordToolUsage(toolID, success, errorMsg); err != nil {
		log.Printf("记录工具使用失败: %v", err)
	}

	return result, err
}

func HandleToolCalls(ctx context.Context, assistantMsg *Message) (err error) {
	inputs := []Message{}
	// 处理每个工具调用
	for _, tc := range assistantMsg.ToolCalls {
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

	if len(inputs) > 0 {
		err = ChatMessage(ctx, inputs...)
	}
	return
}

// ==================== 工具处理器实现 ====================

// 解析文件路径：如果是相对路径，则拼接项目根目录；否则直接使用（需确保在项目内）
func resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		// 检查是否在项目根目录内
		rel, err := filepath.Rel(ProjectRoot, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("路径 %q 不在项目根目录内", path)
		}
		return path, nil
	}
	return filepath.Join(ProjectRoot, path), nil
}

// handleReadFile 读取文件
func handleReadFile(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		log.Printf("argsRaw: %s", string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	fullPath, err := resolvePath(args.Path)
	if err != nil {
		return "", err
	}
	return runBash(ctx, fmt.Sprintf(`cat "%s"`, fullPath))
}

func Shuffle(in string) (out string) {
	runes := []rune(in)
	rand.Shuffle(len(runes), func(i, j int) {
		runes[i], runes[j] = runes[j], runes[i]
	})
	out = string(runes)
	return
}

// handleWriteFile 写入文件
func handleWriteFile(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		log.Printf("argsRaw: %s", string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	fullPath, err := resolvePath(args.Path)
	if err != nil {
		return "", err
	}

	dsctmpeof := "DSCTMPEOF"
	content := args.Content
	for strings.Contains(content, dsctmpeof) {
		dsctmpeof = Shuffle(dsctmpeof)
	}
	dir := filepath.Dir(fullPath)
	script := `test -d "` + dir + `" || mkdir -p -m 0755 "` + dir + `"
cat > ` + fullPath + ` <<'` + dsctmpeof + `'
` + content + `
` + dsctmpeof + `
echo 已成功写入文件: "` + args.Path + `"
`
	return runBash(ctx, script)
}

// handleSearchFiles 搜索文件
func handleSearchFiles(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Pattern string `json:"pattern"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	// 使用find和grep命令实现搜索
	// 基础find命令：从当前目录开始，排除.git目录，只搜索文件
	script := `find . -type f -not -path "./.git/*"`

	// 添加文件名模式匹配
	if args.Pattern != "" {
		// 将Go的glob模式转换为find的-name模式
		// 注意：这里简化处理，复杂的glob模式可能需要转换
		pattern := args.Pattern
		// 转义单引号：将'替换为'\''
		escapedPattern := strings.ReplaceAll(pattern, "'", "'\"'\"'")
		script += fmt.Sprintf(` -name '%s'`, escapedPattern)
	}

	// 添加内容匹配
	if args.Content != "" {
		// 使用-exec和grep进行内容搜索
		// -l: 只显示包含匹配内容的文件名
		// -q: 安静模式，只返回退出状态
		// 转义单引号：将'替换为'\''
		escapedContent := strings.ReplaceAll(args.Content, "'", "'\"'\"'")
		script += fmt.Sprintf(` -exec grep -lq '%s' {} \\;`, escapedContent)
	}

	// 输出结果并限制数量
	script += ` -print 2>/dev/null | head -50`

	// 处理空结果
	script += ` || echo "未找到匹配的文件"`

	return runBash(ctx, script)
}

// gitCommand 执行git命令
func gitCommand(ctx context.Context, args ...string) (string, error) {
	// 手动构建git命令字符串，正确处理参数中的空格
	cmdStr := "git"
	for _, arg := range args {
		// 如果参数包含空格或特殊字符，需要加引号
		if strings.ContainsAny(arg, " \t\n\"'") {
			// 转义单引号：将'替换为'\''
			arg = strings.ReplaceAll(arg, "'", "'\"'\"'")
			cmdStr += fmt.Sprintf(" '%s'", arg)
		} else {
			cmdStr += " " + arg
		}
	}
	return runBash(ctx, cmdStr)
}

// handleGitAdd git添加
func handleGitAdd(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	gitArgs := []string{"add"}
	gitArgs = append(gitArgs, strings.Fields(args.Path)...)
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "已添加到暂存区"
	}
	return out, nil
}

// handleGitCommit git提交
func handleGitCommit(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Message string `json:"message"`
		Options string `json:"options,omitempty"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	options := strings.TrimSpace(args.Options)

	// 更健壮的-m参数检查
	// 检查 -m、-m[空格]、--message 等变体
	optionWords := strings.Fields(options)
	for _, word := range optionWords {
		if word == "-m" || word == "--message" || strings.HasPrefix(word, "-m") {
			return "", fmt.Errorf("message参数已通过message字段提供，不要在options中包含-m或--message")
		}
	}

	gitArgs := []string{"commit", "-m", args.Message}
	if options != "" {
		gitArgs = append(gitArgs, strings.Fields(options)...)
	}
	return gitCommand(ctx, gitArgs...)
}

// handleGitLog git日志
func handleGitLog(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		MaxCount int `json:"max_count"`
	}

	if err := json.Unmarshal(argsRaw, &args); err != nil {
		args.MaxCount = 0
	}

	if args.MaxCount <= 0 {
		args.MaxCount = 10
	}
	out, err := gitCommand(ctx, "log", "-n", fmt.Sprintf("%d", args.MaxCount), "--oneline")
	if err != nil {
		return "", err
	}
	return out, nil
}

// handleGitDiff git差异
func handleGitDiff(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(argsRaw, &args) // 忽略错误，path 可选
	gitArgs := []string{"diff"}
	if args.Path != "" {
		gitArgs = append(gitArgs, "HEAD", "--")
		gitArgs = append(gitArgs, strings.Fields(args.Path)...)
	}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
}

// handleGitStatus git状态
func handleGitStatus(ctx context.Context, argsRaw json.RawMessage) (string, error) {
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
func handleGitPush(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Options string `json:"options,omitempty"`
	}
	// 直接解析，json.Unmarshal能处理空JSON
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	gitArgs := []string{"push"}
	if args.Options != "" {
		gitArgs = append(gitArgs, strings.Fields(args.Options)...)
	}
	out, err := gitCommand(ctx, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
}

func Shebang(script string) (name string, arg []string) {
	shebang := []string{"/usr/bin/env", "bash"}
	idx := strings.Index(script, "\n")
	if idx != -1 {
		line1 := script[0:idx]
		if strings.HasPrefix(line1, "#!") {
			shebang = strings.Fields(line1[2:])
		}
	}
	name = shebang[0]
	arg = shebang[1:]
	return
}

// handleExecuteScript 执行脚本（支持多种解释器，通过shebang指定）
func handleExecuteScript(ctx context.Context, argsRaw json.RawMessage) (out string, err error) {
	var args struct {
		Script string `json:"script"`
	}
	if err = json.Unmarshal(argsRaw, &args); err != nil {
		err = fmt.Errorf("参数解析失败: %w", err)
		log.Printf("%v", err)
		return
	}

	out, err = runBash(ctx, args.Script)
	return
}

func runScript(ctx context.Context, script string, name string, arg []string) (out string, err error) {
	toolName := "unknown"
	if val := ctx.Value(ToolDisplayName); val != nil {
		if nameStr, ok := val.(string); ok {
			toolName = nameStr
		}
	}
	startTime := time.Now()
	log.Printf("执行脚本（%s）: %s %s %v", toolName, script, name, arg)
	lang := path.Base(name)
	if len(arg) > 0 {
		lang = arg[0]
	}
	fmt.Printf("执行脚本（%s）：\n```%s\n%s\n```\n", toolName, lang, script)
	defer func() {
		spend := time.Since(startTime)
		if err == nil {
			fmt.Printf("\n执行成功（%v）：\n```\n%s\n```\n",
				spend, out)
		} else {
			fmt.Printf("\n执行失败（%v）：\n```\n%s\n```\n\n出错信息：\n```\n%s\n```\n",
				spend, out, err.Error())
		}
	}()
	return shellExec(script, name, arg)
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
		log.Printf("%v", err)
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
		log.Printf("执行失败: %v", err)
		return out, err
	}
	return out, nil
}

func runBash(ctx context.Context, script string) (result string, err error) {
	log.Printf("执行脚本: %s", script)
	startTime := time.Now()
	name, arg := Shebang(script)

	out, err := runScript(ctx, script, name, arg)
	executionTime := time.Since(startTime)
	if err != nil {
		log.Printf("执行失败: %v", err)
		// 构建包含执行统计的失败结果
		result := fmt.Sprintf(`=== 执行失败 ===
错误: %v

=== 输出内容 ===
%s

=== 执行统计 ===
执行时间: %v
状态: 失败`,
			err, out, executionTime)
		return result, nil
	}

	// 构建包含执行统计的成功结果
	result = fmt.Sprintf(`=== 执行结果 ===
%s

=== 执行统计 ===
执行时间: %v
状态: 成功`,
		out, executionTime)

	return
}

// handleSqlite 执行SQLite数据库查询和操作
func handleSqlite(ctx context.Context, argsRaw json.RawMessage) (string, error) {
	// 解析参数
	var args struct {
		Script string `json:"script"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("解析参数失败: %w", err)
	}

	if args.Script == "" {
		return "", fmt.Errorf("SQL脚本不能为空")
	}

	// 构建完整的shebang脚本
	fullScript := fmt.Sprintf("#!/usr/bin/env sqlite3 %s\n%s", DBPath, args.Script)

	// 使用现有的runBash执行
	return runBash(ctx, fullScript)
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
					"type":        "integer",
					"description": "最大显示数量，默认10",
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
}
