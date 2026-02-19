package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gitcode.com/nanjunjie/dscli/internal/api"
	"gitcode.com/nanjunjie/dscli/internal/db"
	"gitcode.com/nanjunjie/dscli/internal/log"
	"github.com/spf13/cobra"
)

var chatModel string

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
	log.Debug("dm role=%s", role)
	am = &api.Message{
		Role:       role,
		Content:    dm.Content,
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
		// 使用新的工具调用处理器
		result, err := HandleToolCall(tc.Function.Name, projectRoot, []byte(tc.Function.Arguments))
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
		Role:    "system",
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
	resp, err = client.ChatWithTools(chatModel, messages, GetAllTools())
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
	// 初始化工具系统
	InitTools()
	
	chatCmd.Flags().StringVar(&chatModel, "model", "deepseek-chat", "使用的模型名称")
	rootCmd.AddCommand(chatCmd)
}
