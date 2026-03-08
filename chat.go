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

var chatModel string

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
	histSize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return
	}
	ctx = context.WithValue(ctx, HistSize, histSize)
	ctx = context.WithValue(ctx, StartTime, time.Now())
	ctx = context.WithValue(ctx, CurrentModel, chatModel)

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
	startTime := ContextValue(ctx, StartTime, time.Time{})
	startBalance := ContextValue(ctx, StartBalance, BalanceInfo{})
	reasoning = strings.TrimSpace(reasoning)
	if reasoning != "" {
		Println(reasoning)
	}

	content = strings.TrimSpace(content)
	if content != "" {
		content = strings.TrimSpace(content)
		Println(content)
	}

	// 计算并打印用时（如果提供了开始时间）
	if !startTime.IsZero() {
		duration := time.Since(startTime)
		// 格式化时间，保留1-2位小数
		var durationStr string
		if duration.Seconds() < 60 {
			// 小于60秒，显示秒，保留1位小数
			durationStr = fmt.Sprintf("%.1fs", duration.Seconds())
		} else if duration.Minutes() < 60 {
			// 小于60分钟，显示分钟，保留1位小数
			durationStr = fmt.Sprintf("%.1fm", duration.Minutes())
		} else {
			// 大于等于60分钟，显示小时，保留1位小数
			durationStr = fmt.Sprintf("%.1fh", duration.Hours())
		}
		Println(fmt.Sprintf("⏱️  用时: %s", durationStr))
	}

	// 计算并打印花费（如果提供了开始余额）
	if startBalance.Currency != "" {
		// 获取当前余额
		if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
			// 查找与开始余额相同货币的余额信息
			for _, balance := range resp.BalanceInfos {
				if balance.Currency == startBalance.Currency {
					// 计算花费
					cost := calculateCost(startBalance, balance)
					if cost != "" {
						Println(fmt.Sprintf("💰  花费: %s", cost))
					}
					break
				}
			}
		}
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

func ChatRound(ctx context.Context, prompts []Message, skills []Message, history []Message, inputs ...Message) (err error) {
	// 在每次 ChatRound 开始时更新 StartTime
	ctx = context.WithValue(ctx, StartTime, time.Now())

	// 获取开始余额
	var startBalance BalanceInfo
	if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
		// 使用第一个余额信息（通常是CNY）
		startBalance = resp.BalanceInfos[0]
		ctx = context.WithValue(ctx, StartBalance, startBalance)
	}

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
	err = SaveMessages(stories...)
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
	chatCmd.Flags().StringVar(&chatModel, "model", ModelDeepseekChat, "使用的模型名称")
	chatCmd.Flags().Int("histsize", 8, "history size loaded")
}
