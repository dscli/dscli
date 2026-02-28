package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	DeepseekChat     = int64(0)
	DeepseekReasoner = int64(1)
)

var (
	chatModel string
	tobecont  bool
)

func ChatPreRunE(cmd *cobra.Command, args []string) (err error) {
	// 设置ModelID
	var modelID int64
	switch chatModel {
	case ModelDeepseekChat:
		modelID = DeepseekChat
	case ModelDeepseekReasoner:
		modelID = DeepseekReasoner
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
	userMsg := ""
	if !tobecont {
		userMsg, err = ReadContent()
		if err != nil {
			return
		}
	}
	ctx := cmd.Context()
	if userMsg != "" {
		return ChatMessage(ctx, Message{Role: "user", Content: userMsg})
	}

	am, err := LoadLastOne(ctx)
	if err != nil {
		return
	}

	if am.Role != "assistant" || len(am.ToolCalls) == 0 {
		Info("天下本无事")
		return
	}
	return HandleToolCalls(ctx, am)
}

func ReadContent() (content string, err error) {
	reader := bufio.NewReader(os.Stdin)
	b, err := io.ReadAll(reader)
	if err != nil {
		return
	}
	content = strings.TrimSpace(string(b))
	return
}

func ChatMessage(ctx context.Context, inputs ...Message) (err error) {
	// 1. 加载历史消息
	history, err := LoadHistory(ctx)
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
	resp, err = DeepseekClient.Chat(chatModel, messages, GetAllTools())
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

	if ModelID == DeepseekReasoner && assistantMsg.ReasoningContent != "" {
		Printf("已思考(用时%v)\n\n", time.Since(startTime))
		Println(assistantMsg.ReasoningContent)
		Println("\n------")
	}

	Println(assistantMsg.Content)
	return HandleToolCalls(ctx, &assistantMsg)
}

func init() {
	chatCmd := &cobra.Command{
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
	chatCmd.Flags().StringVar(&chatModel, "model", ModelDeepseekChat, "使用的模型名称")
	chatCmd.Flags().BoolVar(&tobecont, "continue", false, "继续")
	RootCmd.AddCommand(chatCmd)
}
