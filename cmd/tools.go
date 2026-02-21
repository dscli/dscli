package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/nanjunjie/dscli/internal/api"
	"gitcode.com/nanjunjie/dscli/internal/db"
)

// ToolDef 工具定义
type ToolDef struct {
	Name        string
	Description string
	Category    string
	Handler     func(projectRoot string, args json.RawMessage) (string, error)
}

// toolRegistry 工具注册表
var toolRegistry = map[string]ToolDef{}

func init() {
	rand.Seed(time.Now().Unix())
}

// RegisterTool 注册工具
func RegisterTool(tool ToolDef) {
	toolRegistry[tool.Name] = tool
}

// GetAllTools 获取所有工具定义（用于API调用）
func GetAllTools() []api.Tool {
	var tools []api.Tool
	for _, tool := range toolRegistry {
		tools = append(tools, api.Tool{
			Type: "function",
			Function: api.ToolFunction{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  getToolParameters(tool.Name),
			},
		})
	}
	return tools
}

// getToolParameters 获取工具参数定义

// getToolParameters 获取工具参数定义（strict模式）
func getToolParameters(toolName string) map[string]interface{} {
	switch toolName {
	case "read_file":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径（相对于项目根目录或绝对路径）",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		}

	case "write_file":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径（相对于项目根目录或绝对路径）",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "要写入的内容",
				},
			},
			"required":             []string{"path", "content"},
			"additionalProperties": false,
		}

	case "search_files":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "文件名模式，如 '*.go'，为空则匹配所有文件",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "要搜索的内容（如果提供则搜索文件内容）",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		}

	case "git_add":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "文件路径（相对于项目根目录）",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		}

	case "git_commit":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"message": map[string]interface{}{
					"type":        "string",
					"description": "提交信息",
				},
			},
			"required":             []string{"message"},
			"additionalProperties": false,
		}

	case "git_log":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"max_count": map[string]interface{}{
					"type":        "integer",
					"description": "最大显示数量，默认10",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		}

	case "git_diff":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "指定文件路径，不指定则查看所有变更",
				},
			},
			"required":             []string{},
			"additionalProperties": false,
		}

	case "git_status":
		return map[string]interface{}{
			"type":                 "object",
			"properties":           map[string]interface{}{},
			"required":             []string{},
			"additionalProperties": false,
		}

	case "execute_script":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"script": map[string]interface{}{
					"type":        "string",
					"description": "要执行的脚本内容。支持shebang指定解释器（如#!/usr/bin/env bash, #!/usr/bin/env python）。脚本执行结果会以格式化文本返回，包含执行统计信息。示例：\n1. Bash脚本：echo \"Hello\"\n2. Python脚本：#!/usr/bin/env python\nprint(\"Hello\")\n3. 文件操作：cat file.txt\n4. Git操作：git status",
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		}

	case "manage_skills":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"action": map[string]interface{}{
					"type":        "string",
					"description": "操作类型：list, enable, disable, create, delete, search",
					"enum":        []string{"list", "enable", "disable", "create", "delete", "search"},
				},
				"skill_name": map[string]interface{}{
					"type":        "string",
					"description": "技能名称",
				},
				"skill_id": map[string]interface{}{
					"type":        "integer",
					"description": "技能ID",
				},
				"category": map[string]interface{}{
					"type":        "string",
					"description": "技能分类过滤",
				},
				"search_term": map[string]interface{}{
					"type":        "string",
					"description": "搜索关键词",
				},
				"description": map[string]interface{}{
					"type":        "string",
					"description": "技能描述",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "技能内容/规则",
				},
				"priority": map[string]interface{}{
					"type":        "integer",
					"description": "技能优先级",
				},
			},
			"required":             []string{"action"},
			"additionalProperties": false,
		}

	default:
		// 默认返回空参数定义
		return map[string]interface{}{
			"type":                 "object",
			"properties":           map[string]interface{}{},
			"required":             []string{},
			"additionalProperties": false,
		}
	}
}

// HandleToolCall 处理工具调用（带统计）
func HandleToolCall(database *db.DB, toolName string, projectRoot string, args json.RawMessage) (string, error) {
	// 获取工具处理器
	tool, ok := toolRegistry[toolName]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", toolName)
	}

	toolID, err := database.GetOrCreateTool(tool.Name, tool.Description, tool.Category)
	if err != nil {
		// 继续执行工具，但不记录统计
		return tool.Handler(projectRoot, args)
	}

	// 执行工具
	result, err := tool.Handler(projectRoot, args)

	// 记录使用情况
	success := err == nil
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	projectHash := db.GetProjectHash(projectRoot)
	if err := database.RecordToolUsage(toolID, projectHash, success, errorMsg); err != nil {
		log.Printf("记录工具使用失败: %v", err)
	}

	return result, err
}

