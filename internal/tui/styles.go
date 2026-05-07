// Package tui implements the Bubbletea terminal UI for dscli.
//
// Following the Gentleman Bubbletea patterns:
// - Screen constants as iota
// - Single Model struct holds ALL state
// - Update() with type switch
// - Per-screen key handlers returning (tea.Model, tea.Cmd)
// - Vim keys (j/k) for navigation
package tui

import "github.com/charmbracelet/lipgloss"

// ─── Colors (DeepSeek-inspired palette) ─────────────────────────────────────

var (
	colorBase    = lipgloss.Color("#1a1b26") // Dark background
	colorSurface = lipgloss.Color("#24253e") // Panel background
	colorOverlay = lipgloss.Color("#565f89") // Muted borders
	colorText    = lipgloss.Color("#c0caf5") // Light text
	colorSubtext = lipgloss.Color("#9aa5ce") // Dim text
	colorPrimary = lipgloss.Color("#7aa2f7") // Primary blue
	colorGreen   = lipgloss.Color("#9ece6a") // Success
	colorPeach   = lipgloss.Color("#ff9e64") // Warm accent
	colorRed     = lipgloss.Color("#f7768e") // Soft red
	colorBlue    = lipgloss.Color("#2ac3de") // Cyan
	colorMauve   = lipgloss.Color("#bb9af7") // Mauve
	colorYellow  = lipgloss.Color("#e0af68") // Gold
	colorTeal    = lipgloss.Color("#1abc9c") // Teal
)

// ─── Layout Styles ──────────────────────────────────────────────────────────

var (
	appStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Padding(1, 2)

	headerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			BorderStyle(lipgloss.NormalBorder()).
			BorderBottom(true).
			BorderForeground(colorOverlay).
			PaddingBottom(1).
			MarginBottom(1)

	helpStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			MarginTop(1)

	errorStyle = lipgloss.NewStyle().
			Foreground(colorRed).
			Bold(true).
			Padding(0, 1)
)

// ─── Dashboard Styles ───────────────────────────────────────────────────────

var (
	menuItemStyle = lipgloss.NewStyle().
			Foreground(colorText).
			PaddingLeft(2)

	menuSelectedStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true).
				PaddingLeft(1)

	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorMauve).
			MarginBottom(1)

	logoStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorPrimary).
			Border(lipgloss.DoubleBorder()).
			BorderForeground(colorOverlay).
			Padding(0, 2).
			MarginBottom(1)
)

// ─── List Styles ────────────────────────────────────────────────────────────

var (
	listItemStyle = lipgloss.NewStyle().
			Foreground(colorText).
			PaddingLeft(2)

	listSelectedStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true).
				PaddingLeft(1)

	timestampStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Italic(true)

	contentPreviewStyle = lipgloss.NewStyle().
				Foreground(colorSubtext).
				PaddingLeft(4)

	noDataStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Italic(true).
			PaddingLeft(2).
			MarginTop(1)
)

// ─── Detail Styles ──────────────────────────────────────────────────────────

var (
	sectionHeadingStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorMauve).
				MarginTop(1).
				MarginBottom(1)

	detailLabelStyle = lipgloss.NewStyle().
				Foreground(colorSubtext).
				Width(14).
				Align(lipgloss.Right).
				PaddingRight(1)

	detailValueStyle = lipgloss.NewStyle().
				Foreground(colorText)

	detailContentStyle = lipgloss.NewStyle().
				Foreground(colorText).
				PaddingLeft(2)
)

// ─── Chat Styles ────────────────────────────────────────────────────────────

var (
	chatInputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorPrimary).
			Foreground(colorText).
			Padding(0, 1).
			MarginTop(1)

	chatLoadingStyle = lipgloss.NewStyle().
				Foreground(colorSubtext).
				Italic(true).
				PaddingLeft(2)

	// Role label styles (used in history list and elsewhere).
	chatRoleUserStyle = lipgloss.NewStyle().
				Foreground(colorGreen).
				Bold(true)

	chatRoleAssistantStyle = lipgloss.NewStyle().
				Foreground(colorPrimary).
				Bold(true)
)

// Bubble chat — user right (green), assistant left (blue), system centered.
var (
	// userBubbleBase is the base style for user message bubbles.
	// Call .MaxWidth(w) at render time to constrain width.
	userBubbleBase = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorGreen).
			Padding(0, 1)

	// assistantBubbleBase is the base style for assistant message bubbles.
	assistantBubbleBase = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorPrimary).
				Padding(0, 1)

	// thinkBubbleBase is the base style for reasoning/thinking bubbles.
	// Uses mauve border to distinguish from assistant (blue) and user (green).
	thinkBubbleBase = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorMauve).
			Padding(0, 1)


	// thinkLineStyle for reasoning/thinking content preview.
	thinkLineStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Italic(true)

	// toolLineStyle for tool-call result lines.
	toolLineStyle = lipgloss.NewStyle().
			Foreground(colorYellow).
			Italic(true)

	// truncationWarnBubble for truncation warnings (centered, red).
	truncationWarnBubble = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)

	// ── Unified bubble internal styles ─────────────────────────────────
	// assistantBodyStyle: white/bold for assistant's final answer.
	assistantBodyStyle = lipgloss.NewStyle().
				Foreground(colorText).
				Bold(true)
	thinkBodyStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Italic(true)

	// toolBodyStyle: yellow/italic for tool results inside unified bubble.
	toolBodyStyle = lipgloss.NewStyle().
			Foreground(colorYellow).
			Italic(true)

	// stateBodyStyle: subtle for session-state lines.
	stateBodyStyle = lipgloss.NewStyle().
			Foreground(colorSubtext).
			Italic(true)

	// truncationBodyStyle: red/bold for truncation warning inside bubble.
	truncationBodyStyle = lipgloss.NewStyle().
				Foreground(colorRed).
				Bold(true)
)

// bubbleMaxPercent is the maximum bubble width as a percentage of available
// content width (excludes borders and padding).
const bubbleMaxPercent = 72
// ─── Status Badge Styles ────────────────────────────────────────────────────

var (
	badgeSuccessStyle = lipgloss.NewStyle().
				Foreground(colorGreen).
				Bold(true)

	badgeWarnStyle = lipgloss.NewStyle().
			Foreground(colorYellow).
			Bold(true)
)

// ─── Status Bar Styles ──────────────────────────────────────────────────────

var (
	// statusBarBg is the full-width bar background.
	statusBarBg = lipgloss.NewStyle().
			Background(colorSurface)

	// statusVersion is the version badge (mauve bg, dark text).
	statusVersion = lipgloss.NewStyle().
			Background(colorMauve).
			Foreground(colorBase).
			Bold(true).
			Padding(0, 1)

	// statusLabel is for labels like 📁 project / 🤖 model.
	statusLabel = lipgloss.NewStyle().
			Foreground(colorSubtext)

	// statusSep is the separator between sections.
	statusSep = lipgloss.NewStyle().
			Foreground(colorOverlay)

	// statusScreen is the current screen name (rightmost, primary accent).
	statusScreen = lipgloss.NewStyle().
			Foreground(colorPrimary).
			Bold(true).
			Padding(0, 1)
)