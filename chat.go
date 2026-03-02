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
	cont      bool
	abort     bool
	reload    bool
)

var (
	Abortion        = ContextKeyType("Abortion")
	Continue        = ContextKeyType("Continue")
	StartTime       = ContextKeyType("StartTime")
	CurrentModel    = ContextKeyType("CurrentModel")
	CurrentContent  = ContextKeyType("CurrentContent")
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
	content := ""
	if !cont {
		content, err = ReadContent()
		if err != nil {
			return
		}
	}
	content = strings.TrimSpace(content)
	if content == "" {
		cont = true
	}

	ctx := cmd.Context()
	ctx = context.WithValue(ctx, StartTime, time.Now())
	ctx = context.WithValue(ctx, CurrentModel, chatModel)
	ctx = context.WithValue(ctx, Continue, cont)
	ctx = context.WithValue(ctx, Abortion, abort)
	ctx = context.WithValue(ctx, CurrentContent, content)
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

	if !cont && !abort {
		return ChatRound(ctx, prompts, skills, history,
			Message{Role: "user", Content: content})
	}

	histsize := len(history)
	if histsize == 0 {
		Info("天下无事")
		return
	}

	last := history[histsize-1]
	cts := last.ToolCalls
	if last.Role != "assistant" || len(cts) == 0 {
		Info("天下本无事")
		return
	}
	history = history[0 : histsize-1]

	// handle abortion first
	if abort {
		return ChatRound(ctx, prompts, skills, history,
			Message{
				Role:       "tool",
				ToolCallID: cts[0].ID,
				Content: fmt.Sprintf(`TOOL %s FATAL ERROR!!!
NO NEED TO RETRY!!!
LEAVE THINGS TO HUMAN TO HANDLE!!!`, cts[0].Function.Name),
			})
	}
	if cont {
		inputs := HandleToolCalls(ctx, cts)
		if len(inputs) == 0 {
			Warn("inputs should not be empty!")
			return
		}
		return ChatRound(ctx, prompts, skills, history, inputs...)
	}
	return
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

func PrintReasoningContent(ctx context.Context, reasoning string) {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning == "" {
		return
	}
	var startTime time.Time
	if v, ok := ctx.Value(StartTime).(time.Time); ok {
		startTime = v
	}
	duration := time.Since(startTime)
	seconds := duration.Seconds()
	Printf("已思考用时%.2fs\n\n", seconds)
	Println(reasoning)
}

func PrintContent(ctx context.Context, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	var startTime time.Time
	if v, ok := ctx.Value(StartTime).(time.Time); ok {
		startTime = v
	}
	duration := time.Since(startTime)
	seconds := duration.Seconds()
	Printf("用时%.2fs\n\n", seconds)
	content = strings.TrimSpace(content)
	Println(content)
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
	PrintReasoningContent(ctx, story.ReasoningContent)
	PrintContent(ctx, story.Content)
	// 转换并保存到 newMessages（用于后续存储）
	stories = append(stories, story)
	if len(stories) > 0 {
		if err = SaveMessagesBatch(stories); err != nil {
			err = fmt.Errorf("保存消息失败: %w", err)
			return
		}
	}
	tcs := story.ToolCalls
	if len(tcs) == 0 {
		return
	}

	// 检查是否是工具调用的结果（递归调用）
	isToolResult := len(inputs) > 0 && inputs[0].Role == "tool"

	if !isToolResult {
		// 第一次调用，打印详细信息
		Println("调用", len(tcs), "个工具：")
		for i, tc := range tcs {
			Printf("  %d. %s\n", i+1, tc.Function.Name)
		}
	} else {
		// 递归调用，只打印简单信息
		Println("继续调用", len(tcs), "个工具...")
	}

	toolInputs := HandleToolCalls(ctx, tcs)
	if len(toolInputs) > 0 {
		history = append(history, inputs...) // put inputs in history
		story.ReasoningContent = ""          // reset reasoning content
		history = append(history, story)     // put story in history
		return ChatRound(ctx, prompts, skills, history, toolInputs...)
	}
	return
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
	chatCmd.Flags().BoolVar(&cont, "continue", false, "继续")
	chatCmd.Flags().BoolVar(&abort, "abort", false, "放弃")
	chatCmd.Flags().BoolVar(&reload, "reload", false, "重载进程（内部使用）")
	RootCmd.AddCommand(chatCmd)
}
