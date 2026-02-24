package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	DEEPSEEK_CHAT     = int64(0)
	DEEPSEEK_REASONER = int64(1)
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
	// 设置全局SessionID
	SessionID, err = CreateOrGetSessionID()
	if err != nil {
		return
	}
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

	am, err := LoadLastOne()
	if err != nil {
		return
	}

	if am.Role != "assistant" || len(am.ToolCalls) == 0 {
		fmt.Println("天下本无事")
		return
	}
	return HandleToolCalls(am)
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
	messages = append(messages, history...)
	// 4. 添加当前用户消息
	messages = append(messages, inputs...)

	// 5. 记录本轮新增的消息（用于存储）
	news := make([]Message, 0, len(inputs)+1)
	news = append(news, inputs...)

	var resp *ChatResponse
	startTime := time.Now()
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
	news = append(news, assistantMsg)
	if len(news) > 0 {
		if err = SaveMessagesBatch(news); err != nil {
			err = fmt.Errorf("保存消息失败: %w", err)
			return
		}
	}

	if ModelID == DEEPSEEK_REASONER && assistantMsg.ReasoningContent != "" {
		fmt.Printf("已思考(用时%v)\n\n", time.Since(startTime))
		fmt.Println(assistantMsg.ReasoningContent)
		fmt.Println("\n------")
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