func HandleToolCalls(database *db.DB, projectRoot string, sessionID int64, assistantMsg *api.Message) (err error) {
	inputs := []api.Message{}
	// 处理每个工具调用
	for _, tc := range assistantMsg.ToolCalls {
		// 使用新的工具调用处理器
		result, err := HandleToolCall(database, tc.Function.Name, projectRoot, []byte(tc.Function.Arguments))
		if err != nil {
			// But we still need to tell the result to assistant
			result = err.Error()
		}

		inputs = append(inputs, api.Message{
			Role:       "tool",
			ToolCallID: tc.ID,
			Content:    result,
		})
	}

	if len(inputs) > 0 {
		err = ChatMessage(database, projectRoot, sessionID, inputs...)
	}
	return
}

// ==================== 工具处理器实现 ====================

// 解析文件路径：如果是相对路径，则拼接项目根目录；否则直接使用（需确保在项目内）
func resolvePath(projectRoot, path string) (string, error) {
	if filepath.IsAbs(path) {
		// 检查是否在项目根目录内
		rel, err := filepath.Rel(projectRoot, path)
		if err != nil || strings.HasPrefix(rel, "..") {
			return "", fmt.Errorf("路径 %q 不在项目根目录内", path)
		}
		return path, nil
	}
	return filepath.Join(projectRoot, path), nil
}

