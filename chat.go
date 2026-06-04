package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/ainame"
	"gitcode.com/dscli/dscli/internal/chimein"
	"gitcode.com/dscli/dscli/internal/config"
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/dsc"
	"gitcode.com/dscli/dscli/internal/lockfile"
	"gitcode.com/dscli/dscli/internal/mail"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/price"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/toolcall"
	"gitcode.com/dscli/dscli/internal/toolcall/alltools"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const (
	DeepseekChat = int64(0)
)

func ChatPreRunE(cmd *cobra.Command, args []string) (err error) {
	model, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	var modelID int64
	switch model {
	case context.ModelDeepseekChat:
		modelID = DeepseekChat
	default:
		err = fmt.Errorf("do not support %s", model)
		if verbose, _ := cmd.Flags().GetBool("verbose"); verbose {
			fmt.Printf("[DEBUG] ChatPreRunE: unsupported model error: %v\n", err)
		}
		return err
	}

	ctx = context.WithValue(ctx, context.CurrentModelNameKey, model)
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, modelID)
	// 读取 --role 标志并存入 context
	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return err
	}

	if role == "" {
		role = "dev"
	}

	ctx = context.WithValue(ctx, context.CurrentRoleKey, role)

	// 从配置读取上下文窗口大小（默认 1,000,000，对应 DeepSeek V4 百万 token 上下文）。
	// 此值用作历史消息 token 预算的上限，实际截断主要由 --histsize 控制。
	// 配置文件 key: context-window，环境变量: CONTEXT_WINDOW。
	contextWindow := config.GetInt("context-window", 1000000)
	ctx = context.WithValue(ctx, context.LeftTokensKey, contextWindow)

	// 获取stream标志
	stream, err := cmd.Flags().GetBool("stream")
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, context.StreamKey, stream)

	histSize, err := cmd.Flags().GetInt("histsize")
	if err != nil {
		return err
	}

	ctx = context.WithValue(ctx, context.HistSizeKey, histSize)
	cmd.SetContext(ctx)
	return err
}

func ChatRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	// 1. 优先从非阻塞来源读取内容（args 或 --input 文件路径）。
	//    此时不读取 stdin，避免在没有主进程时因无输入而超时出错。
	content, needStdin, err := gatherInput(cmd, args)
	if err != nil {
		return err
	}
	// 2. 尝试获取项目级文件锁。
	//    若已有其他 dscli chat 进程在运行，降级为 climein 模式：
	//    将内容写入 chimeins 表，由主进程在下一轮 ChatRound 注入。
	//    子进程（code review / ask expert）在 lockfile 层通过父进程 PID
	//    判定自动放行，此处无需特殊处理。
	lk, isPrimary, err := lockfile.TryLockLocal()
	if err != nil {
		return fmt.Errorf("lockfile: %w", err)
	}

	if !isPrimary {
		// 降级为 climein
		if needStdin {
			input, _ := cmd.Flags().GetString("input")
			// --input - 明确要求读取 stdin；或 stdin 是管道（非 TTY）时也读取，
			// 避免丢弃用户通过管道传入的内容（如 echo "msg" | dscli chat）。
			if input != "-" && isTerminal(os.Stdin) {
				outfmt.Println("⚠️ 插话内容为空，未执行任何操作。")
				return nil
			}
			b, readErr := io.ReadAll(bufio.NewReader(os.Stdin))
			if readErr != nil {
				return fmt.Errorf("读取标准输入失败: %w", readErr)
			}
			content = strings.TrimSpace(string(b))
		}
		if content == "" {
			outfmt.Println("⚠️ 插话内容为空，未执行任何操作。")
			return nil
		}
		if outfmt.GetOutputMode() == "org" {
			content = outfmt.OrgToMarkdown(content)
		}
		if appendErr := chimein.Append(ctx, content); appendErr != nil {
			return appendErr
		}
		outfmt.PrintUserContent(ctx, content)
		outfmt.Println("✅ 已有主 chat 进程运行中，内容已作为插话追加。")
		return nil
	}

	// 主进程（或 standalone 模式）：持有锁直到进程退出
	if lk != nil {
		defer lk.Close()
	} else if isPrimary {
		// 子进程（code_review / ask_expert）：父进程持有锁，
		// lockfile 通过父进程 PID 判定放行。子进程不显示余额等统计信息。
		ctx = context.WithValue(ctx, context.IsChildProcessKey, true)
	}

	// Set AI name in context for output formatting
	sessionID := session.GetCurrentSessionID(ctx)
	cfg := ainame.LoadOrAssign(sessionID)
	ctx = context.WithValue(ctx, context.AINameCNKey, cfg.NameCN)
	ctx = context.WithValue(ctx, context.AINameENKey, cfg.NameEN)
	ctx = context.WithValue(ctx, context.AINameEmailKey, cfg.Email)
	ctx = context.WithValue(ctx, context.AINameBirdFrogKey, cfg.BirdFrog)

	// Set Git user info in context for output formatting
	ctx = context.WithValue(ctx, context.GitUserNameKey, context.GitUserName())
	ctx = context.WithValue(ctx, context.GitUserEmailKey, context.GitUserEmail())

	// 3. 主进程：如果还需要从 stdin 读取，现在阻塞读取（没有超时限制，
	//    因为用户正在主动使用 chat）。
	if needStdin {
		b, readErr := io.ReadAll(bufio.NewReader(os.Stdin))
		if readErr != nil {
			return fmt.Errorf("读取标准输入失败: %w", readErr)
		}
		content = strings.TrimSpace(string(b))
	}

	if outfmt.GetOutputMode() == "org" {
		content = outfmt.OrgToMarkdown(content)
	}

	outfmt.PrintUserContent(ctx, content)

	// Inject unread mail notification into the user message so the AI
	// can't miss it. Unlike system prompt notifications, user messages
	// demand a response — the AI must acknowledge and act on them.
	if summaries := mail.UnreadMailList(ctx); len(summaries) > 0 {
		if notif := mail.FormatUnreadMailNotification(summaries); notif != "" {
			if content != "" {
				content = notif + "\n\n---\n\n" + content
			} else {
				content = notif
			}
		}
	}

	ctx = context.WithValue(ctx, context.StartTimeKey, time.Now())

	// Fetch starting balance (only when user-balance is enabled)
	var startBalance map[string]string
	if config.GetBool("user-balance", true) {
		if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
			startBalance = resp.BalanceInfos[0]
			ctx = context.WithValue(ctx, context.StartBalanceKey, startBalance)
		}
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

			// Inject pending chime-in before next ChatRound.
			// Chime-ins may have accumulated while the process was not running.
			history, _ = injectChimein(ctx, history)

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

// ReadInput reads user input content from CLI args or --input flag.
// Priority: positional args (space-joined) > --input flag (file path or "-" for stdin).
// When output mode is "org", the input (org format) is converted to markdown
// before being used as message/chimein content.
func ReadInput(cmd *cobra.Command, args []string) (string, error) {
	var content string
	if len(args) > 0 {
		content = strings.Join(args, " ")
	} else {
		input, err := cmd.Flags().GetString("input")
		if err != nil {
			return "", err
		}
		content, err = readInputSource(input)
		if err != nil {
			return "", err
		}
	}

	if outfmt.GetOutputMode() == "org" {
		content = outfmt.OrgToMarkdown(content)
	}
	return content, nil
}

