package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/ainame"
	"github.com/dscli/dscli/internal/chimein"
	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/lockfile"
	"github.com/dscli/dscli/internal/mail"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/price"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/dscli/dscli/internal/toolcall/alltools"
	"github.com/nanjj/clog"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"golang.org/x/text/unicode/norm"
)

const (
	DeepseekChat = int64(0)
)

func ChatPreRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()

	// --model 覆盖配置的默认模型（如切换到视觉模型 deepseek-v4-flash-vision-exp）
	modelName := context.ModelDeepseekChat
	if m, flagErr := cmd.Flags().GetString("model"); flagErr == nil && m != "" {
		modelName = m
	}
	ctx = context.WithValue(ctx, context.CurrentModelNameKey, modelName)
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, DeepseekChat)

	// Read --role flag and store in context
	role, err := cmd.Flags().GetString("role")
	if err != nil {
		return err
	}

	if role == "" {
		role = "dev"
	}

	ctx = context.WithValue(ctx, context.CurrentRoleKey, role)

	// Read context-window from config (default 1,000,000, matching DeepSeek V4 million-token context)
	// This value is used as the upper limit for history message token budget;
	// actual truncation is mainly controlled by --histsize.
	// Config key: context-window, env var: CONTEXT_WINDOW.
	contextWindow := config.GetInt("context-window", 1000000)
	ctx = context.WithValue(ctx, context.LeftTokensKey, contextWindow)

	// Get stream flag
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

// readChimein reads pending chimein from the database, acknowledges it,
// and prints it to the terminal. Returns the chimein content, or ""
// if none is available. Callers use the return value to decide how to inject
// the chimein into the next API request.
//
// Chimein timing is critical: it MUST be called after any blocking operation
// (especially HandleToolCalls) so that chimein typed by the user during tool
// execution is not missed. The three call sites are:
//
//   - ChatRunE Scene A/C (tool-call path): after HandleToolCalls
//   - ChatRunE Scene B (no-tool-call path): before ChatRound (no blocking op)
//   - ChatRound recursion (multi-round tool chain): after HandleToolCalls
func readChimein(ctx context.Context) string {
	span, ctx := clog.StartSpanFromContext(ctx, "readChimein")
	defer span.Finish()
	c, err := chimein.Get(ctx)
	if err != nil || c == "" {
		return ""
	}
	outfmt.PrintClimeinContent(ctx, c)
	return c
}

