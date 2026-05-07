package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/dsc"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/skills"
	"gitcode.com/dscli/dscli/internal/toolcall"
	"gitcode.com/dscli/dscli/internal/toolcall/alltools"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

// ─── Update ─────────────────────────────────────────────────────────────────
// ─── Update ─────────────────────────────────────────────────────────────────

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// Global quit
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		// If chat input is focused, let it handle keys
		if m.Screen == ScreenChat && m.ChatInput.Focused() && !m.ChatLoading {
			return m.handleChatInputKeys(msg)
		}
		return m.handleKeyPress(msg.String())

	// ─── Data loaded messages ──────────────────────────────────────────
	case balanceMsg:
		if msg.err != nil {
			m.ErrorMsg = msg.err.Error()
			return m, nil
		}
		m.BalanceInfos = msg.resp.BalanceInfos
		m.IsAvailable = msg.resp.IsAvailable
		return m, nil

	case modelsMsg:
		if msg.err != nil {
			m.ErrorMsg = msg.err.Error()
			return m, nil
		}
		m.ModelList = msg.resp.Data
		return m, nil

	case historyMsg:
		if msg.err != nil {
			m.ErrorMsg = msg.err.Error()
			return m, nil
		}
		m.HistoryMessages = msg.messages
		// Scroll to bottom so newest messages are visible first.
		if count := len(m.HistoryMessages); count > 0 {
			visibleItems := m.visibleItems(2)
			if count > visibleItems {
				m.Scroll = count - visibleItems
			}
			m.Cursor = count - 1
		}
		return m, nil

	case fetchHistoryDetailMsg:
		if msg.err != nil {
			m.ErrorMsg = msg.err.Error()
			return m, nil
		}
		m.HistoryDetail = msg.message
		return m, nil

	case skillsMsg:
		if msg.err != nil {
			m.ErrorMsg = msg.err.Error()
			return m, nil
		}
		m.SkillInfos = msg.infos
		return m, nil

	case promptContentMsg:
		if msg.err != nil {
			m.ErrorMsg = msg.err.Error()
			return m, nil
		}
		m.PromptContent = msg.content
		return m, nil

	case chatStreamMsg:
		if msg.err != nil {
			m.ChatLoading = false
			m.ChatStream = nil
			m.ErrorMsg = msg.err.Error()
			return m, nil
		}
		// Append or replace incremental response lines.
		// replaceLast enables streaming content: each chunk replaces the
		// previous partial assistant message so the UI sees content grow.
		if msg.replaceLast && len(m.ChatMessages) > 0 && len(msg.lines) == 1 {
			m.ChatMessages[len(m.ChatMessages)-1] = msg.lines[0]
		} else {
			m.ChatMessages = append(m.ChatMessages, msg.lines...)
		}
		// Auto-scroll to bottom on new content
		m.ChatScroll = 0
		if msg.done {
			// Stream completed successfully
			m.ChatLoading = false
			m.ChatStream = nil
			return m, nil
		}
		// Continue streaming: keep spinner ticking and listen for next batch
		return m, tea.Batch(
			m.ChatSpinner.Tick,
			waitForChatStream(m.ChatStream),
		)

	case spinner.TickMsg:
		if m.ChatLoading {
			var cmd tea.Cmd
			m.ChatSpinner, cmd = m.ChatSpinner.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	return m, nil
}

// ─── Key Press Router ──────────────────────────────────────────────────────

func (m Model) handleKeyPress(key string) (tea.Model, tea.Cmd) {
	m.ErrorMsg = ""

	switch m.Screen {
	case ScreenDashboard:
		return m.handleDashboardKeys(key)
	case ScreenBalance:
		return m.handleSimpleScreenKeys(key)
	case ScreenModels:
		return m.handleListScreenKeys(key, len(m.ModelList), m.visibleItems(1))
	case ScreenHistory:
		if key == "enter" && len(m.HistoryMessages) > 0 {
			msg := m.HistoryMessages[m.Cursor]
			m.HistoryDetail = nil
			m.Screen = ScreenHistoryDetail
			m.Scroll = 0
			return m, fetchHistoryDetail(msg.ID)
		}
		return m.handleListScreenKeys(key, len(m.HistoryMessages), m.visibleItems(2))
	case ScreenHistoryDetail:
		return m.handleHistoryDetailKeys(key)
	case ScreenSkills:
		return m.handleListScreenKeys(key, len(m.SkillInfos), m.visibleItems(1))
	case ScreenPrompt:
		return m.handlePromptKeys(key)
	case ScreenChat:
		return m.handleChatKeys(key)
	}
	return m, nil
}