// readInputSource reads content from stdin or file.
// If source is "" or "-", reads from stdin; otherwise reads the file at the given path.
func readInputSource(source string) (string, error) {
	var b []byte
	var err error
	if source == "" || source == "-" {
		b, err = io.ReadAll(bufio.NewReader(os.Stdin))
	} else {
		b, err = os.ReadFile(source)
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// gatherInput reads input from args or --input flag without blocking on stdin.
// Returns content, whether stdin read is still needed, and any error.
// When needStdin=true, the caller must read from os.Stdin to get the content.
func gatherInput(cmd *cobra.Command, args []string) (content string, needStdin bool, err error) {
	if len(args) > 0 {
		return strings.Join(args, " "), false, nil
	}
	input, err := cmd.Flags().GetString("input")
	if err != nil {
		return "", false, err
	}
	if input == "" || input == "-" {
		return "", true, nil // caller must read stdin
	}
	// Read from file
	b, err := os.ReadFile(input)
	if err != nil {
		return "", false, fmt.Errorf("读取输入文件 %s 失败: %w", input, err)
	}
	return strings.TrimSpace(string(b)), false, nil
}

// isTerminal reports whether the given file descriptor is a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
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
	// 子进程（code_review / ask_expert）不显示会话统计，
	// 避免余额等信息泄露到审查/专家输出中。
	if context.ContextValue(ctx, context.IsChildProcessKey, false) {
		return
	}

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

	// token 用量
	if u := price.GetUsage(); u.TotalTokens > 0 {
		var cacheRatio string
		if total := u.PromptCacheHitTokens + u.PromptCacheMissTokens; total > 0 {
			ratio := float64(u.PromptCacheHitTokens) / float64(total) * 100
			cacheRatio = fmt.Sprintf(" %.0f%%", ratio)
		}
		stats = append(stats, fmt.Sprintf("🪙 %d %d%s",
			u.TotalTokens, u.CompletionTokens, cacheRatio))
	}

	// 花费和余额 (only when user-balance is enabled)
	if config.GetBool("user-balance", true) && startBalance["currency"] != "" {
		if resp, err := DeepseekClient.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
			for _, balance := range resp.BalanceInfos {
				if balance["currency"] == startBalance["currency"] {
					// 使用 token 用量计算花费（替代旧的余额差值算法）
					model := context.ContextValue(ctx, context.CurrentModelNameKey, "")
					cost := ""
					if model != "" {
						if c := price.GetCost(model); c > 0 {
							cost = fmt.Sprintf("%s %.2f", startBalance["currency"], c)
						}
					}

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
					if currentBalance < 10.0 {
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
// injectChimein checks for pending chime-in messages and injects them into history.
// Returns the updated history and true if a chime-in was found and injected.
func injectChimein(ctx context.Context, history []prompt.Message) ([]prompt.Message, bool) {
	content, err := chimein.Get(ctx)
	hasChimein := err == nil && content != ""

	// Check unread mail regardless of chimein presence — the AI
	// must not miss mail just because no chimein arrived.
	var notif string
	if summaries := mail.UnreadMailList(ctx); len(summaries) > 0 {
		notif = mail.FormatUnreadMailNotification(summaries)
	}

	// Nothing to inject.
	if !hasChimein && notif == "" {
		return history, false
	}

	// Build the combined message content.
	var msgContent string
	switch {
	case hasChimein && notif != "":
		msgContent = notif + "\n\n---\n\n" + content
	case hasChimein:
		msgContent = content
	default: // only notif
		msgContent = notif
	}

	msg := prompt.Message{Role: "user", Content: msgContent}
	history = append(history, msg)
	outfmt.PrintClimeinContent(ctx, msgContent)
	if saveErr := prompt.SaveMessages(ctx, msg); saveErr != nil {
		outfmt.Debug("failed to save chimein message: %v", saveErr)
	}
	// Only signal "restart needed" for actual chimeins, not for
	// unread-mail-only notifications. Without this guard, a persistent
	// unread mail would cause infinite ChatRound restarts at line 506.
	return history, hasChimein
}


func ChatRound(ctx context.Context, prompts, history []prompt.Message, inputs ...prompt.Message) (err error) {
	// 0. Inject any pending chime-in before calling LLM.
	//    This catches leftover chime-ins from previous sessions that ended
	//    with a text-only response (no tool calls to trigger the check below).
	history, _ = injectChimein(ctx, history)

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
		// No tool calls — check for chime-in before ending the session.
		// If a chime-in arrived during LLM thinking, restart ChatRound
		// so it gets processed immediately instead of being carried over
		// to the next dscli chat invocation.
		if updatedHistory, injected := injectChimein(ctx, history); injected {
			return ChatRound(ctx, prompts, updatedHistory)
		}
		// Conversation ended, print stats
		PrintSessionStats(ctx)
		return err
	}

	toolInputs := toolcall.HandleToolCalls(ctx, tcs)
	if len(toolInputs) > 0 {
		// Tool call inputs saved in db, move them to history
		history = append(history, toolInputs...)

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
