package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/prompt"

	"github.com/charmbracelet/lipgloss"
)

// lastChatMaxScroll caches the maxScroll value computed by viewChat() so
// handleChatKeys() can clamp ChatScroll without knowing totalLines ahead
// of time.  Bubble Tea calls View() after every Update(), so the cache
// is always fresh when the next key event arrives.
var lastChatMaxScroll int

// ─── Logo ────────────────────────────────────────────────────────────────────

// renderLogo returns the dscli ASCII art logo with gradient colors,
// inspired by engram's design pattern.
// renderLogo returns the dscli ASCII art logo with gradient colors,
// inspired by engram's design pattern.
func renderLogo() string {
	// ASCII art: "DSCLI" in 5-row block letters (7-wide each, 39 chars total)
	logoLines := [5]string{
		"███████  ███████ ███████ ██    ██████",
		"██    ██ ██      ██      ██      ██   ",
		"██    ██ ███████ ██      ██      ██   ",
		"██    ██      ██ ██      ██      ██   ",
		"███████  ███████ ███████ █████ ██████",
	}

	// Gradient colors for the rows (purple → blue → cyan → teal → green)
	colors := []lipgloss.Color{
		colorMauve,   // Row 1 - purple
		colorPrimary, // Row 2 - blue
		colorBlue,    // Row 3 - cyan
		colorTeal,    // Row 4 - teal
		colorGreen,   // Row 5 - green
	}

	frameStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(colorOverlay).
		Padding(0, 1).
		MarginBottom(1)

	accentStyle := lipgloss.NewStyle().Foreground(colorMauve).Bold(true)
	taglineStyle := lipgloss.NewStyle().Foreground(colorSubtext).Italic(true)

	var b strings.Builder

	// Header line
	b.WriteString(accentStyle.Render(" 🐋 DSCLI TUI "))
	b.WriteString(strings.Repeat(" ", 16))
	b.WriteString(accentStyle.Render(" ONLINE "))
	b.WriteString("\n\n")

	// ASCII art with gradient
	for i, line := range logoLines {
		b.WriteString(" ")
		b.WriteString(lipgloss.NewStyle().Foreground(colors[i]).Bold(true).Render(line))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Tagline
	b.WriteString(taglineStyle.Render(" > dscli — DeepSeek CLI"))

	return frameStyle.Render(b.String()) + "\n"
}

// ─── View (main router) ────────────────────────────────────────────────────

func (m Model) View() string {
	var content string

	switch m.Screen {
	case ScreenDashboard:
		content = m.viewDashboard()
	case ScreenBalance:
		content = m.viewBalance()
	case ScreenModels:
		content = m.viewModels()
	case ScreenHistory:
		content = m.viewHistory()
	case ScreenHistoryDetail:
		content = m.viewHistoryDetail()
	case ScreenSkills:
		content = m.viewSkills()
	case ScreenPrompt:
		content = m.viewPrompt()
	case ScreenChat:
		// viewChat() handles its own 2-char margins internally via
		// alignment styles with Padding(0,2).  No outer lipgloss
		// wrapper needed — this avoids double-processing ANSI-rich
		// lines which caused alignment corruption (especially when
		// scrolling) and top-border clipping on first render.
		return m.viewChat() + "\n" + m.renderStatusBar()
	default:
		content = "Unknown screen"
	}

	if m.ErrorMsg != "" {
		content += "\n" + errorStyle.Render("Error: "+m.ErrorMsg)
	}

	if m.Width > 0 {
		rendered := appStyle.Width(m.Width).Render(content)
		return rendered + "\n" + m.renderStatusBar()
	}
	return appStyle.Render(content) + "\n" + m.renderStatusBar()
}

// ─── Dashboard ─────────────────────────────────────────────────────────────

func (m Model) viewDashboard() string {
	var b strings.Builder

	// Logo (ASCII art with gradient colors)
	b.WriteString(renderLogo())
	b.WriteString("\n")

	// Menu
	b.WriteString(titleStyle.Render("  Menu"))
	b.WriteString("\n")

	for i, item := range dashboardMenuItems {
		if i == m.Cursor {
			b.WriteString(menuSelectedStyle.Render("▸ " + item))
		} else {
			b.WriteString(menuItemStyle.Render("  " + item))
		}
		b.WriteString("\n")
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter select • q quit"))
	return b.String()
}

// ─── Balance ───────────────────────────────────────────────────────────────

func (m Model) viewBalance() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("💰 Account Balance"))
	b.WriteString("\n\n")

	if len(m.BalanceInfos) == 0 {
		b.WriteString(noDataStyle.Render("Loading balance..."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc/q back"))
		return b.String()
	}

	for _, info := range m.BalanceInfos {
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Currency:"),
			detailValueStyle.Render(info["currency"])))
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Total:"),
			detailValueStyle.Bold(true).Render(info["total_balance"])))
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Granted:"),
			detailValueStyle.Render(info["granted_balance"])))
		b.WriteString(fmt.Sprintf("%s %s\n",
			detailLabelStyle.Render("Topped Up:"),
			detailValueStyle.Render(info["topped_up_balance"])))
		b.WriteString("\n")
	}

	if !m.IsAvailable {
		b.WriteString(badgeWarnStyle.Render("⚠ Account currently unavailable"))
		b.WriteString("\n\n")
	}

	b.WriteString(helpStyle.Render("  esc/q back"))
	return b.String()
}