// ─── Dashboard ─────────────────────────────────────────────────────────────

var dashboardMenuItems = []string{
	"💰 Balance",
	"🤖 Models",
	"📜 History",
	"🔧 Skills",
	"📝 Prompt",
	"💬 Chat",
	"🚪 Quit",
}

func (m Model) handleDashboardKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down", "j":
		if m.Cursor < len(dashboardMenuItems)-1 {
			m.Cursor++
		}
	case "enter", " ":
		return m.handleDashboardSelection()
	case "q":
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) handleDashboardSelection() (tea.Model, tea.Cmd) {
	switch m.Cursor {
	case 0: // Balance
		m.Screen = ScreenBalance
		return m, fetchBalance(m.client)
	case 1: // Models
		m.Screen = ScreenModels
		return m, fetchModels(m.client)
	case 2: // History
		m.Screen = ScreenHistory
		m.Cursor = 0
		m.Scroll = 0
		return m, fetchHistory()
	case 3: // Skills
		m.Screen = ScreenSkills
		m.Cursor = 0
		m.Scroll = 0
		return m, fetchSkills()
	case 4: // Prompt
		m.Screen = ScreenPrompt
		m.Scroll = 0
		return m, fetchPromptContent(m.ctx, "chat")
	case 5: // Chat
		m.PrevScreen = ScreenDashboard
		m.Screen = ScreenChat
		m.Cursor = 0
		m.ChatInput.SetValue("")
		m.ChatInput.Focus()
		m.ChatMessages = nil
		m.ChatLoading = false
		m.ChatScroll = 0
		m.ChatStream = nil
		return m, nil
	case 6: // Quit
		return m, tea.Quit
	}
	return m, nil
}

// ─── Simple Screens (balance) ──────────────────────────────────────────────

func (m Model) handleSimpleScreenKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q":
		m.Screen = ScreenDashboard
		m.Cursor = 0
		return m, nil
	}
	return m, nil
}

// ─── List Screens (models, history, skills) ────────────────────────────────

func (m Model) handleListScreenKeys(key string, itemCount int, visibleItems int) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.Cursor > 0 {
			m.Cursor--
			if m.Cursor < m.Scroll {
				m.Scroll = m.Cursor
			}
		}
	case "down", "j":
		if m.Cursor < itemCount-1 {
			m.Cursor++
			if m.Cursor >= m.Scroll+visibleItems {
				m.Scroll = m.Cursor - visibleItems + 1
			}
		}
	case "esc", "q":
		m.Screen = ScreenDashboard
		m.Cursor = 0
		m.Scroll = 0
		return m, nil
	}
	return m, nil
}

// visibleItems returns the number of items that fit in the terminal for a
// list view where each item occupies linesPerItem display lines.
//
// History items take 2 lines (main + preview), Models/Skills take 1 line.
// The overhead (header, indicators, help) is estimated at 9 lines for
// 1-line items and 5 lines for 2-line items (▲/▼ indicators).
// ─── Visible items calculation ──────────────────────────────────────────────

// List overheads — fixed display rows consumed by non-item elements.
// These are used by visibleItems to compute how many list items fit.
const (
	// overheadOneLine is for 1-line-per-item lists (Models, Skills).
	// Layout: header(1) + pagination "X-Y of Z"(1) + help(2) + margin ≈ 9.
	// Using a safe overestimate prevents accidental overflow on small terminals.
	overheadOneLine = 9

	// overheadTwoLine is for 2-line-per-item lists (History: main + preview).
	// Layout: header(1) + ▲ indicator(1) + ▼ indicator(1) + help(2) = 5.
	overheadTwoLine = 5
)

