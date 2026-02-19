package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/nanjunjie/dscli/internal/api"
	"gitcode.com/nanjunjie/dscli/internal/db"
	"gitcode.com/nanjunjie/dscli/internal/log"
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

// RegisterTool 注册工具
func RegisterTool(tool ToolDef) {
	toolRegistry[tool.Name] = tool
	log.Info("注册工具: %s (%s)", tool.Name, tool.Category)
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

	case "run_command":
		return map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "要执行的 shell 命令，如 'git log --oneline | head -5'",
				},
			},
			"required":             []string{"command"},
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
func HandleToolCall(toolName string, projectRoot string, args json.RawMessage) (string, error) {
	// 获取工具处理器
	tool, ok := toolRegistry[toolName]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", toolName)
	}

	// 获取或创建工具记录
	database, err := db.New()
	if err != nil {
		log.Error("初始化数据库失败: %v", err)
		// 继续执行工具，但不记录统计
		return tool.Handler(projectRoot, args)
	}
	defer database.Close()

	toolID, err := database.GetOrCreateTool(tool.Name, tool.Description, tool.Category)
	if err != nil {
		log.Error("获取或创建工具记录失败: %v", err)
		// 继续执行工具，但不记录统计
		return tool.Handler(projectRoot, args)
	}

	// 执行工具
	startTime := time.Now()
	result, err := tool.Handler(projectRoot, args)
	duration := time.Since(startTime)

	// 记录使用情况
	success := err == nil
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	projectHash := db.GetProjectHash(projectRoot)
	if err := database.RecordToolUsage(toolID, projectHash, success, errorMsg); err != nil {
		log.Error("记录工具使用失败: %v", err)
	}

	log.Info("工具调用: %s, 耗时: %v, 成功: %v", toolName, duration, success)

	return result, err
}

// GetToolHandler 获取工具处理器（兼容旧代码）
func GetToolHandler(toolName string) func(projectRoot string, args json.RawMessage) (string, error) {
	return func(projectRoot string, args json.RawMessage) (string, error) {
		return HandleToolCall(toolName, projectRoot, args)
	}
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
		log.Debug("argsRaw: %s", string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	fullPath, err := resolvePath(projectRoot, args.Path)
	if err != nil {
		return "", err
	}
	log.FileOperation("读取文件", args.Path)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("读取文件失败: %w", err)
	}
	return string(data), nil
}

// handleWriteFile 写入文件
func handleWriteFile(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		log.Debug("argsRaw: %s", string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	fullPath, err := resolvePath(projectRoot, args.Path)
	if err != nil {
		return "", err
	}
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	log.FileOperation("写入文件", args.Path)
	if err := os.WriteFile(fullPath, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return fmt.Sprintf("已成功写入文件: %s", args.Path), nil
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
	var results []string
	log.Info("搜索文件", "pattern", args.Pattern, "content", args.Content)
	err := filepath.Walk(projectRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过无法访问的路径
		}
		if info.IsDir() {
			// 忽略 .git 目录
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(projectRoot, path)
		if err != nil {
			return nil
		}
		// 文件名匹配
		if args.Pattern != "" {
			matched, _ := filepath.Match(args.Pattern, info.Name())
			if !matched {
				return nil
			}
		}
		// 内容匹配
		if args.Content != "" {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil
			}
			if !strings.Contains(string(data), args.Content) {
				return nil
			}
		}
		results = append(results, rel)
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("搜索失败: %w", err)
	}
	if len(results) == 0 {
		return "未找到匹配的文件", nil
	}
	return strings.Join(results, "\n"), nil
}

// gitCommand 执行git命令
func gitCommand(projectRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git 命令失败: %s\n输出: %s", err, out)
	}
	return string(out), nil
}

// handleGitAdd git添加
func handleGitAdd(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	log.GitOperation("git add", "path", args.Path)
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
	log.GitOperation("git commit", "message", args.Message)
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
	log.GitOperation("git log", "max_count", args.MaxCount)
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
	log.GitOperation("git diff", "path", args.Path)
	out, err := gitCommand(projectRoot, gitArgs...)
	if err != nil {
		return "", err
	}
	return out, nil
}

// handleGitStatus git状态
func handleGitStatus(projectRoot string, argsRaw json.RawMessage) (string, error) {
	log.GitOperation("git status")
	out, err := gitCommand(projectRoot, "status", "--short")
	if err != nil {
		return "", err
	}
	if out == "" {
		out = "工作区干净，无变更"
	}
	return out, nil
}

// handleRunCommand 执行命令
func handleRunCommand(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	if args.Command == "" {
		return "", fmt.Errorf("命令不能为空")
	}

	log.Info("执行命令: %s", args.Command)
	cmd := exec.Command("bash", "-c", args.Command)
	cmd.Dir = projectRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("命令执行失败: %v\n输出:\n%s", err, out), nil
	}
	return string(out), nil
}

// handleManageSkills 管理技能
func handleManageSkills(projectRoot string, argsRaw json.RawMessage) (string, error) {
	log.Info("manage_skills called for project: %s", projectRoot)
	log.Info("args: %s", string(argsRaw))
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

	// 注册系统操作工具
	RegisterTool(ToolDef{
		Name:        "run_command",
		Description: "在项目根目录执行任意 shell 命令（支持管道、组合命令）。谨慎使用，避免破坏性操作。",
		Category:    "system",
		Handler:     handleRunCommand,
	})

	// 注册技能管理工具
	RegisterTool(ToolDef{
		Name:        "manage_skills",
		Description: "管理项目的技能（最佳实践规则）",
		Category:    "skills",
		Handler:     handleManageSkills,
	})

	log.Info("工具系统初始化完成，共注册 %d 个工具", len(toolRegistry))
}