// handleReadFile 读取文件
func handleReadFile(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		log.Printf("argsRaw: %s", string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	fullPath, err := resolvePath(projectRoot, args.Path)
	if err != nil {
		return "", err
	}
	return runBash(projectRoot, fmt.Sprintf(`cat "%s"`, fullPath))
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
func handleWriteFile(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		log.Printf("argsRaw: %s", string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}

	fullPath, err := resolvePath(projectRoot, args.Path)
	if err != nil {
		return "", err
	}

	dsctmpeof := "DSCTMPEOF"
	content := args.Content
	for strings.Contains(content, dsctmpeof) {
		dsctmpeof = Shuffle(dsctmpeof)
	}
	script := fmt.Sprintf(`mkdir -p "%s"
cat > %s <<'%s'
%s
%s
echo 已成功写入文件: "%s"
`, filepath.Dir(fullPath), fullPath, dsctmpeof, content, dsctmpeof, args.Path)
	return runBash(projectRoot, script)
}

// handleSearchFiles 搜索文件
func handleSearchFiles(projectRoot string, argsRaw json.RawMessage) (string, error) {
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

	return runBash(projectRoot, script)
}

// gitCommand 执行git命令
func gitCommand(projectRoot string, args ...string) (string, error) {
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
	return runBash(projectRoot, cmdStr)
}

// handleGitAdd git添加
func handleGitAdd(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	out, err := gitCommand(projectRoot, "add", args.Path)
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "已添加到暂存区"
	}
	return out, nil
}

// handleGitCommit git提交
func handleGitCommit(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Message string `json:"message"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	out, err := gitCommand(projectRoot, "commit", "-m", args.Message)
	if err != nil {
		return "", err
	}
	return out, nil
}

// handleGitLog git日志
func handleGitLog(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		MaxCount int `json:"max_count"`
	}

	if err := json.Unmarshal(argsRaw, &args); err != nil {
		args.MaxCount = 0
	}

	if args.MaxCount <= 0 {
		args.MaxCount = 10
	}
	out, err := gitCommand(projectRoot, "log", "-n", fmt.Sprintf("%d", args.MaxCount), "--oneline")
	if err != nil {
		return "", err
	}
	return out, nil
}

// handleGitDiff git差异
func handleGitDiff(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	_ = json.Unmarshal(argsRaw, &args) // 忽略错误，path 可选
	gitArgs := []string{"diff"}
	if args.Path != "" {
		gitArgs = append(gitArgs, "--", args.Path)
	}
	out, err := gitCommand(projectRoot, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
}

// handleGitStatus git状态
func handleGitStatus(projectRoot string, argsRaw json.RawMessage) (string, error) {
	out, err := gitCommand(projectRoot, "status", "--short")
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "工作区干净，无变更"
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

// handleBash 执行Bash脚本（使用echo script | bash方式）
func handleBash(projectRoot string, argsRaw json.RawMessage) (out string, err error) {
	var args struct {
		Script string `json:"script"`
	}
	if err = json.Unmarshal(argsRaw, &args); err != nil {
		err = fmt.Errorf("参数解析失败: %w", err)
		log.Printf("%v", err)
		return
	}

	out, err = runBash(projectRoot, args.Script)
	return
}

func runScriptShebang(projectRoot string, script string, name string, arg []string) (out string, err error) {
	log.Printf("执行脚本: %s %s %v", script, name, arg)
	buf := bytes.NewBuffer([]byte{})
	fmt.Printf("执行脚本：\n#+begin_src %s\n%s\n#+end_src\n", path.Base(name), script)
	subproc := exec.Command(name, arg...)
	subproc.Dir = projectRoot
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

func runBash(projectRoot string, script string) (result string, err error) {
	log.Printf("执行脚本: %s", script)
	startTime := time.Now()
	name, arg := Shebang(script)

	out, err := runScriptShebang(projectRoot, script, name, arg)
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
		fmt.Printf("\n#+begin_example\n%s\n#+end_example\n", result)
		return result, nil
	}

	// 构建包含执行统计的成功结果
	result = fmt.Sprintf(`=== 执行结果 ===
%s

=== 执行统计 ===
执行时间: %v
状态: 成功`,
		out, executionTime)
	fmt.Printf("\n#+begin_example\n%s\n#+end_example\n", result)

	return
}

func handleManageSkills(projectRoot string, argsRaw json.RawMessage) (string, error) {
	log.Printf("manage_skills called for project: %s", projectRoot)
	log.Printf("args: %s", string(argsRaw))
	return "Skills management is under development", nil
}

// InitTools 初始化工具系统
func InitTools() {
	// 注册文件操作工具
	RegisterTool(ToolDef{
		Name:        "read_file",
		Description: "读取项目内指定文件的内容",
		Category:    "file_ops",
		Handler:     handleReadFile,
	})

	RegisterTool(ToolDef{
		Name:        "write_file",
		Description: "将内容写入文件（覆盖或新建）",
		Category:    "file_ops",
		Handler:     handleWriteFile,
	})

	RegisterTool(ToolDef{
		Name:        "search_files",
		Description: "在项目中搜索文件（按文件名模式或文件内容）",
		Category:    "file_ops",
		Handler:     handleSearchFiles,
	})

	// 注册Git操作工具
	RegisterTool(ToolDef{
		Name:        "git_add",
		Description: "将文件添加到 Git 暂存区",
		Category:    "git",
		Handler:     handleGitAdd,
	})

	RegisterTool(ToolDef{
		Name:        "git_commit",
		Description: "提交暂存区更改",
		Category:    "git",
		Handler:     handleGitCommit,
	})

	RegisterTool(ToolDef{
		Name:        "git_log",
		Description: "查看提交历史",
		Category:    "git",
		Handler:     handleGitLog,
	})

	RegisterTool(ToolDef{
		Name:        "git_diff",
		Description: "查看文件或暂存区的差异",
		Category:    "git",
		Handler:     handleGitDiff,
	})

	RegisterTool(ToolDef{
		Name:        "git_status",
		Description: "查看 Git 仓库状态",
		Category:    "git",
		Handler:     handleGitStatus,
	})

	// 注册Bash脚本工具
	RegisterTool(ToolDef{
		Name:        "execute_script",
		Description: "在项目根目录执行脚本。支持shebang指定解释器（如bash、python等）。脚本通过标准输入传递，避免命令行长度限制。\n\n输出格式：\n- 成功时：返回包含执行结果和执行统计的格式化文本\n- 失败时：返回包含错误信息、输出内容和执行统计的格式化文本\n\n示例：\n1. Bash脚本：echo \"Hello\"\n2. Python脚本：#!/usr/bin/env python\nprint(\"Hello\")\n3. 文件操作：cat file.txt\n4. Git操作：git status\n\n注意：谨慎使用，避免破坏性操作。确保脚本在项目目录内执行。",
		Category:    "system",
		Handler:     handleBash,
	})

	// 注册技能管理工具
	RegisterTool(ToolDef{
		Name:        "manage_skills",
		Description: "管理项目的技能（最佳实践规则）",
		Category:    "skills",
		Handler:     handleManageSkills,
	})

	log.Printf("工具系统初始化完成，共注册 %d 个工具", len(toolRegistry))
}