// visibleItems returns the number of items that fit in the terminal for a
// list view where each item occupies linesPerItem display lines.
//
// linesPerItem must be >= 1.  Overhead is chosen automatically:
//   - 1 → overheadOneLine (9)
//   - 2 → overheadTwoLine (5)
//
// Result is always at least 3 (legacy minimum; very small terminals may
// still overflow but Height < 10 is rare in practice).
func (m Model) visibleItems(linesPerItem int) int {
	if linesPerItem < 1 {
		linesPerItem = 1
	}

	var overhead int
	switch linesPerItem {
	case 2:
		overhead = overheadTwoLine
	default:
		overhead = overheadOneLine
	}

	n := (m.Height - overhead) / linesPerItem
	if n < 3 {
		n = 3
	}
	return n
}

// ─── History Detail ─────────────────────────────────────────────────────────

func (m Model) handleHistoryDetailKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc", "q", "left":
		m.Screen = ScreenHistory
		m.HistoryDetail = nil
		// Scroll to bottom so newest messages are visible.
		if count := len(m.HistoryMessages); count > 0 {
			visibleItems := m.visibleItems(2)
			if count > visibleItems {
				m.Scroll = count - visibleItems
			}
			m.Cursor = count - 1
		} else {
			m.Scroll = 0
			m.Cursor = 0
		}
		return m, nil
	case "up", "k":
		if m.Scroll > 0 {
			m.Scroll--
		}
	case "down", "j":
		// The view function does precise clamping; this is a rough guard.
		maxEst := 0
		if m.HistoryDetail != nil && m.Height > 0 {
			wrapWidth := m.Width - 6
			if wrapWidth < 20 {
				wrapWidth = 20
			}
			// Rough: ~1 line per wrapWidth chars + 15 lines overhead (fields/headings)
			estLines := (len(m.HistoryDetail.Content)+len(m.HistoryDetail.ReasoningContent))/wrapWidth + 15
			maxEst = estLines - (m.Height - 4) // 4 fixed rows
			if maxEst < 0 {
				maxEst = 0
			}
		}
		if m.Scroll < maxEst || maxEst == 0 {
			m.Scroll++
		}
	}
	return m, nil
}

// ─── Prompt ────────────────────────────────────────────────────────────────

func (m Model) handlePromptKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "up", "k":
		if m.Scroll > 0 {
			m.Scroll--
		}
	case "down", "j":
		m.Scroll++
	case "esc", "q":
		m.Screen = ScreenDashboard
		m.Scroll = 0
	}
	return m, nil
}

// ─── Chat ──────────────────────────────────────────────────────────────────

func (m Model) handleChatInputKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		input := m.ChatInput.Value()
		if input != "" {
			m.ChatInput.SetValue("")
			m.ChatLoading = true
			// 立即在 UI 中显示用户消息，避免 DB 操作导致的延迟
			m.ChatMessages = append(m.ChatMessages, chatLine{Role: "user", Content: input})
			m.ChatScroll = 0
			// Create channel for streaming incremental chat responses
			ch := make(chan chatStreamMsg, 10)
			m.ChatStream = ch
			return m, tea.Batch(
				m.ChatSpinner.Tick,
				startChatStream(m.ctx, m.client, input, ch),
			)
		}
		return m, nil
	case "esc":
		m.ChatInput.Blur()
		return m, nil
	}

	var cmd tea.Cmd
	m.ChatInput, cmd = m.ChatInput.Update(msg)
	return m, cmd
}

