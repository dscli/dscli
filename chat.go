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
	reload    bool
)

var (
	StartTime       = ContextKeyType("StartTime")
	CurrentModel    = ContextKeyType("CurrentModel")
	IsReload        = ContextKeyType("IsReload")
	CommandLineArgs = ContextKeyType("CommandLineArgs")
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
	content, err := ReadContent()
	if err != nil {
		return
	}

	ctx := cmd.Context()
	ctx = context.WithValue(ctx, StartTime, time.Now())
	ctx = context.WithValue(ctx, CurrentModel, chatModel)
	ctx = context.WithValue(ctx, IsReload, reload)
	// 存储命令行参数
	ctx = context.WithValue(ctx, CommandLineArgs, os.Args)

	prompts, err := LoadPrompts(ctx)
	if err != nil {
		return
	}

	skills, err := LoadSkills(ctx)
	if err != nil {
		return
	}

	history, err := LoadHistory(ctx)
	if err != nil {
		return
	}

	// 如果是重载进程，需要特殊处理
	if reload {
		return handleReload(ctx, prompts, skills, history)
	}

	return ChatRound(ctx, prompts, skills, history,
		Message{Role: "user", Content: content})
}

// handleReload 处理重载逻辑
func handleReload(ctx context.Context, prompts []Message, skills []Message, history []Message) (err error) {
	Info("🔄 检测到重载进程，正在恢复对话...")

	// 找到最后一个assistant消息（包含未完成的工具调用）
	var lastAssistant *Message
	for idx := len(history) - 1; idx >= 0; idx-- {
		if history[idx].Role == "assistant" && len(history[idx].ToolCalls) > 0 {
			lastAssistant = &history[idx]
			break
		}
	}

	if lastAssistant == nil {
		Warn("未找到未完成的工具调用，继续正常对话")
		return ChatRound(ctx, prompts, skills, history)
	}

	// 处理未完成的工具调用
	tcs := lastAssistant.ToolCalls
	Info("恢复处理 %d 个未完成的工具调用...", len(tcs))

	// 执行工具调用
	toolInputs := HandleToolCalls(ctx, tcs)

	// 移除最后一个assistant消息（因为它包含未完成的工具调用）
	newHistory := make([]Message, 0, len(history)-1)
	for _, msg := range history {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// 跳过这个未完成的消息
			continue
		}
		newHistory = append(newHistory, msg)
	}

	// 继续对话
	if len(toolInputs) > 0 {
		return ChatRound(ctx, prompts, skills, newHistory, toolInputs...)
	}

	return nil
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

func PrintContent(ctx context.Context, reasoning string, content string) {
	var startTime time.Time
	if v, ok := ctx.Value(StartTime).(time.Time); ok {
		startTime = v
	}

	reasoning = strings.TrimSpace(reasoning)
	if reasoning != "" {
		duration := time.Since(startTime)
		seconds := duration.Seconds()
		Printf("已思考用时%.2fs\n\n", seconds)
		Println(reasoning)
	}

	content = strings.TrimSpace(content)
	if content != "" {
		duration := time.Since(startTime)
		seconds := duration.Seconds()
		Printf("用时%.2fs\n\n", seconds)
		content = strings.TrimSpace(content)
		Println(content)
	}
}

func ChatRound(ctx context.Context, prompts []Message, skills []Message, history []Message, inputs ...Message) (err error) {
	// 在每次 ChatRound 开始时更新 StartTime
	ctx = context.WithValue(ctx, StartTime, time.Now())

	// 1. 构造 messages 切片（包含历史）
	messages := make([]Message, 0, len(history)+len(prompts)+len(skills))
	messages = append(messages, prompts...)
	messages = append(messages, history...)
	// 2. 添加当前用户消息
	messages = append(messages, inputs...)

	// 3. 记录本轮新增的消息（用于存储）
	stories := make([]Message, 0, len(inputs)+1)
	stories = append(stories, inputs...)
	var resp *ChatResponse
	resp, err = DeepseekClient.Chat(chatModel, messages, GetAllTools())
	if err != nil {
		err = fmt.Errorf("聊天请求失败: %w", err)
		return
	}

	if len(resp.Choices) == 0 {
		err = fmt.Errorf("错误: 未收到回复")
		return
	}

	story := resp.Choices[0].Message
	PrintContent(ctx, story.ReasoningContent, story.Content)
	stories = append(stories, story)
	// save stories here
	err = SaveMessages(stories)
	story.ReasoningContent = "" // reset reasoning content

	if err != nil {
		Error("%v", err)
	}
	if len(stories) > 0 {
		history = append(history, stories...)
	}

	tcs := story.ToolCalls
	if len(tcs) == 0 {
		return
	}

	PrintToolCalls(ctx, tcs)
	toolInputs := HandleToolCalls(ctx, tcs)
	if len(toolInputs) > 0 {
		return ChatRound(ctx, prompts, skills, history, toolInputs...)
	}
	return
}

func PrintToolCalls(ctx context.Context, tcs []ToolCall) {
	names := []string{}
	for _, tc := range tcs {
		names = append(names, GetToolDisplayName(tc.Function.Name))
	}
	Printf("Running Tool Calls: %v\n", names)
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
	chatCmd.Flags().BoolVar(&reload, "reload", false, "重载进程（内部使用）")
	RootCmd.AddCommand(chatCmd)
}