func ChatRunE(cmd *cobra.Command, args []string) (err error) {
	ctx := cmd.Context()
	span, ctx := clog.StartSpanFromContext(ctx, "ChatRunE")
	defer span.Finish()

	// 1. 优先从非阻塞来源读取内容（args 或 --input 文件路径）。
	//    此时不读取 stdin，避免在没有主进程时因无输入而超时出错。
	content, needStdin, err := gatherInput(cmd, args)
	if err != nil {
		return err
	}
	// --attach 图片上传：仅视觉模型支持；失败直接报错而非静默忽略。
	attachBlocks, err := uploadAttachments(ctx, cmd)
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
		if len(attachBlocks) > 0 {
			return fmt.Errorf("已有主 chat 进程运行中，--attach 不支持插话模式")
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
	cfg := ainame.LoadOrAssign(ctx, sessionID)
	ctx = context.WithValue(ctx, context.AINameCNKey, cfg.NameCN)
	ctx = context.WithValue(ctx, context.AINameENKey, cfg.NameEN)
	ctx = context.WithValue(ctx, context.UserIDKey, fmt.Sprintf("%s-%d",
		slugify(cfg.NameEN), sessionID))
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

	// Inject unread mail notification as a single line at the top of
	// the user message. Unlike system prompt injection, this doesn't
	// affect cache stability — user content varies per-message anyway.
	if summaries := mail.UnreadMailList(ctx); len(summaries) > 0 {
		if notif := mail.FormatUnreadMailLine(summaries); notif != "" {
			if content != "" {
				content = notif + "\n" + content
			} else {
				content = notif
			}
		}
	}

	ctx = context.WithValue(ctx, context.StartTimeKey, time.Now())

	// Fetch starting balance (only when user-balance is enabled)
	var startBalance map[string]string
	if config.GetBool("user-balance", true) {
		if resp, err := DeepseekClient.Balance(ctx); err == nil && len(resp.BalanceInfos) > 0 {
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
			// 上次进程中断遗留的工具调用，自动恢复执行。用户主动停止
			// （Ctrl+C/kill）会在退出前标记，不会走到这里；SIGKILL 等
			// 不可捕获信号仍会重放，提示用户可 Ctrl+C 中断。
			outfmt.Warn("⚠️ 检测到 %d 个上次中断遗留的工具调用，正在恢复执行（Ctrl+C 可中断）", len(tcs))
			// Print reasoning content or content
			outfmt.PrintContent(ctx, lastHist.ReasoningContent, lastHist.Content, 0, 0)
			toolInputs := toolcall.HandleToolCalls(ctx, tcs)

			// Scene A/C: Execute tools first, THEN read chimein. This ensures
			// any chimein typed during tool execution (e.g. "stop, wrong file!")
			// is captured. Chimein is prepended to the user's original content
			// so it reads as a refinement of the same turn.
			if c := readChimein(ctx); c != "" {
				if content != "" {
					content = c + "\n" + content
				} else {
					content = c
				}
			}

			// Append tool results to history
			history = append(history, toolInputs...)

			inputs := []prompt.Message{}
			if content != "" {
				inputs = append(inputs, userMessageWithAttach(content, attachBlocks))
			}

			return ChatRound(ctx, prompts, history, inputs...)
		}
	}
	// Scene B: No tool calls — no blocking op, read chimein immediately.
	if c := readChimein(ctx); c != "" {
		if content != "" {
			content = c + "\n" + content
		} else {
			content = c
		}
	}

	return ChatRound(ctx, prompts, history,
		userMessageWithAttach(content, attachBlocks))
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

// uploadAttachments 上传 --attach 指定的图片文件，返回 file 内容块。
// 仅视觉模型支持（其他模型 API 会以 400 拒绝图片）。
// 上传失败直接报错，避免静默忽略导致用户误以为图片已发送。
func uploadAttachments(ctx context.Context, cmd *cobra.Command) ([]prompt.ContentBlock, error) {
	attach, err := cmd.Flags().GetStringSlice("attach")
	if err != nil || len(attach) == 0 {
		return nil, nil
	}
	model := context.ContextValue(ctx, context.CurrentModelNameKey, "")
	if !prompt.IsVisionModel(model) {
		return nil, fmt.Errorf("模型 %q 不支持图片输入；--attach 仅支持视觉模型（如 deepseek-v4-flash-vision-exp）", model)
	}
	key := config.Get("deepseek-api-key", "")
	url := config.Get("deepseek-base-url", "https://api.deepseek.com")
	api := dsc.NewFilesAPI(key, url)
	blocks := make([]prompt.ContentBlock, 0, len(attach))
	for _, p := range attach {
		file, err := api.Upload(ctx, p, dsc.UploadOptions{})
		if err != nil {
			return nil, fmt.Errorf("--attach %s: %w", p, err)
		}
		outfmt.Debug("attached %s -> %s\n", p, file.ID)
		blocks = append(blocks, prompt.FileBlock(file.ID))
	}
	return blocks, nil
}

// userMessageWithAttach 构造用户消息：有附件时 content 序列化为块数组
// （text 块在前、file 块在后），无附件时保持纯字符串。
func userMessageWithAttach(content string, attach []prompt.ContentBlock) prompt.Message {
	if len(attach) == 0 {
		return prompt.Message{Role: "user", Content: content}
	}
	msg := prompt.Message{Role: "user", Content: content, ContentBlocks: attach}
	if content != "" {
		msg.ContentBlocks = append([]prompt.ContentBlock{prompt.TextBlock(content)}, attach...)
	}
	return msg
}

// cleanMessagesForModel 按模型能力清洗消息：非视觉模型发送前剥离
// 图片内容块（保留纯文本），防止历史中的图片块导致 API 400。
func cleanMessagesForModel(messages []prompt.Message, model string) {
	if prompt.IsVisionModel(model) {
		return
	}
	for i := range messages {
		if len(messages[i].ContentBlocks) > 0 {
			messages[i].ContentBlocks = nil
		}
	}
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

// slugify converts s to a DeepSeek user_id-safe string [a-zA-Z0-9].
// Uses NFD decomposition so accented Latin chars (ö, é, ü, ñ, etc.)
// decompose to their base character + combining mark, then the
// combining mark is dropped.
func slugify(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range norm.NFD.String(s) {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
		// Drop everything else (combining marks, spaces, punctuation)
	}
	return strings.ToLower(b.String())
}

// isTerminal reports whether the given file descriptor is a terminal.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
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
	span, ctx := clog.StartSpanFromContext(ctx, "PrintSessionStats")
	defer span.Finish()

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
		stats = append(stats, fmt.Sprintf("⏳ %s", durationStr))
	}

	// token 用量
	if u := price.GetUsage(); u.TotalTokens > 0 {
		var cacheRatio string
		if total := u.PromptCacheHitTokens + u.PromptCacheMissTokens; total > 0 {
			ratio := float64(u.PromptCacheHitTokens) / float64(total) * 100
			cacheRatio = fmt.Sprintf("%.0f%%", ratio)
		}
		reasoningTokens := 0
		if u.CompletionTokensDetails != nil {
			reasoningTokens = u.CompletionTokensDetails.ReasoningTokens
		}
		stats = append(stats, fmt.Sprintf("🪙 %d, %d(%d), %d(%s)",
			u.TotalTokens, u.CompletionTokens, reasoningTokens, u.PromptTokens, cacheRatio))
	}

	// 花费和余额 (only when user-balance is enabled)
	if config.GetBool("user-balance", true) && startBalance["currency"] != "" {
		if resp, err := DeepseekClient.Balance(ctx); err == nil && len(resp.BalanceInfos) > 0 {
			for _, balance := range resp.BalanceInfos {
				if balance["currency"] == startBalance["currency"] {
					// 计算花费（基于 token 用量）
					model := context.ContextValue(ctx, context.CurrentModelNameKey, "")
					var cost string
					var costVal float64
					if model != "" {
						costVal = price.GetCost(model)
						if costVal > 0 {
							cost = fmt.Sprintf("%s %.2f", startBalance["currency"], costVal)
						}
					}

					// 计算预估余额（开始余额 - 本次花费）
					startTotal, err := parseBalance(startBalance["total_balance"])
					var currentBalance float64
					var balanceDisplay string
					if err == nil && costVal > 0 {
						currentBalance = startTotal - costVal
						balanceDisplay = fmt.Sprintf("%s %.2f", startBalance["currency"], currentBalance)
					} else {
						// Fallback to API-returned balance
						currentBalance, _ = parseBalance(balance["total_balance"])
						balanceDisplay = fmt.Sprintf("%s %s", balance["currency"], balance["total_balance"])
					}

					// 花费
					if cost != "" {
						stats = append(stats, fmt.Sprintf("💰 %s", cost))
					}

					// 余额
					stats = append(stats, fmt.Sprintf("💳 %s", balanceDisplay))

					// 如果余额较低，显示提醒
					if currentBalance < 10.0 {
						stats = append(stats, "⚠️ Low balance, please recharge!")
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
	span, ctx := clog.StartSpanFromContext(ctx, "ChatRound")
	defer span.Finish()

	// 1. Construct messages slice (prompts → history → inputs)
	messages := make([]prompt.Message, 0, len(prompts)+len(history)+len(inputs))
	messages = append(messages, prompts...)
	messages = append(messages, history...)

	// 2. Add current user messages
	messages = append(messages, inputs...)

	// 非视觉模型：剥离历史中的图片块（避免 API 400），纯文本保留
	model := context.ContextValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
	cleanMessagesForModel(messages, model)

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
	// API 响应 ID 即会话 ID（与 internal/lp 的 WebChat 会话注册同源），
	// 随消息落库可供追溯/关联。resp.ID 为空时存 NULL（insertMessage 的
	// NullString 处理），不做流式路径那样的时间戳回退——非流式响应总有 ID。
	story.ConversationID = resp.ID
	// Check if response was truncated
	if resp.Choices[0].FinishReason == "length" {
		outfmt.Warn("note: response truncated due to length limit, may be incomplete.")
		ctx = context.WithValue(ctx, context.FinishReasonLengthKey, true)
	} else {
		if context.ContextValue(ctx, context.FinishReasonLengthKey, false) {
			ctx = context.WithValue(ctx, context.FinishReasonLengthKey, false)
		}
	}

	var thinkingTokens, contentTokens int
	if resp.Usage != nil {
		story.SetTokens(resp.Usage.CompletionTokens)
		if resp.Usage.CompletionTokensDetails != nil {
			thinkingTokens = resp.Usage.CompletionTokensDetails.ReasoningTokens
		}
		contentTokens = resp.Usage.CompletionTokens - thinkingTokens
	}

	outfmt.PrintContent(ctx, story.ReasoningContent, story.Content, thinkingTokens, contentTokens)
	// Print usage/cache stats for this round
	if resp.Usage != nil {
		u := resp.Usage
		cacheTotal := u.PromptCacheHitTokens + u.PromptCacheMissTokens
		var cacheRatio string
		if cacheTotal > 0 {
			ratio := float64(u.PromptCacheHitTokens) / float64(cacheTotal) * 100
			cacheRatio = fmt.Sprintf("%.0f%%", ratio)
		}
		reasoningTokens := 0
		if u.CompletionTokensDetails != nil {
			reasoningTokens = u.CompletionTokensDetails.ReasoningTokens
		}
		// Token line
		tokenLine := fmt.Sprintf("🪙 %d, %d(%d), %d(%s)",
			u.TotalTokens, u.CompletionTokens, reasoningTokens, u.PromptTokens, cacheRatio)

		// Cost for this round — convert yuan to fen (分) for readable small amounts
		model := context.ContextValue(ctx, context.CurrentModelNameKey, "")
		if model != "" {
			if cost := u.Cost(model); cost > 0 {
				tokenLine += fmt.Sprintf("  💰 %.2f 分", cost*100)
			}
		}

		outfmt.Println(tokenLine + "\n")
	}
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
		outfmt.Println()
		PrintSessionStats(ctx)
		return err
	}

	toolInputs := toolcall.HandleToolCalls(ctx, tcs)
	if len(toolInputs) > 0 {
		// Tool call inputs saved in db, move them to history
		history = append(history, toolInputs...)

		// Read chimein that arrived during tool execution. Unlike ChatRunE
		// (which prepends to existing user content), here there is no existing
		// user content — the chimein becomes a new user turn.
		roundInputs := []prompt.Message{}
		if c := readChimein(ctx); c != "" {
			roundInputs = append(roundInputs, prompt.Message{Role: "user", Content: c})
		}

		// 模型自主上传的图片：vision_file_read 的 handler 通过双消息协议
		// （DualMessage，见 internal/toolcall/dual.go）在 HandleToolCalls
		// 层已把 file_id 注入为附加 user 消息并落库/追加到 history，
		// 模型下一轮请求即可看到图片，无需在此重复注入。
		return ChatRound(ctx, prompts, history, roundInputs...)
	}
	return err
}

func init() {
	chatCmd := AddRootCommand(&cobra.Command{
		Use:   "chat",
		Short: "Chat with DeepSeek (supports tool calling: file ops, Git)",
		Long: `Send a message to the DeepSeek chat model and get a response.
Input is read from stdin. Conversation history is isolated per project directory.
Supports tool calling: file I/O, search, Git operations.

Image input (vision models, e.g. deepseek-v4-flash-vision-exp):
  dscli chat --model deepseek-v4-flash-vision-exp --attach screenshot.png "图中有什么？"
  Files are uploaded to the DeepSeek Files API and referenced by file_id
  (no base64 blobs in history). Vision file tools are also available to the
  model: vision_file_read / vision_file_list / vision_file_info / vision_file_delete.

Examples:
  echo "Create a main.go file" | dscli chat
  echo "Add README.md to Git and commit" | dscli chat
  cat prompt.txt | dscli chat`,
		PreRunE: ChatPreRunE,
		RunE:    ChatRunE,
	})
	chatCmd.Flags().String("role", "dev", "Role: dev (developer), expert (domain expert), review (code review), test (QA engineer), architect (software architect)")
	chatCmd.Flags().Int("histsize", 8, "history size loaded")
	chatCmd.Flags().String("input", "", "read content from input file or read content from stdin if input file empty")
	chatCmd.Flags().Bool("stream", false, "Enable streaming output (SSE)")
	chatCmd.Flags().String("model", "", "Override model (e.g. deepseek-v4-flash-vision-exp); default from config model-deepseek-chat")
	chatCmd.Flags().StringSlice("attach", nil, "Image file paths to upload and attach (vision models only)")
}