func (m Model) handleChatKeys(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "i", "/":
		m.ChatInput.Focus()
		return m, nil
	case "up", "k":
		// Scroll up one display line.
		m.ChatScroll++
		m.clampChatScroll()
		return m, nil
	case "down", "j":
		// Scroll down one display line.
		if m.ChatScroll > 0 {
			m.ChatScroll--
		}
		m.clampChatScroll()
		return m, nil
	case "pgup":
		// Page up: scroll up by ~half the chat area.
		page := (m.Height - 8)
		if page < 1 {
			page = 1
		}
		m.ChatScroll += page
		m.clampChatScroll()
		return m, nil
	case "pgdown":
		// Page down: scroll down by ~half the chat area.
		page := (m.Height - 8)
		if page < 1 {
			page = 1
		}
		m.ChatScroll -= page
		if m.ChatScroll < 0 {
			m.ChatScroll = 0
		}
		m.clampChatScroll()
		return m, nil
	case "g":
		// Scroll to top (vim-style).  Set a large value; viewChat() clamps
		// it to the actual maximum based on total lines.
		m.ChatScroll = 1_000_000
		m.clampChatScroll()
		return m, nil
	case "G":
		// Scroll to bottom (vim-style).
		m.ChatScroll = 0
		return m, nil
	case "esc", "q":
		m.ChatInput.Blur()
		m.Screen = ScreenDashboard
		m.Cursor = 0
		m.ChatMessages = nil
		m.ChatLoading = false
		m.ChatScroll = 0
		m.ChatStream = nil
		return m, nil
	}
	return m, nil
}

// clampChatScroll tightens ChatScroll to a reasonable maximum so that
// 'j' (scroll-down) is immediately responsive after 'g' (scroll-to-top)
// sentinel, rather than requiring thousands of keypresses to burn down
// the sentinel value.
func (m *Model) clampChatScroll() {
	if lastChatMaxScroll > 0 && m.ChatScroll > lastChatMaxScroll {
		m.ChatScroll = lastChatMaxScroll
	}
}

// ─── Commands (data loading) ───────────────────────────────────────────────

func fetchBalance(client dsc.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Balance()
		return balanceMsg{resp: resp, err: err}
	}
}

func fetchModels(client dsc.Client) tea.Cmd {
	return func() tea.Msg {
		resp, err := client.Models()
		return modelsMsg{resp: resp, err: err}
	}
}

func fetchHistory() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		ctx = context.WithValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
		ctx = context.WithValue(ctx, context.CurrentModelNameKey, context.ModelDeepseekChat)
		ctx = context.WithValue(ctx, context.HistSizeKey, 1000)
		messages, err := prompt.ListHistory(ctx)
		return historyMsg{messages: messages, err: err}
	}
}

func fetchHistoryDetail(id int64) tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		msg, err := prompt.ShowMessage(ctx, id)
		return fetchHistoryDetailMsg{message: msg, err: err}
	}
}

func fetchSkills() tea.Cmd {
	return func() tea.Msg {
		infos, err := skills.ListAll()
		return skillsMsg{infos: infos, err: err}
	}
}

func fetchPromptContent(ctx context.Context, model string) tea.Cmd {
	return func() tea.Msg {
		content := prompt.GetPromptTemplate(ctx, model)
		return promptContentMsg{content: content, err: nil}
	}
}

func getToolCallNames(tools []prompt.ToolCall) []string {
	toolNames := make([]string, 0, len(tools))
	for _, tc := range tools {
		toolNames = append(toolNames, tc.Function.Name)
	}
	return toolNames
}

