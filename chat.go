package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/chimein"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/dsc"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/toolcall"
	"gitcode.com/dscli/dscli/internal/toolcall/alltools"
	"github.com/spf13/cobra"
)

const (
	DeepseekChat     = int64(0)
	DeepseekReasoner = int64(1)
)

func chatCommonPreRunE(cmd *cobra.Command, _ []string) (err error) {
	model, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var modelID int64
	switch model {
	case context.ModelDeepseekChat:
		ctx = context.WithValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
		modelID = DeepseekChat
	case context.ModelDeepseekReasoner:
		ctx = context.WithValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekReasoner)
		modelID = DeepseekReasoner
	default:
		err = fmt.Errorf("do not support %s", model)
		if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
			fmt.Printf("[DEBUG] ChatPreRunE: unsupported model error: %v\n", err)
		}
		return err
	}
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, modelID)

	// 读取 --role 标志并存入 context
	role, err := cmd.Flags().GetString("role")
	if err != nil || role == "" {
		role = "dev"
	}

	ctx = context.WithValue(ctx, context.CurrentRoleKey, role)

	// 计算工具 tokens
	var tokens int
	tools := alltools.GetAllTools(ctx)
	for _, tool := range tools {
		tokens += tool.GetTokens()
	}

	prompts, err := prompt.LoadPrompts(ctx)
	if err != nil {
		return err
	}
	for _, p := range prompts {
		tokens += p.GetTokens()
	}

	ctx = context.WithValue(ctx, context.LeftTokensKey, 131072-tokens)
	cmd.SetContext(ctx)

	return err
}

func ChatPreRunE(cmd *cobra.Command, args []string) (err error) {
	err = chatCommonPreRunE(cmd, args)
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	// 获取stream标志
	stream, err := cmd.Flags().GetBool("stream")
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, context.StreamKey, stream)
	cmd.SetContext(ctx)
	return err
}

func ChatRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	content := ""
	input := ""
	if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		input, err = cmd.Flags().GetString("input")
		if err != nil {
			return err
		}
		ctx = context.WithValue(ctx, context.InputContentKey, input)

		content, err = ReadContentWithTimeout(ctx)
		if err != nil {
			return err
		}
	}

	outfmt.PrintUserContent(ctx, content)
	histSize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, context.HistSizeKey, histSize)
	ctx = context.WithValue(ctx, context.StartTimeKey, time.Now())

	// Fetch starting balance
	var startBalance map[string]string
	if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
		startBalance = resp.BalanceInfos[0]
		ctx = context.WithValue(ctx, context.StartBalanceKey, startBalance)
	}
	prompts, err := prompt.LoadPrompts(ctx)
	if err != nil {
		return err
	}

	history, err := prompt.LoadHistory(ctx)
	if err != nil {
		return err
	}

	// Check if there is history and the last message has tool calls
	if len(history) > 0 {
		lastHist := history[len(history)-1]
		tcs := lastHist.ToolCalls
		if len(tcs) > 0 {
			// Print reasoning content or content
			outfmt.PrintContent(ctx, lastHist.ReasoningContent, lastHist.Content)
			toolInputs := toolcall.HandleToolCalls(ctx, tcs)
			// Execute tool calls
			history = append(history, toolInputs...)

			inputs := []prompt.Message{}
			if content != "" {
				inputs = append(inputs, prompt.Message{
					Role:    "user",
					Content: content,
				})
			}

			return ChatRound(ctx, prompts, history, inputs...)
		}
	}

	return ChatRound(ctx, prompts, history,
		prompt.Message{Role: "user", Content: content})
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
	input := context.ContextValue(ctx, context.InputContentKey, "")
	var b []byte
	if input == "" || input == "-" {
		reader := bufio.NewReader(os.Stdin)
		b, err = io.ReadAll(reader)
		if err != nil {
			return content, err
		}
		content = strings.TrimSpace(string(b))
		return content, err
	}
	b, err = os.ReadFile(input)
	if err != nil {
		return content, err
	}
	content = strings.TrimSpace(string(b))
	return content, err
}

