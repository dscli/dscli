package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/nanjunjie/dscli/internal/api"
	"gitcode.com/nanjunjie/dscli/internal/db"
	"gitcode.com/nanjunjie/dscli/internal/log"
	"github.com/spf13/cobra"
)

var chatModel string

// 工具定义
var tools = []api.Tool{
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "read_file",
			Description: "读取项目内指定文件的内容",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "文件路径（相对于项目根目录或绝对路径）",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "write_file",
			Description: "将内容写入文件（覆盖或新建）",
			Parameters: map[string]interface{}{
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
				"required": []string{"path", "content"},
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "search_files",
			Description: "在项目中搜索文件（按文件名模式或文件内容）",
			Parameters: map[string]interface{}{
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
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "git_add",
			Description: "将文件添加到 Git 暂存区",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "文件路径（相对于项目根目录）",
					},
				},
				"required": []string{"path"},
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "git_commit",
			Description: "提交暂存区更改",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "提交信息",
					},
				},
				"required": []string{"message"},
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "git_log",
			Description: "查看提交历史",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"max_count": map[string]interface{}{
						"type":        "integer",
						"description": "最大显示数量，默认10",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "git_diff",
			Description: "查看文件或暂存区的差异",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "指定文件路径，不指定则查看所有变更",
					},
				},
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "git_status",
			Description: "查看 Git 仓库状态",
			Parameters: map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	},
	{
		Type: "function",
		Function: api.ToolFunction{
			Name:        "run_command",
			Description: "在项目根目录执行任意 shell 命令（支持管道、组合命令）。谨慎使用，避免破坏性操作。",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "要执行的 shell 命令，如 'git log --oneline | head -5'",
					},
				},
				"required": []string{"command"},
			},
		},
	},
}

// 工具执行函数映射
var toolHandlers = map[string]func(projectRoot string, args json.RawMessage) (string, error){
	"read_file":    handleReadFile,
	"write_file":   handleWriteFile,
	"search_files": handleSearchFiles,
	"git_add":      handleGitAdd,
	"git_commit":   handleGitCommit,
	"git_log":      handleGitLog,
	"git_diff":     handleGitDiff,
	"git_status":   handleGitStatus,
	"run_command":  handleRunCommand,
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "与 DeepSeek 对话（支持工具调用：文件操作、Git）",
	Long: `发送一条消息给 DeepSeek 聊天模型并获取回复。
消息内容通过标准输入提供，自动按项目目录隔离对话历史。
支持工具调用：文件读写、搜索、Git 操作。

示例：
  echo "帮我创建一个 main.go 文件" | dscli chat
  echo "把 README.md 添加到 Git 并提交" | dscli chat
  cat prompt.txt | dscli chat --model deepseek-chat`,
	RunE: ChatRunE,
}

func ChatRunE(cmd *cobra.Command, args []string) (err error) {
	log.Info("开始处理聊天请求")
	// 1. 确定项目根路径
	projectRoot, err := getProjectRoot()
	log.Info("项目根目录: %s", projectRoot)
	if err != nil {
		err = fmt.Errorf("无法确定项目根路径: %w", err)
		return
	}

	// 2. 打开数据库
	database, err := db.New()
	log.DatabaseOperation("打开数据库")
	if err != nil {
		err = fmt.Errorf("初始化数据库失败: %w", err)
		return
	}
	defer database.Close()

	// 3. 获取会话ID
	sessionID, err := database.GetOrCreateSession(projectRoot)
	if err != nil {
		err = fmt.Errorf("获取会话失败: %w", err)
		return
	}

	// 4. 读取标准输入
	reader := bufio.NewReader(os.Stdin)
	content, err := io.ReadAll(reader)
	if err != nil {
		return
	}
	userMsg := strings.TrimSpace(string(content))
	log.Info("用户输入长度: %d 字符", len(userMsg))
	if userMsg != "" {
		return ChatMessage(database, projectRoot, sessionID, api.Message{Role: "user", Content: userMsg})
	}

	dm, err := database.LoadLastOne(sessionID)
	if err != nil {
		return
	}

	am := ToApiMessage(dm)
	if am.Role != "assistant" || len(am.ToolCalls) == 0 {
		fmt.Println("天下本无事")
		return
	}
	return HandleToolCalls(database, projectRoot, sessionID, am)
}

func ToApiMessage(dm *db.Message) (am *api.Message) {
	role := dm.Role
	log.Debug("dm role=%s", role);
	am = &api.Message{
		Role:    role,
		Content: dm.Content,
		ToolCallID: dm.ToolCallID,
	}
	log.Debug("am: %s", am)
	if len(dm.ToolCalls) != 0 {
		err := json.Unmarshal(dm.ToolCalls, &am.ToolCalls)
		if err != nil {
			log.Error("error: %v", err)
			return nil
		}
	}
	return
}

func ToDBMessage(apim api.Message) (dbm db.Message) {
	role := apim.Role
	dbm.Content = apim.Content
	dbm.Role = apim.Role
	if role == "tool" {
		dbm.ToolCallID = apim.ToolCallID
	}
	if role == "assistant" && len(apim.ToolCalls) > 0 {
		data, err := json.Marshal(apim.ToolCalls)
		if err != nil {
			log.Error("error: %v", err)
			return
		}
		dbm.ToolCalls = data
	}
	return
}


