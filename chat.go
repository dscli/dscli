package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	DEEPSEEK_CHAT     = int64(0)
	DEEPSEEK_REASONER = int64(1)
)

var (
	chatModel string
)

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
	PreRunE: ChatPreRunE,
	RunE:    ChatRunE,
}

func ChatPreRunE(cmd *cobra.Command, args []string) (err error) {
	// 设置ModelID
	var modelID int64
	switch chatModel {
	case "deepseek-chat":
		modelID = DEEPSEEK_CHAT
	case "deepseek-reasoner":
		modelID = DEEPSEEK_REASONER
	default:
		err = fmt.Errorf("do not support %s", chatModel)
		return
	}

	// 设置全局ModelID
	ModelID = modelID

	return
}

func ChatRunE(cmd *cobra.Command, args []string) (err error) {
	// 1. 读取标准输入
	reader := bufio.NewReader(os.Stdin)
	content, err := io.ReadAll(reader)
	if err != nil {
		return
	}
	userMsg := strings.TrimSpace(string(content))
	if userMsg != "" {
		return ChatMessage(Message{Role: "user", Content: userMsg})
	}

	dm, err := LoadLastOne()
	if err != nil {
		return
	}

	am := ToApiMessage(dm)
	if am.Role != "assistant" || len(am.ToolCalls) == 0 {
		fmt.Println("天下本无事")
		return
	}
	return HandleToolCalls(am)
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

func ChatMessage(inputs ...Message) (err error) {
	// 1. 加载历史消息
	history, err := LoadHistory()
	if err != nil {
		err = fmt.Errorf("加载历史消息失败: %w", err)
		return
	}

	// 2. 系统prompt
	systemMessage := Message{
		Role:    "system",
		Content: GetSystemPrompt(),
	}
	// 3. 构造 messages 切片（包含历史）
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
	// 4. 添加当前用户消息
	messages = append(messages, inputs...)

	// 5. 记录本轮新增的消息（用于存储）
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
		if err = SaveMessagesBatch(dbmessages); err != nil {
			err = fmt.Errorf("保存消息失败: %w", err)
			return
		}
	}

	if ModelID == DEEPSEEK_REASONER && assistantMsg.ReasoningContent != "" {
		fmt.Println(assistantMsg.ReasoningContent)
	}

	fmt.Println(assistantMsg.Content)
	return HandleToolCalls(&assistantMsg)
}

func init() {
	chatCmd.Flags().StringVar(&chatModel, "model", "deepseek-chat", "使用的模型名称")
	rootCmd.AddCommand(chatCmd)

	// 初始化工具系统
	InitTools()
}
