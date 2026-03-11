package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	DeepseekChat     = int64(0)
	DeepseekReasoner = int64(1)
)

func ChatPreRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	model, err := cmd.Flags().GetString("model")
	if err != nil {
		return
	}

	if model == "" {
		model = ModelDeepseekChat
	}

	ctx = context.WithValue(ctx, CurrentModelName, model)
	// 调试：打印chatModel和ModelDeepseekChat的值
	if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
		fmt.Printf("[DEBUG] ChatPreRunE: chatModel='%s', ModelDeepseekChat='%s'\n",
			model, ModelDeepseekChat)
	}

	// 设置ModelID
	var modelID int64
	switch model {
	case ModelDeepseekChat:
		modelID = DeepseekChat
	case ModelDeepseekReasoner:
		modelID = DeepseekReasoner
	default:
		err = fmt.Errorf("do not support %s", model)
		if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
			fmt.Printf("[DEBUG] ChatPreRunE: unsupported model error: %v\n", err)
		}
		return
	}
	ctx = context.WithValue(ctx, CurrentModelID, modelID)

	// 获取stream标志
	stream, err := cmd.Flags().GetBool("stream")
	if err != nil {
		return
	}
	ctx = context.WithValue(ctx, "streaming_enabled", stream)

	sessionID, err := CreateOrGetSessionID()
	if err != nil {
		return
	}
	ctx = context.WithValue(ctx, CurrentSessionID, sessionID)
	ctx = context.WithValue(ctx, InsideShellExec, os.Getenv(string(InsideShellExec)) == "1")
	cmd.SetContext(ctx)
	return
}

func ChatRunE(cmd *cobra.Command, args []string) (err error) {
	if err != nil {
		return
	}
	ctx := cmd.Context()
	content := ""
	input := ""
	if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		input, err = cmd.Flags().GetString("input")
		if err != nil {
			return
		}
		ctx = context.WithValue(ctx, InputContent, input)

		content, err = ReadContentWithTimeout(ctx)
		if err != nil {
			return
		}
	}
	histSize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return
	}
	ctx = context.WithValue(ctx, HistSize, histSize)
	ctx = context.WithValue(ctx, StartTime, time.Now())

	// 获取开始余额
	var startBalance BalanceInfo
	if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
		// 使用第一个余额信息（通常是CNY）
		startBalance = resp.BalanceInfos[0]
		ctx = context.WithValue(ctx, StartBalance, startBalance)
	}

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

	// 检查是否有历史记录，并且最后一个历史记录包含工具调用
	if len(history) > 0 {
		lastHist := history[len(history)-1]
		tcs := lastHist.ToolCalls
		if len(tcs) > 0 {
			// 执行工具调用
			history = append(history, HandleToolCalls(ctx, tcs)...)

			inputs := []Message{}
			if content != "" {
				inputs = append(inputs, Message{
					Role:    "user",
					Content: content,
				})
			}

			return ChatRound(ctx, prompts, skills, history, inputs...)
		}
	}

	return ChatRound(ctx, prompts, skills, history,
		Message{Role: "user", Content: content})
}

func ReadContentWithTimeout(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, time.Second*10)
	defer cancel()
	// 用于传递读取结果的通道
	resultCh := make(chan string, 1)
	errCh := make(chan error, 1)

	go func() {
		content, err := ReadContent(ctx)
		if err != nil {
			errCh <- err
		}
		resultCh <- content
	}()

	select {
	case <-ctx.Done():
		// context 超时或取消
		return "", ctx.Err()
	case res := <-resultCh:
		return res, nil
	case err := <-errCh:
		return "", err
	}
}

func ReadContent(ctx context.Context) (content string, err error) {
	input := ContextValue(ctx, InputContent, "")
	var b []byte
	if input == "" || input == "-" {
		reader := bufio.NewReader(os.Stdin)
		b, err = io.ReadAll(reader)
		if err != nil {
			return
		}
		content = strings.TrimSpace(string(b))
		return
	}
	b, err = os.ReadFile(input)
	if err != nil {
		return
	}
	content = strings.TrimSpace(string(b))
	return
}

func PrintContent(ctx context.Context, reasoning string, content string) {
	reasoning = strings.TrimSpace(reasoning)
	if reasoning != "" {
		Println(reasoning)
	}

	content = strings.TrimSpace(content)
	if content != "" {
		content = strings.TrimSpace(content)
		Println(content)
	}
}