// calculateCost 计算花费
func calculateCost(startBalance, endBalance map[string]string) string {
	// 解析余额字符串为浮点数
	startTotal, err1 := parseBalance(startBalance["total_balance"])
	endTotal, err2 := parseBalance(endBalance["total_balance"])

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
	return fmt.Sprintf("%s %.2f", startBalance["currency"], cost)
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
	startTime := context.ContextValue(ctx, context.StartTimeKey, time.Time{})
	startBalance := context.ContextValue(ctx, context.StartBalanceKey, map[string]string{})

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
	if startBalance["currency"] != "" {
		if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
			for _, balance := range resp.BalanceInfos {
				if balance["currency"] == startBalance["currency"] {
					// 计算花费
					cost := calculateCost(startBalance, balance)

					// 解析当前余额
					currentBalance, err := parseBalance(balance["total_balance"])
					if err != nil {
						currentBalance = 0
					}

					// 花费
					if cost != "" {
						stats = append(stats, fmt.Sprintf("💰 %s", cost))
					}

					// 余额
					stats = append(stats, fmt.Sprintf("💳 %s %s", balance["currency"], balance["total_balance"]))

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
		outfmt.Println(strings.Join(stats, "  "))
	}
}

func ChatRound(ctx context.Context, prompts, history []prompt.Message, inputs ...prompt.Message) (err error) {
	// 1. Construct messages slice (prompts → history → inputs)
	messages := make([]prompt.Message, 0, len(prompts)+len(history)+len(inputs))
	messages = append(messages, prompts...)
	messages = append(messages, history...)

	// 2. Add current user messages
	messages = append(messages, inputs...)

	// 3. Track new messages for this round (for storage)
	stories := make([]prompt.Message, 0, len(inputs)+1)
	stories = append(stories, inputs...)

	// 加载工具（非 dev 角色 GetAllTools 内部返回空）
	tools := alltools.GetAllTools(ctx)

	var resp *dsc.ChatResponse
	resp, err = DeepseekClient.Chat(ctx, messages, tools)
	if err != nil {
		messagesJSON, marshalErr := outfmt.JSONMarshal(messages)
		if marshalErr != nil {
			err = fmt.Errorf("chat request failed: %w", err)
		} else {
			err = fmt.Errorf("chat request failed: %w\nmessages=%s", err, string(messagesJSON))
		}
		return err
	}

	if len(resp.Choices) == 0 {
		err = fmt.Errorf("error: no response received")
		return err
	}

	// story retains ReasoningContent (for persistence and display),
	// dsc.Chat() will clean it up when used as input (API requirement)
	story := resp.Choices[0].Message
	// Check if response was truncated
	if resp.Choices[0].FinishReason == "length" {
		outfmt.Warn("note: response truncated due to length limit, may be incomplete.")
		ctx = context.WithValue(ctx, context.FinishReasonLengthKey, true)
	} else {
		if context.ContextValue(ctx, context.FinishReasonLengthKey, false) {
			ctx = context.WithValue(ctx, context.FinishReasonLengthKey, false)
		}
	}

	outfmt.PrintContent(ctx, story.ReasoningContent, story.Content)
	stories = append(stories, story)
	tcs := story.ToolCalls

	// save stories here
	err = prompt.SaveMessages(ctx, stories...)
	if err != nil {
		outfmt.Error("%v", err)
	}

	if len(stories) > 0 {
		history = append(history, stories...)
	}

	if len(tcs) == 0 {
		// Conversation ended, print stats
		PrintSessionStats(ctx)
		return err
	}

	toolInputs := toolcall.HandleToolCalls(ctx, tcs)
	if len(toolInputs) > 0 {
		// Tool call inputs saved in db, move them to history
		history = append(history, toolInputs...)

		// Check for chime-in: user may have inserted a message during tool execution
		if content, getErr := chimein.Get(ctx); getErr == nil && content != "" {
			msg := prompt.Message{Role: "user", Content: content}
			history = append(history, msg)
			outfmt.PrintClimeinContent(ctx, content)
			if saveErr := prompt.SaveMessages(ctx, msg); saveErr != nil {
				outfmt.Debug("failed to save chimein message: %v", saveErr)
			}
			chimein.Reset(ctx)
		}

		return ChatRound(ctx, prompts, history)
	}
	return err
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
	chatCmd.Flags().String("model", context.ModelDeepseekChat, "使用的模型名称")
	chatCmd.Flags().String("role", "dev", "角色：dev（开发助手）/ expert（领域专家）/ review（代码审查）")
	chatCmd.Flags().Int("histsize", 8, "history size loaded")
	chatCmd.Flags().String("input", "", "read content from input file or read content from stdin if input file empty")
	chatCmd.Flags().Bool("stream", false, "启用流式输出（SSE）")
}