// ─── Models ────────────────────────────────────────────────────────────────

func (m Model) viewModels() string {
	var b strings.Builder

	count := len(m.ModelList)
	header := fmt.Sprintf("🤖 Models — %d available", count)
	b.WriteString(headerStyle.Render(header))
	b.WriteString("\n")

	if count == 0 {
		b.WriteString(noDataStyle.Render("Loading models..."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc/q back"))
		return b.String()
	}

	visibleItems := m.visibleItems(1)
	end := m.Scroll + visibleItems
	if end > count {
		end = count
	}

	for i := m.Scroll; i < end; i++ {
		mdl := m.ModelList[i]
		cursor := "  "
		style := listItemStyle
		if i == m.Cursor {
			cursor = "▸ "
			style = listSelectedStyle
		}
		b.WriteString(fmt.Sprintf("%s%s  %s  %s\n",
			cursor,
			style.Render(mdl.ID),
			detailValueStyle.Render(mdl.Object),
			timestampStyle.Render(mdl.OwnedBy)))
	}

	if count > visibleItems {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("%d-%d of %d", m.Scroll+1, end, count))))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • esc/q back"))
	return b.String()
}

// ─── History ───────────────────────────────────────────────────────────────

func (m Model) viewHistory() string {
	var b strings.Builder
	count := len(m.HistoryMessages)

	b.WriteString(headerStyle.Render(fmt.Sprintf("📜 History — %d messages", count)))
	b.WriteString("\n")

	if count == 0 {
		b.WriteString(noDataStyle.Render("No history messages found."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc/q back"))
		return b.String()
	}

	// Each history item occupies 2 display lines (main + content preview).
	// visibleItems(2) accounts for this: (Height - 5) / 2, min 3.
	visibleItems := m.visibleItems(2)
	end := m.Scroll + visibleItems
	if end > count {
		end = count
	}

	// Top scroll indicator — always visible
	if m.Scroll > 0 {
		b.WriteString("  ▲ " + timestampStyle.Render(fmt.Sprintf(
			"%d items above", m.Scroll)))
	} else {
		b.WriteString("  ▲ " + timestampStyle.Render("at top"))
	}
	b.WriteString("\n")

	for i := m.Scroll; i < end; i++ {
		msg := m.HistoryMessages[i]
		cursor := "  "
		roleStyle := chatRoleUserStyle
		if i == m.Cursor {
			cursor = "▸ "
		}

		role := msg.Role
		if role == "assistant" {
			roleStyle = chatRoleAssistantStyle
		}

		preview := truncateStr(msg.Content, 80)
		tcs := ""
		if msg.ToolCallID != "" || len(msg.ToolCalls) > 0 {
			tcs = " 🔧"
		}

		timeStr := msg.CreatedAt.Format("2006-01-02 15:04:05")
		b.WriteString(fmt.Sprintf("%s#%-5d %s%s %s\n",
			cursor,
			msg.ID,
			roleStyle.Render(fmt.Sprintf("%-10s", role)),
			tcs,
			timestampStyle.Render(timeStr)))

		if preview != "" {
			b.WriteString(contentPreviewStyle.Render(preview))
			b.WriteString("\n")
		}
	}

	// Bottom scroll indicator — always visible
	if end < count {
		b.WriteString("  ▼ " + timestampStyle.Render(fmt.Sprintf(
			"%d items below", count-end)))
	} else {
		b.WriteString("  ▼ " + timestampStyle.Render("at bottom"))
	}
	b.WriteString("\n")

	b.WriteString(helpStyle.Render("\n  j/k navigate • enter view details • esc/q back"))
	return b.String()
}

// ─── History Detail ─────────────────────────────────────────────────────────

func (m Model) viewHistoryDetail() string {
	var b strings.Builder

	if m.HistoryDetail == nil {
		b.WriteString(headerStyle.Render("📜 History — Message Detail"))
		b.WriteString("\n\n")
		b.WriteString(timestampStyle.Render("  Loading..."))
		b.WriteString(helpStyle.Render("\n  esc/q back"))
		return b.String()
	}

	msg := m.HistoryDetail

	// Wrap width for content sections.
	wrapWidth := m.Width - 6
	if wrapWidth < 20 {
		wrapWidth = 20
	}

	// ── Build all display lines ───────────────────────────────────────
	// Layout:
	//   Row 0: header (always visible)
	//   Row 1: ▲ indicator (always visible, blank look-alike at top)
	//   Rows 2..H-3: scrollable content
	//   Row H-2: ▼ indicator (always visible)
	//   Row H-1: help (always visible)
	//
	// allLines[0] = header, allLines[1] = blank separator,
	// allLines[2:] = fields + reasoning + content sections.

	var allLines []string

	allLines = append(allLines, headerStyle.Render(fmt.Sprintf("📜 History — Message #%d", msg.ID)))
	allLines = append(allLines, "")

	// Field labels and values
	type fieldRow struct {
		label string
		value string
	}
	fields := []fieldRow{
		{"ID", fmt.Sprint(msg.ID)},
		{"ModelID", fmt.Sprint(msg.ModelID)},
		{"SessionID", fmt.Sprint(msg.SessionID)},
		{"Role", msg.Role},
		{"ToolCallID", msg.ToolCallID},
		{"ToolCalls", truncateStr(prompt.ToSQLNullString(msg.ToolCalls).String, 120)},
		{"CreatedAt", msg.CreatedAt.Format("2006-01-02 15:04:05")},
	}

	for _, f := range fields {
		allLines = append(allLines,
			"  "+detailLabelStyle.Render(f.label)+" "+detailValueStyle.Render(f.value))
	}

	// Reasoning Content
	if msg.ReasoningContent != "" {
		allLines = append(allLines, "")
		allLines = append(allLines, sectionHeadingStyle.Render("── Reasoning Content ──"))
		wrapped := detailContentStyle.Width(wrapWidth).Render(msg.ReasoningContent)
		allLines = append(allLines, strings.Split(wrapped, "\n")...)
	}

	// Main Content
	allLines = append(allLines, "")
	allLines = append(allLines, sectionHeadingStyle.Render("── Content ──"))
	if msg.Content != "" {
		wrapped := detailContentStyle.Width(wrapWidth).Render(msg.Content)
		allLines = append(allLines, strings.Split(wrapped, "\n")...)
	} else {
		allLines = append(allLines, timestampStyle.Render("(empty)"))
	}

	// ── Scroll logic ──────────────────────────────────────────────────
	// Fixed rows: header(1) + top indicator(1) + bottom indicator(1) + help(1) = 4
	// Content starts at allLines[2] (index 0=header, 1=blank/indicator placeholder).
	// Offset formula: scrollStart = 2 + m.Scroll

	const fixedRows = 4 // header + top ▲ + bottom ▼ + help
	contentStart := 2   // allLines index where scrollable content begins
	scrollable := len(allLines) - contentStart

	vis := m.Height - fixedRows
	if vis < 3 {
		vis = 3
	}

	maxScroll := scrollable - vis
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.Scroll > maxScroll {
		m.Scroll = maxScroll
	}

	scrollStart := contentStart + m.Scroll
	scrollEnd := scrollStart + vis
	if scrollEnd > len(allLines) {
		scrollEnd = len(allLines)
	}

	atBottom := scrollEnd >= len(allLines)

	// ── Render ────────────────────────────────────────────────────────

	// Row 0: header
	b.WriteString(allLines[0])
	b.WriteString("\n")

	// Row 1: ▲ indicator — always visible
	if m.Scroll > 0 {
		b.WriteString("  ▲ " + timestampStyle.Render(fmt.Sprintf(
			"scrolled %d lines (j=down k=up)", m.Scroll)))
	} else {
		b.WriteString("  ▲ " + timestampStyle.Render("at top — j/k to scroll"))
	}
	b.WriteString("\n")

	// Rows 2..H-3: scrollable content
	for i := scrollStart; i < scrollEnd; i++ {
		b.WriteString(allLines[i])
		b.WriteString("\n")
	}

	// Row H-2: ▼ indicator — always visible
	if !atBottom {
		remaining := len(allLines) - scrollEnd
		b.WriteString("  ▼ " + timestampStyle.Render(fmt.Sprintf(
			"%d more lines below", remaining)))
	} else {
		b.WriteString("  ▼ " + timestampStyle.Render("at bottom"))
	}
	b.WriteString("\n")

	// Row H-1: help
	b.WriteString(helpStyle.Render("\n  j/k scroll • esc/q back"))
	return b.String()
}

// ─── Skills ────────────────────────────────────────────────────────────────

func (m Model) viewSkills() string {
	var b strings.Builder
	count := len(m.SkillInfos)

	b.WriteString(headerStyle.Render(fmt.Sprintf("🔧 Skills — %d available", count)))
	b.WriteString("\n")

	if count == 0 {
		b.WriteString(noDataStyle.Render("No skills found."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc/q back"))
		return b.String()
	}

	visibleItems := m.visibleItems(1)
	end := m.Scroll + visibleItems
	if end > count {
		end = count
	}

	for i := m.Scroll; i < end; i++ {
		info := m.SkillInfos[i]
		cursor := "  "
		style := listItemStyle
		if i == m.Cursor {
			cursor = "▸ "
			style = listSelectedStyle
		}

		autoInject := ""
		if info.AutoInject {
			autoInject = " ⚡auto"
		}

		b.WriteString(fmt.Sprintf("%s%s  %s%s\n",
			cursor,
			style.Render(info.Name),
			timestampStyle.Render(info.Scope),
			autoInject))
	}

	if count > visibleItems {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("%d-%d of %d", m.Scroll+1, end, count))))
	}

	b.WriteString(helpStyle.Render("\n  j/k navigate • esc/q back"))
	return b.String()
}

// ─── Prompt ────────────────────────────────────────────────────────────────

func (m Model) viewPrompt() string {
	var b strings.Builder

	b.WriteString(headerStyle.Render("📝 System Prompt (chat model)"))
	b.WriteString("\n")

	if m.PromptContent == "" {
		b.WriteString(noDataStyle.Render("Loading prompt..."))
		b.WriteString("\n\n")
		b.WriteString(helpStyle.Render("  esc/q back"))
		return b.String()
	}

	// Wrap content
	wrapWidth := m.Width - 6
	if wrapWidth < 20 {
		wrapWidth = 20
	}
	wrapped := detailContentStyle.Width(wrapWidth).Render(m.PromptContent)
	contentLines := strings.Split(wrapped, "\n")
	maxLines := m.Height - 9
	if maxLines < 5 {
		maxLines = 5
	}

	maxScroll := len(contentLines) - maxLines
	if maxScroll < 0 {
		maxScroll = 0
	}
	if m.Scroll > maxScroll {
		m.Scroll = maxScroll
	}

	end := m.Scroll + maxLines
	if end > len(contentLines) {
		end = len(contentLines)
	}

	for i := m.Scroll; i < end; i++ {
		b.WriteString(contentLines[i])
		b.WriteString("\n")
	}

	if len(contentLines) > maxLines {
		b.WriteString(fmt.Sprintf("\n  %s",
			timestampStyle.Render(fmt.Sprintf("line %d-%d of %d", m.Scroll+1, end, len(contentLines)))))
	}

	b.WriteString(helpStyle.Render("\n  j/k scroll • esc/q back"))
	return b.String()
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func truncateStr(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "..."
}

// renderBubble renders a chat bubble with proper word-wrapping.
//
// In lipgloss v1.1.0, MaxWidth incorrectly truncates content instead of
// wrapping it.  We work around this by using Width() (which wraps correctly)
// but only when the content exceeds contentAreaW.  Short content skips Width
// so the bubble shrinks to fit naturally.
//
// The prefix (e.g. "  👤 ") is attached to the first content line after
// wrapping, so it stays inline for short messages and only gets a separate
// line when the first text line is too long to accommodate it.
//
// Parameters:
//   - base:  bubble border style (e.g. userBubbleBase)
//   - prefix: leading text like "  👤 " (empty for thinking bubbles)
//   - content: the message text (may contain embedded newlines)
//   - wrapStyle: pre-built Width(contentAreaW) style for wrapping
//   - contentAreaW: max text area width (bubbleMaxW - border - padding)
func renderBubble(base lipgloss.Style, prefix, content string, wrapStyle lipgloss.Style, contentAreaW int) string {
	fullText := prefix + content

	// Fast path: content fits without wrapping → render as-is (bubble shrinks).
	needsWrap := false
	for _, line := range strings.Split(fullText, "\n") {
		if lipgloss.Width(line) > contentAreaW {
			needsWrap = true
			break
		}
	}
	if !needsWrap {
		return base.Render(fullText)
	}

	// Slow path: content needs wrapping.
	// Pre-wrap the content without the prefix so the wrapping engine sees
	// CJK character boundaries correctly (otherwise prefix + CJK text form
	// one giant "word" and the prefix ends up alone on the first line).
	wrappedContent := wrapStyle.Render(content)
	wrappedLines := strings.Split(wrappedContent, "\n")

	// Attach the prefix to the first wrapped line if it fits.
	if prefix != "" && len(wrappedLines) > 0 {
		firstLine := wrappedLines[0]
		if lipgloss.Width(prefix+firstLine) <= contentAreaW {
			wrappedLines[0] = prefix + firstLine
		} else {
			// Prefix doesn't fit → give it its own line.
			wrappedLines = append([]string{prefix}, wrappedLines...)
		}
	}

	return base.Render(strings.Join(wrappedLines, "\n"))
}

// ─── Padding helpers ─────────────────────────────────────────────────────
// These use plain spaces instead of lipgloss alignment to avoid
// ANSI-on-ANSI rendering corruption that caused top-border clipping.

// padRight returns s with each line right-aligned within width w,
// using plain spaces for 2-char left/right margins.
func padRight(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lw := lipgloss.Width(line)
		left := w - 4 - lw // 2 margin + text + 2 margin = w
		if left < 0 {
			left = 0
		}
		lines[i] = strings.Repeat(" ", 2+left) + line + "  "
	}
	return strings.Join(lines, "\n")
}

// padLeft returns s with each line left-aligned within width w,
// using plain spaces for 2-char left/right margins.
func padLeft(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lw := lipgloss.Width(line)
		right := w - 4 - lw
		if right < 0 {
			right = 0
		}
		lines[i] = "  " + line + strings.Repeat(" ", 2+right)
	}
	return strings.Join(lines, "\n")
}

