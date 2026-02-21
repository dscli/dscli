package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	// 1. 确定项目根路径
	projectRoot, err := getProjectRoot()
	if err != nil {
		err = fmt.Errorf("无法确定项目根路径: %w", err)
		return
	}

	// 2. 打开数据库
	database, err := New()
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
	if userMsg != "" {
		return ChatMessage(database, projectRoot, sessionID, Message{Role: "user", Content: userMsg})
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

func ToApiMessage(dm *RawMessage) (am *Message) {
	role := dm.Role
	am = &Message{
		Role:       role,
		Content:    dm.Content,
		ToolCallID: dm.ToolCallID,
	}
	if len(dm.ToolCalls) != 0 {
		err := json.Unmarshal(dm.ToolCalls, &am.ToolCalls)
		if err != nil {
			return nil
		}
	}
	return
}

func ToDBMessage(apim Message) (dbm RawMessage) {
	role := apim.Role
	dbm.Content = apim.Content
	dbm.Role = apim.Role
	if role == "tool" {
		dbm.ToolCallID = apim.ToolCallID
	}
	if role == "assistant" && len(apim.ToolCalls) > 0 {
		data, err := json.Marshal(apim.ToolCalls)
		if err != nil {
			return
		}
		dbm.ToolCalls = data
	}
	return
}

func ChatMessage(database *DB, projectRoot string, sessionID int64, inputs ...Message) (err error) {
	// 5. 加载历史消息
	history, err := database.LoadHistory(sessionID)
	if err != nil {
		err = fmt.Errorf("加载历史消息失败: %w", err)
		return
	}

	// 添加系统提示（包含当前日期）
	currentDate := time.Now().Format("2006-01-02")
	// 计算一个相对日期（比如一年前）
	oneYearAgo := time.Now().AddDate(-1, 0, 0).Format("2006年1月2日")

	systemMessage := Message{
		Role: "system",
		Content: fmt.Sprintf(`你是一个专业的编程助手。
当前日期：%s，注意你的知识截至于当前日期之前，比如%s，请基于当前日期处理与时间相关的需求。
当前工作目录：%s ，你可以操作（增删改查）当前工作目录下的所有文件和目录，注意当前工作目录由版本控制系统Git管控，你最好不要直接读写.git目录下的文件，但你可以通过git操作。
配置目录：~/.dscli，你可操作配置目录下的任何文件，但不能删除以下文件 1) sqlite.db，2) dscli.env，你可以通过数据库接口如sqlite3操作数据库文件。
你的工作流程：
1. 仔细分析用户的问题，拆解出需要完成的步骤，
2. 如果需要运行修改代码，搜索信息，文件读写，Git操作或执行其他操作，请调用相应的工具（工具列表已通过API工具参数提供），
3. 在调用工具前，可以用自然语言简要说明你的计划，或者调用工具要达到的目的（可选），
4. 当工具返回结果后，分析结果并决定下一步的行动，直至任务完成，
5. 最终给出清晰，准确的答案。

请保持逻辑严谨，逐步推进。`, currentDate, oneYearAgo, projectRoot),
	}
	// 6. 构造 messages 切片（包含历史）
	messages := make([]Message, 0, len(history)+2)
	messages = append(messages, systemMessage)
	for _, m := range history {
		apiMsg := Message{
			Role:    m.Role,
			Content: m.Content,
		}
		if m.ToolCallID != "" {
			apiMsg.ToolCallID = m.ToolCallID
		}
		if len(m.ToolCalls) > 0 {
			var toolCalls []ToolCall
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
	var dbmessages []RawMessage
	for _, m := range inputs {
		dbmessages = append(dbmessages, ToDBMessage(m))
	}

	var resp *ChatResponse
	resp, err = client.Chat(chatModel, messages, GetAllTools())
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