func HandleToolCalls(database *db.DB, projectRoot string, sessionID int64, assistantMsg *api.Message) (err error) {
	inputs := []api.Message{}
	// 处理每个工具调用
	for _, tc := range assistantMsg.ToolCalls {
		// 执行工具
		handler, ok := toolHandlers[tc.Function.Name]
		if !ok {
			// 未知工具，返回错误信息
			err = fmt.Errorf("错误: 未知工具 '%s'", tc.Function.Name)
			return
		}

		var result string
		result, err = handler(projectRoot, []byte(tc.Function.Arguments))
		if err != nil {
			log.Error("执行%s失败: %v", tc.Function.Name, err)
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

func ChatMessage(database *db.DB, projectRoot string, sessionID int64, inputs ...api.Message) (err error) {
	// 5. 加载历史消息
	history, err := database.LoadHistory(sessionID)
	log.DatabaseOperation("加载历史消息", "sessionID", sessionID, "消息数量", len(history))
	if err != nil {
		err = fmt.Errorf("加载历史消息失败: %w", err)
		return
	}

	// 添加系统提示（包含当前日期）
	currentDate := time.Now().Format("2006-01-02")
	systemMessage := api.Message{
		Role: "system",
		Content: fmt.Sprintf("当前日期：%s\\n你是一个编程助手，可以读写文件、执行Git操作、搜索文件等。请根据用户请求提供帮助。", currentDate),
	}
	// 6. 构造 messages 切片（包含历史）
	messages := make([]api.Message, 0, len(history)+2)
	messages = append(messages, systemMessage)
	for _, m := range history {
		apiMsg := api.Message{
			Role:    m.Role,
			Content: m.Content,
		}
		if m.ToolCallID != "" {
			apiMsg.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			var toolCalls []api.ToolCall
			err = json.Unmarshal(m.ToolCalls, &toolCalls)
			if err != nil {
				err = fmt.Errorf("反序列化ToolCalls失败: %w", err)
				return
			}
			apiMsg.ToolCalls = toolCalls
		}
		messages = append(messages, apiMsg)
	}
	// 添加当前用户消息
	messages = append(messages, inputs...)

	// 7. 记录本轮新增的消息（用于存储）
	var dbmessages []db.Message
	for _, m := range inputs {
		dbmessages = append(dbmessages, ToDBMessage(m))
	}

	var resp *api.ChatResponse // resp
	log.Info("调用大模型API，模型: %s", chatModel)
	resp, err = client.ChatWithTools(chatModel, messages, tools)
	if err != nil {
		err = fmt.Errorf("聊天请求失败: %w", err)
		return
	}

	if len(resp.Choices) == 0 {
		err = fmt.Errorf("错误: 未收到回复")
		return
	}

	assistantMsg := resp.Choices[0].Message

	// 转换并保存到 newMessages（用于后续存储）
	dbAssistantMsg := ToDBMessage(assistantMsg)
	dbmessages = append(dbmessages, dbAssistantMsg)
	if len(dbmessages) > 0 {
		if err = database.SaveMessagesBatch(sessionID, dbmessages); err != nil {
			err = fmt.Errorf("保存消息失败: %w", err)
			return
		}
	}

	fmt.Println(assistantMsg.Content)
	return HandleToolCalls(database, projectRoot, sessionID, &assistantMsg)
}

// 工具处理函数实现 -------------------------------------------------

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

func handleReadFile(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		log.Debug("argsRaw: %s",string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	fullPath, err := resolvePath(projectRoot, args.Path)
	log.FileOperation("写入文件", args.Path)
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

func handleWriteFile(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		log.Debug("argsRaw: %s",string(argsRaw))
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	fullPath, err := resolvePath(projectRoot, args.Path)
	log.FileOperation("写入文件", args.Path)
	if err != nil {
		return "", err
	}
	// 确保目录存在
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	if err := os.WriteFile(fullPath, []byte(args.Content), 0o644); err != nil {
		return "", fmt.Errorf("写入文件失败: %w", err)
	}
	return fmt.Sprintf("已成功写入文件: %s", args.Path), nil
}

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

// Git 操作辅助：在项目根目录执行 git 命令
func gitCommand(projectRoot string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git 命令失败: %s\n输出: %s", err, out)
	}
	return string(out), nil
}

func handleGitAdd(projectRoot string, argsRaw json.RawMessage) (string, error) {
	var args struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		return "", fmt.Errorf("参数解析失败: %w", err)
	}
	// 路径相对于项目根目录
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

	// 使用 bash -c 执行，以支持管道和复合命令
	log.Info("执行命令: %s", args.Command)
	cmd := exec.Command("bash", "-c", args.Command)
	cmd.Dir = projectRoot

	out, err := cmd.CombinedOutput()
	if err != nil {
		// 即使命令执行失败，也返回输出内容，便于模型调试
		return fmt.Sprintf("命令执行失败: %v\n输出:\n%s", err, out), nil
	}
	return string(out), nil
}

// getProjectRoot 获取当前项目根目录（用于会话隔离）
func getProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	gitRoot, err := findGitRoot(cwd)
	if err == nil && gitRoot != "" {
		return gitRoot, nil
	}
	return filepath.Abs(cwd)
}

func findGitRoot(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		gitPath := filepath.Join(absDir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return absDir, nil
		}
		parent := filepath.Dir(absDir)
		if parent == absDir {
			break
		}
		absDir = parent
	}
	return "", fmt.Errorf("未找到 Git 仓库根目录")
}

func init() {
	chatCmd.Flags().StringVar(&chatModel, "model", "deepseek-chat", "使用的模型名称")
	rootCmd.AddCommand(chatCmd)
}