// startChatStream 在goroutine中启动聊天工具调用循环，逐步将结果通过channel流式传回。
// 每次LLM响应和工具执行结果作为独立的chatStreamMsg发送，让UI实时更新。
// 本函数与 chat.go 的 ChatRunE+ChatRound 逻辑保持一致：
// 加载系统提示词 → 加载跨会话历史 → 处理未完成工具调用 → 工具调用循环 → 保存所有记录到DB。
func startChatStream(ctx context.Context, client dsc.Client, userInput string, ch chan chatStreamMsg) tea.Cmd {
	ctx = context.WithValue(ctx, context.StartTimeKey, time.Now())
	go func() {
		defer close(ch)
		var startBalance map[string]string
		if resp, err := client.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
			startBalance = resp.BalanceInfos[0]
			ctx = context.WithValue(ctx, context.StartBalanceKey, startBalance)
		}
		// ── 1. 加载系统提示词（等同 chatCommonPreRunE） ──────────────
		prompts, err := prompt.LoadPrompts(ctx)
		if err != nil {
			prompts = []prompt.Message{{Role: "system", Content: prompt.GetSystemPrompt(ctx)}}
		}

		// ── 2. 计算剩余token预算（等同 chatCommonPreRunE） ───────────
		tools := alltools.GetAllTools(ctx)
		tokens := 0
		for _, tool := range tools {
			tokens += tool.GetTokens()
		}
		for _, p := range prompts {
			tokens += p.GetTokens()
		}
		ctx = context.WithValue(ctx, context.LeftTokensKey, 131072-tokens)

		// ── 3. 初始化会话（必须在LoadHistory之前） ──────────────────
		_ = session.GetCurrentSessionID(ctx)

		// ── 4. 保存用户消息到DB ─────────────────────────────────────
		userMsg := prompt.Message{Role: "user", Content: userInput}
		if saveErr := prompt.SaveMessages(ctx, userMsg); saveErr != nil {
			outfmt.Debug("保存用户消息失败: %v", saveErr)
		}

		// ── 5. 用户消息已在 handleChatInputKeys 中立即显示，此处不再发送 ──

		// ── 6. 加载跨会话历史（等同 ChatRunE） ──────────────────────
		history, err := prompt.LoadHistory(ctx)
		if err != nil {
			outfmt.Debug("加载历史失败: %v", err)
		}

		// ── 7. 处理历史上未完成的工具调用（等同 ChatRunE） ──────────
		// 加载的历史中可能不包含刚刚保存的userMsg（取决于查询条件），
		// 即使包含，后续构建messages时也只追加userMsg一次。
		if len(history) > 0 {
			lastHist := history[len(history)-1]
			if lastHist.Role == "assistant" && len(lastHist.ToolCalls) > 0 {
				// 生成待显示的pending行
				var pendingLines []chatLine
				cl := chatLine{
					Role:             lastHist.Role,
					Content:          lastHist.Content,
					ReasoningContent: lastHist.ReasoningContent,
					HasToolCalls:     true,
					ToolCallNames:    getToolCallNames(lastHist.ToolCalls),
				}
				pendingLines = append(pendingLines, cl)

				// 执行工具调用（HandleToolCalls 内部会保存到 DB）
				toolInputs := toolcall.HandleToolCalls(ctx, lastHist.ToolCalls)
				pendingLines = append(pendingLines, buildToolChatLines(toolInputs, lastHist.ToolCalls)...)
				history = append(history, toolInputs...)

				if len(pendingLines) > 0 {
					ch <- chatStreamMsg{lines: pendingLines}
				}
			}
		}

		// ── 8. 构建消息数组：prompts → history → userMsg ────────────
		messages := make([]prompt.Message, 0, len(prompts)+len(history)+1)
		messages = append(messages, prompts...)
		messages = append(messages, history...)
		messages = append(messages, userMsg)

		// ── 9. 工具调用循环（使用流式API） ──────────────────────────
		for {
			// 创建流式通道并启动读取goroutine
			streamCh := make(chan dsc.StreamChunk, 100)
			var fullContent, fullReasoning strings.Builder
			streamDone := make(chan struct{})

			go func() {
				defer close(streamDone)
				// 先发送空的助手消息占位，让 UI 立即显示气泡
				ch <- chatStreamMsg{lines: []chatLine{{Role: "assistant", Content: ""}}}
				for chunk := range streamCh {
					if chunk.Done {
						return
					}
					if chunk.Content != "" {
						fullContent.WriteString(chunk.Content)
					}
					if chunk.ReasoningContent != "" {
						fullReasoning.WriteString(chunk.ReasoningContent)
					}
					// 发送增量更新，替换上一条助手消息
					ch <- chatStreamMsg{
						lines:       []chatLine{{Role: "assistant", Content: fullContent.String(), ReasoningContent: fullReasoning.String()}},
						replaceLast: true,
					}
				}
			}()

			// 调用流式API（阻塞直到流结束）
			resp, err := client.ChatStreamChan(ctx, messages, tools, streamCh)
			close(streamCh) // 通知读取goroutine停止
			<-streamDone    // 等待读取goroutine结束

			if err != nil {
				ch <- chatStreamMsg{err: err}
				return
			}

			if len(resp.Choices) == 0 {
				ch <- chatStreamMsg{err: fmt.Errorf("no response received")}
				return
			}

			choice := resp.Choices[0]
			story := choice.Message

			// 处理截断标志
			if choice.FinishReason == "length" {
				ctx = context.WithValue(ctx, context.FinishReasonLengthKey, true)
			} else {
				if context.ContextValue(ctx, context.FinishReasonLengthKey, false) {
					ctx = context.WithValue(ctx, context.FinishReasonLengthKey, false)
				}
			}

			// 发送最终完整的助手消息（包含工具调用信息），替换流式占位
			finalCL := chatLine{
				Role:             story.Role,
				Content:          story.Content,
				ReasoningContent: story.ReasoningContent,
				FinishReason:     choice.FinishReason,
				HasToolCalls:     len(story.ToolCalls) > 0,
			}
			if finalCL.HasToolCalls {
				finalCL.ToolCallNames = getToolCallNames(story.ToolCalls)
			}
			ch <- chatStreamMsg{
				lines:       []chatLine{finalCL},
				replaceLast: true,
			}

			// 保存助手消息到DB（等同 ChatRound）
			if saveErr := prompt.SaveMessages(ctx, story); saveErr != nil {
				outfmt.Debug("保存助手消息失败: %v", saveErr)
			}

			// 无工具调用 → 对话结束
			if len(story.ToolCalls) == 0 {
				cline := chatLine{
					Role:    "state",
					Content: sessionStats(ctx, client),
				}
				ch <- chatStreamMsg{done: true, lines: []chatLine{cline}}
				return
			}

			// 执行工具调用（HandleToolCalls 内部已保存到 DB，无需重复保存）
			toolInputs := toolcall.HandleToolCalls(ctx, story.ToolCalls)
			toolLines := buildToolChatLines(toolInputs, story.ToolCalls)
			if len(toolLines) > 0 {
				ch <- chatStreamMsg{lines: toolLines}
			}

			// 将助手消息和工具结果追加到messages，供下一轮使用
			messages = append(messages, story)
			messages = append(messages, toolInputs...)
		}

		// 超出最大轮次
		cline := chatLine{
			Role:    "state",
			Content: sessionStats(ctx, client),
		}
		ch <- chatStreamMsg{done: true, lines: []chatLine{cline}}
	}()

	// 返回从channel读取第一条消息的命令
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return chatStreamMsg{done: true}
		}
		return msg
	}
}