// padCenter returns s with each line center-aligned within width w,
// using plain spaces for 2-char left/right margins.
func padCenter(s string, w int) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lw := lipgloss.Width(line)
		totalPad := w - 4 - lw
		if totalPad < 0 {
			totalPad = 0
		}
		left := totalPad / 2
		right := totalPad - left
		lines[i] = strings.Repeat(" ", 2+left) + line + strings.Repeat(" ", 2+right)
	}
	return strings.Join(lines, "\n")
}

// ─── Chat (full-screen) ──────────────────────────────────────────────────────

// viewChat renders a full-screen chat interface:
//
//	┌─────────────────────────────────┐  y=0
//	│ 💬 Chat                         │  header
//	│                                 │  spacer
//	│ ▲ scrolled (N lines)            │  indicator (if scrolled)
//	│ ... chat messages ...           │  chat area (scrollable, line-based)
//	│                                 │  ↑ padded to fixed height
//	│ ┌─ Input ─────────────────────┐ │  input (fixed bottom)
//	│ enter send • i • j/k • G • esc │  help  (fixed bottom)
//	│ dscli v1.0 │ 📁 proj │ 🤖      │  statusbar (fixed bottom)
//	└─────────────────────────────────┘  y=m.Height-1
//
// The bottom 5 lines are pinned to the terminal bottom by padding the chat
// area with empty lines.  Scroll offset (ChatScroll) is a line offset from
// the bottom — pressing j/k moves by 1 display line, not 1 message.
func (m Model) viewChat() string {
	var b strings.Builder

	// ── Dimensions ────────────────────────────────────────────────────────

	// Content width equals terminal width (chat takes full screen).
	contentW := m.Width
	if contentW < 30 {
		contentW = 30
	}

	// Available width for bubble content (between 2-char left/right margins).
	// Margins are now baked into the alignment styles (Padding 0,2), not
	// applied by an outer lipgloss wrapper in View().
	availW := contentW - 4
	if availW < 30 {
		availW = 30
	}

	bubbleMaxW := availW * bubbleMaxPercent / 100
	if bubbleMaxW < 16 {
		bubbleMaxW = 16
	}

	contentAreaW := bubbleMaxW - 4 // border(2) + padding(2)
	if contentAreaW < 8 {
		contentAreaW = 8
	}
	wrapStyle := lipgloss.NewStyle().Width(contentAreaW)
	// Margins are now applied as plain spaces via padRight/padLeft/padCenter,
	// completely avoiding lipgloss alignment on top of ANSI-styled content.
	// This eliminates the risk of lipgloss-on-lipgloss rendering corruption
	// that caused top-border clipping on first render (issue #1).
	// Layout line budget:
	//   topFixed:        header(1) + spacer(1) = 2
	//   bottomFixed:     input(2, incl MarginTop) + help(2, incl MarginTop) = 4
	//                    statusbar is appended by View(), outside padding
	//   scrollIndicator: 0 or 1
	const topFixedLines = 2
	const bottomFixedLines = 4 // input+help only; statusbar is outside

	indicatorLines := 0
	if m.ChatScroll > 0 {
		indicatorLines = 1
	}

	chatAreaHeight := m.Height - topFixedLines - bottomFixedLines - indicatorLines
	if chatAreaHeight < 1 {
		chatAreaHeight = 1
	}

	// Error banner steals 1 line from the chat area when present.
	if m.ErrorMsg != "" {
		chatAreaHeight--
		if chatAreaHeight < 1 {
			chatAreaHeight = 1
		}
	}

	// ── Group chatLines into response units ───────────────────────────────
	//
	// Each user message is its own unit (right-aligned green bubble).
	// Consecutive non-user messages (assistant, tool, state, etc.) are
	// merged into a single response group and rendered as ONE unified
	// bubble with multi-section content.

	type section struct {
		header string        // section header, e.g. "💭 思考过程:"; empty = no header
		body   string        // section body text
		style  lipgloss.Style // body style (inside bubble)
	}

	type displayLine struct {
		role    string // "user" | "assistant" | "spinner"
		content string
	}

	var displayLines []displayLine

	// Group consecutive non-user chatLines into response groups.
	// Each user chatLine is emitted immediately as its own displayLine.
	flushGroup := func(group []chatLine) {
		if len(group) == 0 {
			return
		}
		// Build multi-section content for this response group.
		var sections []section
		for _, cl := range group {
			if cl.ReasoningContent != "" {
				// ── Thinking section ──
				sections = append(sections, section{
					header: "💭 思考过程:",
					body:   "  ✨" + strings.TrimSpace(cl.ReasoningContent),
					style:  thinkBodyStyle,
				})
				// ── Assistant answer (if present after reasoning) ──
				if cl.Content != "" {
					sections = append(sections, section{
						body:  strings.TrimSpace(cl.Content),
						style: assistantBodyStyle,
					})
				}
				// ── Tool calls ──
				if cl.HasToolCalls {
					var sb strings.Builder
					for _, t := range cl.ToolCallNames {
						sb.WriteString("    ➜  ")
						sb.WriteString(t)
						sb.WriteString("\n")
					}
					sections = append(sections, section{
						header: "🔄 工具调用:",
						body:   sb.String(),
						style:  thinkBodyStyle,
					})
				}
			} else if cl.Role == "tool" {
				// ── Tool result ──
				sections = append(sections, section{
					header: "🔧 工具结果:",
					body:   strings.TrimSpace(cl.Content),
					style:  toolBodyStyle,
				})
			} else if cl.FinishReason == "length" {
				// ── Truncation warning ──
				sections = append(sections, section{
					body:  "⚠️ 响应可能被截断（超出长度限制）",
					style: truncationBodyStyle,
				})
			} else if cl.Role == "state" {
				// ── Session state (e.g. cost/tokens) ──
				sections = append(sections, section{
					body:  cl.Content,
					style: stateBodyStyle,
				})
			} else {
				// ── Plain assistant ──
				sections = append(sections, section{
					body:  cl.Content,
					style: assistantBodyStyle,
				})
			}
		}

		// Render sections into a single styled content string.
		var contentBuilder strings.Builder
		for i, sec := range sections {
			if i > 0 {
				contentBuilder.WriteString("\n")
			}
			if sec.header != "" {
				contentBuilder.WriteString(sec.header)
				contentBuilder.WriteString("\n")
			}
			contentBuilder.WriteString(sec.style.Render(sec.body))
		}
		displayLines = append(displayLines, displayLine{
			role:    "assistant",
			content: contentBuilder.String(),
		})
	}

	var group []chatLine
	for _, cl := range m.ChatMessages {
		if cl.Role == "user" {
			// Flush any pending non-user group before the user message.
			flushGroup(group)
			group = nil
			// User message is always its own bubble.
			displayLines = append(displayLines, displayLine{
				role:    "user",
				content: cl.Content,
			})
		} else {
			group = append(group, cl)
		}
	}
	flushGroup(group) // flush trailing group

	// Spinner (if loading) appended as a display line.
	if m.ChatLoading {
		displayLines = append(displayLines, displayLine{
			role:    "spinner",
			content: m.ChatSpinner.View() + " Thinking...",
		})
	}

	// ── Render & flatten to individual lines ──────────────────────────────

	var allLines []string
	for _, dl := range displayLines {
		var rendered string
		switch dl.role {
		case "user":
			// Right-align user messages using plain spaces — avoids
			// lipgloss-on-lipgloss rendering issues with ANSI borders.
			rendered = padRight(renderBubble(userBubbleBase, "👤 ", dl.content, wrapStyle, contentAreaW), contentW)
		case "assistant":
			// Left-aligned unified bubble with all assistant/tool/thinking content.
			rendered = padLeft(renderBubble(assistantBubbleBase, "🧠 ", dl.content, wrapStyle, contentAreaW), contentW)
		case "spinner":
			rendered = padLeft(chatLoadingStyle.Render(dl.content), contentW)
		default:
			rendered = timestampStyle.Render(dl.content)
		}

		lines := strings.Split(rendered, "\n")
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
		allLines = append(allLines, lines...)
	}
	// ── Scroll logic (line-based) ─────────────────────────────────────────

	totalLines := len(allLines)

	// Compute maxScroll and cache it for handleChatKeys().
	maxScroll := totalLines - chatAreaHeight
	maxScroll = max(maxScroll, 0)
	lastChatMaxScroll = maxScroll

	// Use a local clamped scroll offset; the model's ChatScroll may exceed
	// maxScroll (e.g. 'g' sentinel) and handleChatKeys() clamps it later.
	scroll := m.ChatScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}

	visibleStart := totalLines - chatAreaHeight - scroll
	visibleEnd := totalLines - scroll

	if visibleStart < 0 {
		visibleStart = 0
	}
	if visibleEnd < 0 {
		visibleEnd = 0
	}
	if visibleEnd > totalLines {
		visibleEnd = totalLines
	}
	if visibleStart >= visibleEnd {
		visibleStart = 0
		visibleEnd = totalLines
		if visibleEnd > chatAreaHeight {
			visibleEnd = chatAreaHeight
		}
	}

	visibleCount := visibleEnd - visibleStart

	// ── Render output ─────────────────────────────────────────────────────
	// ── Render output ─────────────────────────────────────────────────────

	// Header (with 2-char left margin)
	b.WriteString("  ")
	b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render("💬 Chat"))
	b.WriteString("\n")
	b.WriteString("  \n") // blank separator with left margin

	// Scroll indicator (with 2-char left margin)
	if m.ChatScroll > 0 {
		b.WriteString("  ")
		b.WriteString(timestampStyle.Render(fmt.Sprintf(
			"▲ scrolled %d lines (total %d)  G=bottom  j/k=scroll",
			m.ChatScroll, totalLines,
		)))
		b.WriteString("\n")
	}

	// Error banner (with 2-char left margin)
	if m.ErrorMsg != "" {
		b.WriteString("  ")
		b.WriteString(errorStyle.Render("Error: " + m.ErrorMsg))
		b.WriteString("\n")
	}

	// Visible chat lines — margins already baked into alignment styles
	for i := visibleStart; i < visibleEnd; i++ {
		b.WriteString(allLines[i])
		b.WriteString("\n")
	}

	// Pad remaining chat area (with left margin for consistency)
	for i := visibleCount; i < chatAreaHeight; i++ {
		b.WriteString("  \n")
	}

	// ── Bottom fixed elements ─────────────────────────────────────────────
	// Each starts with 2-char left margin for visual consistency.
	// The status bar (full-width) is appended by View() outside this content.

	// Input line (with 2-char left margin)
	if m.ChatInput.Focused() {
		b.WriteString("  ")
		b.WriteString(chatInputStyle.Render(m.ChatInput.View()))
	} else {
		b.WriteString("  \n")
		b.WriteString("  ")
		b.WriteString(timestampStyle.Render("Press 'i' to type, esc/q to leave"))
	}
	b.WriteString("\n")

	// Help line (with 2-char left margin)
	b.WriteString("  ")
	b.WriteString(helpStyle.Render("enter send • i focus input • j/k scroll • G bottom • esc back"))
	b.WriteString("\n")

	return b.String()
}