// calculateCost 计算花费
func calculateCost(startBalance, endBalance BalanceInfo) string {
	// 解析余额字符串为浮点数
	startTotal, err1 := parseBalance(startBalance.TotalBalance)
	endTotal, err2 := parseBalance(endBalance.TotalBalance)

	if err1 != nil || err2 != nil {
		return "" // 解析失败，不显示花费
	}

	// 计算花费（开始余额 - 结束余额）
	cost := startTotal - endTotal

	// 如果花费很小或为负数，不显示
	if cost <= 0 {
		return ""
	}

	// 格式化花费，精确到分
	return fmt.Sprintf("%s %.2f", startBalance.Currency, cost)
}

// parseBalance 解析余额字符串
func parseBalance(balanceStr string) (float64, error) {
	// 移除货币符号和空格
	balanceStr = strings.TrimSpace(balanceStr)
	// 尝试解析为浮点数
	return strconv.ParseFloat(balanceStr, 64)
}

// PrintSessionStats 打印会话统计信息
func PrintSessionStats(ctx context.Context) {
	startTime := ContextValue(ctx, StartTime, time.Time{})
	startBalance := ContextValue(ctx, StartBalance, BalanceInfo{})

	// 收集要显示的信息
	var stats []string

	// 用时
	if !startTime.IsZero() {
		duration := time.Since(startTime)
		var durationStr string
		if duration.Seconds() < 60 {
			durationStr = fmt.Sprintf("%.1fs", duration.Seconds())
		} else if duration.Minutes() < 60 {
			durationStr = fmt.Sprintf("%.1fm", duration.Minutes())
		} else {
			durationStr = fmt.Sprintf("%.1fh", duration.Hours())
		}
		stats = append(stats, fmt.Sprintf("⏱️ %s", durationStr))
	}

	// 花费和余额
	if startBalance.Currency != "" {
		if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
			for _, balance := range resp.BalanceInfos {
				if balance.Currency == startBalance.Currency {
					// 计算花费
					cost := calculateCost(startBalance, balance)

					// 解析当前余额
					currentBalance, err := parseBalance(balance.TotalBalance)
					if err != nil {
						currentBalance = 0
					}

					// 花费
					if cost != "" {
						stats = append(stats, fmt.Sprintf("💰 %s", cost))
					}

					// 余额
					stats = append(stats, fmt.Sprintf("💳 %s %s", balance.Currency, balance.TotalBalance))

					// 如果余额较低，显示提醒
					if currentBalance < 10.0 { // 余额低于10元时提醒
						stats = append(stats, "⚠️ 余额较低，请及时充值！")
					}

					break
				}
			}
		}
	}

	// 在一行中显示所有统计信息
	if len(stats) > 0 {
		Println(strings.Join(stats, "  "))
	}
}

func ChatRound(ctx context.Context, prompts []Message, skills []Message, history []Message, inputs ...Message) (err error) {
	// 1. 构造 messages 切片（包含历史）
	messages := make([]Message, 0, len(history)+len(prompts)+len(skills))
	messages = append(messages, prompts...)
	messages = append(messages, history...)

	// 2. 添加当前用户消息
	messages = append(messages, inputs...)

	// 3. 记录本轮新增的消息（用于存储）
	stories := make([]Message, 0, len(inputs)+1)
	stories = append(stories, inputs...)

	// 获取stream标志
	stream := false
	if streamVal := ctx.Value("streaming_enabled"); streamVal != nil {
		if s, ok := streamVal.(bool); ok {
			stream = s
		}
	}

	var resp *ChatResponse
	resp, err = DeepseekClient.Chat(ctx, messages, GetAllTools(ctx), stream)
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
	err = SaveMessages(ctx, stories...)
	story.ReasoningContent = "" // reset reasoning content

	if err != nil {
		Error("%v", err)
	}
	if len(stories) > 0 {
		history = append(history, stories...)
	}

	tcs := story.ToolCalls
	if len(tcs) == 0 {
		// 会话结束，打印统计信息
		PrintSessionStats(ctx)
		return
	}

	toolInputs := HandleToolCalls(ctx, tcs)
	if len(toolInputs) > 0 {
		// Now tool call inputs saved in db
		// move them to history
		history = append(history, toolInputs...)
		return ChatRound(ctx, prompts, skills, history)
	}
	return
}

func init() {
	chatCmd := AddRootCommand(&cobra.Command{
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
	})
	chatCmd.Flags().String("model", ModelDeepseekChat, "使用的模型名称")
	chatCmd.Flags().Int("histsize", 8, "history size loaded")
	chatCmd.Flags().String("input", "", "read content from input file or read content from stdin if input file empty")
	chatCmd.Flags().Bool("stream", false, "启用流式输出（SSE）")
}
