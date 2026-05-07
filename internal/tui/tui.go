package tui

import (
	"context"

	"gitcode.com/dscli/dscli/internal/dsc"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/skills"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// ─── Screens ────────────────────────────────────────────────────────────────

type Screen int

const (
	ScreenDashboard Screen = iota
	ScreenBalance
	ScreenModels
	ScreenHistory
	ScreenHistoryDetail
	ScreenSkills
	ScreenPrompt
	ScreenChat
)

// ─── Custom Messages ────────────────────────────────────────────────────────

type balanceMsg struct {
	resp *dsc.BalanceResponse
	err  error
}

type modelsMsg struct {
	resp *dsc.ModelsResponse
	err  error
}

type historyMsg struct {
	messages []*prompt.Message
	err      error
}

type skillsMsg struct {
	infos []skills.SkillInfo
	err   error
}

type promptContentMsg struct {
	content string
	err     error
}

// chatStreamMsg carries incremental chat results from the streaming goroutine.
// When done is true, the chat stream has completed (success or error).
// When replaceLast is true, msg.lines replaces the last ChatMessages entry
// instead of being appended; this enables incremental content streaming.
type chatStreamMsg struct {
	lines       []chatLine
	err         error
	done        bool
	replaceLast bool
}

// fetchHistoryDetailMsg carries the result of fetching a single message's details.
type fetchHistoryDetailMsg struct {
	message *prompt.Message
	err     error
}

// ─── Model ──────────────────────────────────────────────────────────────────

type Model struct {
	// API client (shared with main via SetClient)
	client dsc.Client
	ctx    context.Context

	// App info
	Version     string
	Build       string
	ProjectRoot string

	// Screen state
	Screen     Screen
	PrevScreen Screen
	Width      int
	Height     int
	Cursor     int
	Scroll     int
	ErrorMsg   string

	// Balance
	BalanceInfos []map[string]string
	IsAvailable  bool

	// Models
	ModelList []dsc.Model

	// History
	HistoryMessages []*prompt.Message
	HistoryDetail   *prompt.Message

	// Skills
	SkillInfos []skills.SkillInfo

	// Prompt
	PromptContent string

	// Chat
	ChatInput    textinput.Model
	ChatMessages []chatLine
	ChatLoading  bool
	ChatSpinner  spinner.Model
	ChatScroll   int                // scroll offset for chat view
	ChatStream   chan chatStreamMsg // channel for streaming chat responses
}

// chatLine represents a single line in the chat display.
// chatLine represents a single line in the chat display.
type chatLine struct {
	Role             string // "user", "assistant", or "tool"
	Content          string
	ReasoningContent string // deepseek reasoner reasoning content
	FinishReason     string // "length" if truncated
	HasToolCalls     bool   // whether this message triggered tool calls
	ToolCallNames    []string
}

// New creates a new TUI model.
// New creates a new TUI model.
func New(ctx context.Context, client dsc.Client, version, build, projectRoot string) Model {
	ti := textinput.New()
	ti.Placeholder = "Type a message and press Enter..."
	ti.CharLimit = 4096
	ti.Width = 60

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = chatLoadingStyle

	return Model{
		client:      client,
		ctx:         ctx,
		Version:     version,
		Build:       build,
		ProjectRoot: projectRoot,
		Screen:      ScreenDashboard,
		ChatInput:   ti,
		ChatSpinner: sp,
	}
}

// Init loads initial data.
func (m Model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

// SetClient sets the API client (called from main).
func (m *Model) SetClient(client dsc.Client) {
	m.client = client
}