func sessionStats(ctx context.Context, client dsc.Client) string {
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
		if resp, err := client.Balance(); err == nil && len(resp.BalanceInfos) > 0 {
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
		return strings.Join(stats, "  ")
	}
	return ""
}

// parseBalance 解析余额字符串
func parseBalance(balanceStr string) (float64, error) {
	// 移除货币符号和空格
	balanceStr = strings.TrimSpace(balanceStr)
	// 尝试解析为浮点数
	return strconv.ParseFloat(balanceStr, 64)
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

// buildToolChatLines 从工具调用结果构建 Chat 显示行。
// toolInputs 是 HandleToolCalls 返回的 prompt.Message（Role="tool"，ToolCalls 为空），
// sourceToolCalls 是本次调用的原始 ToolCall 定义（用于 ID→名称 映射）。
func buildToolChatLines(toolInputs []prompt.Message, sourceToolCalls []prompt.ToolCall) []chatLine {
	toolNameMap := make(map[string]string, len(sourceToolCalls))
	for _, tc := range sourceToolCalls {
		toolNameMap[tc.ID] = tc.Function.Name
	}
	lines := make([]chatLine, 0, len(toolInputs))
	for _, ti := range toolInputs {
		toolName := toolNameMap[ti.ToolCallID]
		switch {
		case toolName != "":
			// 正常：通过 ID 找到名称
		case ti.ToolCallID != "":
			toolName = "未知工具(" + ti.ToolCallID + ")" // fallback
		default:
			toolName = "未知工具" // ToolCallID 为空
		}
		lines = append(lines, chatLine{
			Role:    ti.Role,
			Content: fmt.Sprintf("%s: %s", toolName, truncateStr(ti.Content, 200)),
		})
	}
	return lines
}

func waitForChatStream(ch chan chatStreamMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return chatStreamMsg{done: true}
		}
		return msg
	}
}