// renderStatusBar builds the bottom status bar showing app version, project
func (m Model) renderStatusBar() string {
	if m.Width < 1 {
		return ""
	}

	// Left side: version badge + project root
	versionBadge := statusVersion.Render("dscli " + m.Version)
	projectLabel := statusLabel.Render(" 📁 " + shortenPath(m.ProjectRoot))
	left := versionBadge + " " + statusSep.Render("│") + " " + projectLabel

	// Right side: model + screen name
	modelLabel := statusLabel.Render("🤖 " + modelDisplayName())
	screenLabel := statusScreen.Render(screenIcon(m.Screen) + " " + screenTitle(m.Screen))
	right := modelLabel + " " + statusSep.Render("│") + " " + screenLabel

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := m.Width - leftW - rightW
	if gap < 1 {
		gap = 1
	}

	return statusBarBg.Width(m.Width).Render(left + strings.Repeat(" ", gap) + right)
}

// screenTitle returns a short human-readable name for each screen.
func screenTitle(s Screen) string {
	switch s {
	case ScreenDashboard:
		return "Home"
	case ScreenBalance:
		return "Balance"
	case ScreenModels:
		return "Models"
	case ScreenHistory:
		return "History"
	case ScreenHistoryDetail:
		return "Detail"
	case ScreenSkills:
		return "Skills"
	case ScreenPrompt:
		return "Prompt"
	case ScreenChat:
		return "Chat"
	default:
		return "Unknown"
	}
}

// screenIcon returns an emoji icon for each screen.
func screenIcon(s Screen) string {
	switch s {
	case ScreenDashboard:
		return "🏠"
	case ScreenBalance:
		return "💰"
	case ScreenModels:
		return "🤖"
	case ScreenHistory:
		return "📜"
	case ScreenHistoryDetail:
		return "📜"
	case ScreenSkills:
		return "🔧"
	case ScreenPrompt:
		return "📝"
	case ScreenChat:
		return "💬"
	default:
		return "❓"
	}
}

// modelDisplayName returns the currently configured chat model name.
func modelDisplayName() string {
	return context.ModelDeepseekChat
}

// shortenPath replaces the home directory prefix with ~ for compact display.
func shortenPath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	// Fallback: show only the last two components (parent/base).
	base := filepath.Base(p)
	parent := filepath.Base(filepath.Dir(p))
	if parent != "." && parent != "/" && parent != "" {
		return parent + "/" + base
	}
	return base
}