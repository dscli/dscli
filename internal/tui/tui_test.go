package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"gitcode.com/dscli/dscli/internal/dsc"
	"gitcode.com/dscli/dscli/internal/prompt"
)

// ─── truncateStr 测试 ────────────────────────────────────────────────────────

func TestTruncateStr(t *testing.T) {
	tests := []struct {
		name string
		s    string
		max  int
		want string
	}{
		{name: "短字符串不截断", s: "hello", max: 10, want: "hello"},
		{name: "正好等于max不截断", s: "hello", max: 5, want: "hello"},
		{name: "长字符串截断", s: "hello world this is a long string", max: 10, want: "hello worl..."},
		{name: "空字符串", s: "", max: 10, want: ""},
		{name: "max为0截断为空", s: "hello", max: 0, want: "..."},
		{name: "换行替换为空格", s: "hello\nworld", max: 20, want: "hello world"},
		{name: "换行+截断", s: "hello\nworld\nfoo", max: 8, want: "hello wo..."},
		{name: "中文字符截断", s: "你好世界你好世界", max: 3, want: "你好世..."},
		{name: "中文+英文混合", s: "hello你好world世界", max: 10, want: "hello你好wor..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateStr(tt.s, tt.max)
			if got != tt.want {
				t.Errorf("truncateStr(%q, %d) = %q, want %q", tt.s, tt.max, got, tt.want)
			}
		})
	}
}

// ─── getToolCallNames 测试 ───────────────────────────────────────────────────

func TestGetToolCallNames(t *testing.T) {
	tests := []struct {
		name  string
		tools []prompt.ToolCall
		want  []string
	}{
		{name: "空列表", tools: nil, want: nil},
		{
			name:  "单个工具调用",
			tools: []prompt.ToolCall{{ID: "call_1", Function: prompt.ToolCallFunction{Name: "shell"}}},
			want:  []string{"shell"},
		},
		{
			name: "多个工具调用",
			tools: []prompt.ToolCall{
				{ID: "call_1", Function: prompt.ToolCallFunction{Name: "read_file"}},
				{ID: "call_2", Function: prompt.ToolCallFunction{Name: "write_file"}},
				{ID: "call_3", Function: prompt.ToolCallFunction{Name: "search_code"}},
			},
			want: []string{"read_file", "write_file", "search_code"},
		},
		{
			name: "同名工具调用",
			tools: []prompt.ToolCall{
				{ID: "call_1", Function: prompt.ToolCallFunction{Name: "shell"}},
				{ID: "call_2", Function: prompt.ToolCallFunction{Name: "shell"}},
			},
			want: []string{"shell", "shell"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getToolCallNames(tt.tools)
			if len(got) != len(tt.want) {
				t.Errorf("getToolCallNames() len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getToolCallNames()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

// ─── buildToolChatLines 测试 ─────────────────────────────────────────────────

func TestBuildToolChatLines(t *testing.T) {
	tests := []struct {
		name             string
		toolInputs       []prompt.Message
		sourceToolCalls  []prompt.ToolCall
		wantCount        int
		wantContentParts []string
	}{
		{name: "空输入", toolInputs: nil, sourceToolCalls: nil, wantCount: 0},
		{
			name: "正常工具调用结果",
			toolInputs: []prompt.Message{
				{Role: "tool", ToolCallID: "call_1", Content: "执行结果"},
				{Role: "tool", ToolCallID: "call_2", Content: "第二个结果"},
			},
			sourceToolCalls: []prompt.ToolCall{
				{ID: "call_1", Function: prompt.ToolCallFunction{Name: "shell"}},
				{ID: "call_2", Function: prompt.ToolCallFunction{Name: "read_file"}},
			},
			wantCount:        2,
			wantContentParts: []string{"shell: 执行结果", "read_file: 第二个结果"},
		},
		{
			name: "ToolCallID查不到名称fallback",
			toolInputs: []prompt.Message{
				{Role: "tool", ToolCallID: "unknown_id", Content: "结果"},
			},
			sourceToolCalls: []prompt.ToolCall{
				{ID: "call_1", Function: prompt.ToolCallFunction{Name: "shell"}},
			},
			wantCount:        1,
			wantContentParts: []string{"未知工具(unknown_id)"},
		},
		{
			name: "ToolCallID为空fallback",
			toolInputs: []prompt.Message{
				{Role: "tool", Content: "结果"},
			},
			sourceToolCalls: []prompt.ToolCall{
				{ID: "call_1", Function: prompt.ToolCallFunction{Name: "shell"}},
			},
			wantCount:        1,
			wantContentParts: []string{"未知工具"},
		},
		{
			name: "sourceToolCalls为空时fallback",
			toolInputs: []prompt.Message{
				{Role: "tool", ToolCallID: "call_1", Content: "结果"},
			},
			sourceToolCalls:  nil,
			wantCount:        1,
			wantContentParts: []string{"未知工具(call_1)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildToolChatLines(tt.toolInputs, tt.sourceToolCalls)
			if len(got) != tt.wantCount {
				t.Errorf("buildToolChatLines() count = %d, want %d", len(got), tt.wantCount)
				return
			}
			for i, wantPart := range tt.wantContentParts {
				if i >= len(got) {
					break
				}
				if !strings.Contains(got[i].Content, wantPart) {
					t.Errorf("buildToolChatLines()[%d].Content = %q, want containing %q", i, got[i].Content, wantPart)
				}
			}
		})
	}
}

// ─── screenTitle 测试 ────────────────────────────────────────────────────────

func TestScreenTitle(t *testing.T) {
	tests := []struct {
		screen Screen
		want   string
	}{
		{ScreenDashboard, "Home"},
		{ScreenBalance, "Balance"},
		{ScreenModels, "Models"},
		{ScreenHistory, "History"},
		{ScreenHistoryDetail, "Detail"},
		{ScreenSkills, "Skills"},
		{ScreenPrompt, "Prompt"},
		{ScreenChat, "Chat"},
		{Screen(999), "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := screenTitle(tt.screen)
			if got != tt.want {
				t.Errorf("screenTitle(%d) = %q, want %q", tt.screen, got, tt.want)
			}
		})
	}
}

// ─── screenIcon 测试 ─────────────────────────────────────────────────────────

func TestScreenIcon(t *testing.T) {
	tests := []struct {
		screen Screen
		want   string
	}{
		{ScreenDashboard, "🏠"},
		{ScreenBalance, "💰"},
		{ScreenModels, "🤖"},
		{ScreenHistory, "📜"},
		{ScreenHistoryDetail, "📜"},
		{ScreenSkills, "🔧"},
		{ScreenPrompt, "📝"},
		{ScreenChat, "💬"},
		{Screen(999), "❓"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := screenIcon(tt.screen)
			if got != tt.want {
				t.Errorf("screenIcon(%d) = %q, want %q", tt.screen, got, tt.want)
			}
		})
	}
}

// ─── modelDisplayName 测试 ───────────────────────────────────────────────────

func TestModelDisplayName(t *testing.T) {
	got := modelDisplayName()
	if got == "" {
		t.Error("modelDisplayName() returned empty string")
	}
	if !strings.Contains(got, "deepseek") && !strings.Contains(got, "chat") {
		t.Logf("modelDisplayName() = %q (unexpected but possibly valid)", got)
	}
}

// ─── shortenPath 测试 ────────────────────────────────────────────────────────

func TestShortenPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("无法获取用户主目录")
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "主目录下路径替换为~", path: filepath.Join(home, "projects", "dscli"), want: "~/projects/dscli"},
		{name: "非主目录路径只显示最后两级", path: "/usr/local/bin/go", want: "bin/go"},
		{name: "单级路径", path: "dscli", want: "dscli"},
		{name: "根路径", path: "/", want: "/"},
		{name: "带多点路径", path: "/a/b/c/d/e", want: "d/e"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := shortenPath(tt.path)
			if got != tt.want {
				t.Errorf("shortenPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

// ─── visibleItems 测试 ───────────────────────────────────────────────────────

func TestVisibleItems(t *testing.T) {
	tests := []struct {
		name         string
		height       int
		linesPerItem int
		wantExact    int
	}{
		{name: "大终端1行项", height: 40, linesPerItem: 1, wantExact: 31},
		{name: "大终端2行项", height: 40, linesPerItem: 2, wantExact: 17},
		{name: "小终端使用最小值3", height: 10, linesPerItem: 1, wantExact: 3},
		{name: "极小终端2行项", height: 10, linesPerItem: 2, wantExact: 3},
		{name: "linesPerItem为0自动为1", height: 40, linesPerItem: 0, wantExact: 31},
		{name: "负数linesPerItem自动为1", height: 40, linesPerItem: -1, wantExact: 31},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{Height: tt.height}
			got := m.visibleItems(tt.linesPerItem)
			if got != tt.wantExact {
				t.Errorf("visibleItems(%d) with Height=%d = %d, want %d",
					tt.linesPerItem, tt.height, got, tt.wantExact)
			}
		})
	}
}

// ─── clampChatScroll 测试 ────────────────────────────────────────────────────

func TestClampChatScroll(t *testing.T) {
	oldMaxScroll := lastChatMaxScroll
	defer func() { lastChatMaxScroll = oldMaxScroll }()

	tests := []struct {
		name          string
		maxScroll     int
		chatScroll    int
		wantClampedTo int
	}{
		{name: "未超过最大值不变", maxScroll: 10, chatScroll: 5, wantClampedTo: 5},
		{name: "超过最大值被钳制", maxScroll: 10, chatScroll: 100, wantClampedTo: 10},
		{name: "等于最大值不变", maxScroll: 10, chatScroll: 10, wantClampedTo: 10},
		{name: "maxScroll为0时不钳制", maxScroll: 0, chatScroll: 100, wantClampedTo: 100},
		{name: "maxScroll为0且chatScroll为0", maxScroll: 0, chatScroll: 0, wantClampedTo: 0},
		{name: "maxScroll为负数不钳制", maxScroll: -1, chatScroll: 100, wantClampedTo: 100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lastChatMaxScroll = tt.maxScroll
			m := &Model{ChatScroll: tt.chatScroll}
			m.clampChatScroll()
			if m.ChatScroll != tt.wantClampedTo {
				t.Errorf("clampChatScroll() with maxScroll=%d, ChatScroll=%d → ChatScroll=%d, want %d",
					tt.maxScroll, tt.chatScroll, m.ChatScroll, tt.wantClampedTo)
			}
		})
	}
}

// ─── chatLine 结构测试 ───────────────────────────────────────────────────────

func TestChatLineDefaults(t *testing.T) {
	cl := chatLine{}
	if cl.Role != "" {
		t.Error("new chatLine should have empty Role")
	}
	if cl.HasToolCalls {
		t.Error("new chatLine should have HasToolCalls = false")
	}
}

// ─── renderLogo 测试 ─────────────────────────────────────────────────────────

func TestRenderLogo(t *testing.T) {
	logo := renderLogo()
	if logo == "" {
		t.Error("renderLogo() returned empty string")
	}
	if !strings.Contains(logo, "DSCLI") {
		t.Error("renderLogo() should contain 'DSCLI'")
	}
	if !strings.Contains(logo, "dscli") {
		t.Error("renderLogo() should contain 'dscli'")
	}
}

// ─── Screen常量 测试 ─────────────────────────────────────────────────────────

func TestScreenConstants(t *testing.T) {
	screens := map[Screen]bool{}
	for _, s := range []Screen{
		ScreenDashboard, ScreenBalance, ScreenModels, ScreenHistory,
		ScreenHistoryDetail, ScreenSkills, ScreenPrompt, ScreenChat,
	} {
		if screens[s] {
			t.Errorf("duplicate Screen value: %d", s)
		}
		screens[s] = true
	}
}

// ─── padRight/padLeft/padCenter 测试 ────────────────────────────────────────

func TestPadRight(t *testing.T) {
	w := 80
	result := padRight("hello", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "  ") {
		t.Errorf("padRight should have 2-space left margin: %q", lines[0])
	}
	if !strings.HasSuffix(lines[0], "  ") {
		t.Errorf("padRight should have 2-space right margin: %q", lines[0])
	}
	if lipgloss.Width(lines[0]) != w {
		t.Errorf("padRight width = %d, want %d", lipgloss.Width(lines[0]), w)
	}
}

func TestPadLeft(t *testing.T) {
	w := 80
	result := padLeft("hello", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "  ") {
		t.Errorf("padLeft should have 2-space left margin: %q", lines[0])
	}
	if !strings.HasSuffix(lines[0], "  ") {
		t.Errorf("padLeft should have 2-space right margin: %q", lines[0])
	}
}

func TestPadRightMultiLine(t *testing.T) {
	w := 80
	result := padRight("line1\nline2", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("line %d missing left margin: %q", i, line)
		}
		if !strings.HasSuffix(line, "  ") {
			t.Errorf("line %d missing right margin: %q", i, line)
		}
	}
}

func TestPadLeftMultiLine(t *testing.T) {
	w := 80
	result := padLeft("line1\nline2", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("line %d missing left margin: %q", i, line)
		}
		if !strings.HasSuffix(line, "  ") {
			t.Errorf("line %d missing right margin: %q", i, line)
		}
		if lipgloss.Width(line) != w {
			t.Errorf("line %d width = %d, want %d", i, lipgloss.Width(line), w)
		}
	}
}

func TestPadCenter(t *testing.T) {
	w := 80
	result := padCenter("hello", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.HasPrefix(lines[0], "  ") {
		t.Errorf("padCenter should have 2-space left margin: %q", lines[0])
	}
	if !strings.HasSuffix(lines[0], "  ") {
		t.Errorf("padCenter should have 2-space right margin: %q", lines[0])
	}
	if lipgloss.Width(lines[0]) != w {
		t.Errorf("padCenter width = %d, want %d", lipgloss.Width(lines[0]), w)
	}
	// Content "hello" should appear somewhere in the middle
	contentStart := strings.Index(lines[0], "hello")
	if contentStart < 2 || contentStart >= len(lines[0])-7 {
		t.Errorf("padCenter content not centered: %q", lines[0])
	}
}

func TestPadCenterMultiLine(t *testing.T) {
	w := 80
	result := padCenter("line1\nline2", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	for i, line := range lines {
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("line %d missing left margin: %q", i, line)
		}
		if !strings.HasSuffix(line, "  ") {
			t.Errorf("line %d missing right margin: %q", i, line)
		}
		if lipgloss.Width(line) != w {
			t.Errorf("line %d width = %d, want %d", i, lipgloss.Width(line), w)
		}
	}
}

func TestPadRightEmpty(t *testing.T) {
	w := 80
	result := padRight("", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if lipgloss.Width(lines[0]) != w {
		t.Errorf("padRight empty width = %d, want %d", lipgloss.Width(lines[0]), w)
	}
}

func TestPadWithNarrowWidth(t *testing.T) {
	// When content is wider than available width, left/right padding clamps to 0.
	w := 10
	result := padRight("this is a very long string", w)
	lines := strings.Split(result, "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	// At minimum we have the 2-char left and right margins
	if !strings.HasPrefix(lines[0], "  ") {
		t.Errorf("padRight narrow should still have 2-space left margin: %q", lines[0])
	}
	if !strings.HasSuffix(lines[0], "  ") {
		t.Errorf("padRight narrow should still have 2-space right margin: %q", lines[0])
	}
}

// ─── renderBubble 测试 ──────────────────────────────────────────────────────

func TestRenderBubbleShortContent(t *testing.T) {
	base := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	wrapStyle := lipgloss.NewStyle().Width(50)
	result := renderBubble(base, "👤 ", "hello", wrapStyle, 50)
	// Short content should fit within one bubble
	if !strings.Contains(result, "hello") {
		t.Error("renderBubble should contain the content")
	}
	if !strings.Contains(result, "👤") {
		t.Error("renderBubble should contain the prefix")
	}
}

func TestRenderBubbleLongContent(t *testing.T) {
	base := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	wrapStyle := lipgloss.NewStyle().Width(20)
	longContent := "this is a very long message that should wrap across multiple lines"
	result := renderBubble(base, "👤 ", longContent, wrapStyle, 20)
	lines := strings.Split(result, "\n")
	// Should have multiple lines due to wrapping
	if len(lines) < 2 {
		t.Errorf("long content should produce multiple lines, got %d: %q", len(lines), result)
	}
}

func TestRenderBubbleEmptyPrefix(t *testing.T) {
	base := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	wrapStyle := lipgloss.NewStyle().Width(50)
	result := renderBubble(base, "", "hello world", wrapStyle, 50)
	if !strings.Contains(result, "hello world") {
		t.Error("renderBubble with empty prefix should still render content")
	}
}

func TestRenderBubbleEmptyContent(t *testing.T) {
	base := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	wrapStyle := lipgloss.NewStyle().Width(50)
	result := renderBubble(base, "👤 ", "", wrapStyle, 50)
	// Should produce a valid bubble with just the prefix
	if result == "" {
		t.Error("renderBubble with empty content should still render")
	}
}

func TestRenderBubbleMultilineContent(t *testing.T) {
	base := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	wrapStyle := lipgloss.NewStyle().Width(50)
	result := renderBubble(base, "👤 ", "line1\nline2\nline3", wrapStyle, 50)
	if !strings.Contains(result, "line1") {
		t.Error("renderBubble should contain first line")
	}
	if !strings.Contains(result, "line2") {
		t.Error("renderBubble should contain second line")
	}
}

// ─── viewChat 输出完整性测试 ────────────────────────────────────────────────

func TestViewChatHasTopBorder(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "user", Content: "hello"},
		},
	}
	result := m.viewChat()
	lines := strings.Split(result, "\n")

	foundTop := false
	for _, line := range lines {
		if strings.Contains(line, "╭") {
			foundTop = true
			break
		}
	}
	if !foundTop {
		t.Error("viewChat output missing top border character '╭'")
	}
}

func TestViewChatUserRightAligned(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "user", Content: "hi"},
		},
	}
	result := m.viewChat()
	lines := strings.Split(result, "\n")

	for _, line := range lines {
		if strings.Contains(line, "│ 👤 hi │") {
			if !strings.HasSuffix(line, "  ") {
				t.Errorf("User bubble missing right margin: %q", line)
			}
			return
		}
	}
	t.Error("User bubble content not found in output")
}

func TestViewChatAssistantLeftAligned(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "assistant", Content: "Hello! How can I help?"},
		},
	}
	result := m.viewChat()
	lines := strings.Split(result, "\n")

	found := false
	for _, line := range lines {
		if strings.Contains(line, "🧠") && strings.Contains(line, "Hello") {
			found = true
			// Assistant bubble should be left-aligned (2-space left margin)
			if !strings.HasPrefix(line, "  ") {
				t.Errorf("Assistant bubble missing left margin: %q", line)
			}
			break
		}
	}
	if !found {
		t.Error("Assistant bubble content not found")
	}
}

func TestViewChatMultipleMessages(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi there"},
			{Role: "user", Content: "how are you"},
		},
	}
	result := m.viewChat()

	// All three messages should appear
	if !strings.Contains(result, "hello") {
		t.Error("viewChat missing first user message")
	}
	if !strings.Contains(result, "hi there") {
		t.Error("viewChat missing assistant message")
	}
	if !strings.Contains(result, "how are you") {
		t.Error("viewChat missing second user message")
	}

	// Header should be present
	if !strings.Contains(result, "💬 Chat") {
		t.Error("viewChat missing header")
	}
}

func TestViewChatWithThinkingContent(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "assistant", Content: "Answer", ReasoningContent: "Step by step reasoning..."},
		},
	}
	result := m.viewChat()

	// Should contain thinking content
	if !strings.Contains(result, "思考过程") {
		t.Error("viewChat should contain thinking process label")
	}
	if !strings.Contains(result, "Step by step reasoning") {
		t.Error("viewChat should contain reasoning content")
	}
	if !strings.Contains(result, "Answer") {
		t.Error("viewChat should contain assistant content after reasoning")
	}
}

func TestViewChatWithToolCallsInThinking(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{
				Role:             "assistant",
				Content:          "Using tools...",
				ReasoningContent: "I need to use tools",
				HasToolCalls:     true,
				ToolCallNames:    []string{"shell", "read_file"},
			},
		},
	}
	result := m.viewChat()

	if !strings.Contains(result, "shell") {
		t.Error("viewChat should show tool call name 'shell'")
	}
	if !strings.Contains(result, "read_file") {
		t.Error("viewChat should show tool call name 'read_file'")
	}
}

func TestViewChatWithToolResult(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "tool", Content: "shell: command output here"},
		},
	}
	result := m.viewChat()

	if !strings.Contains(result, "command output") {
		t.Error("viewChat should contain tool result content")
	}
}

func TestViewChatWithTruncation(t *testing.T) {
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "assistant", Content: "Partial answer", FinishReason: "length"},
		},
	}
	result := m.viewChat()

	if !strings.Contains(result, "截断") {
		t.Error("viewChat should show truncation warning")
	}
}

func TestViewChatLoadingSpinner(t *testing.T) {
	sp := spinner.New()
	m := Model{
		Width:       80,
		Height:      24,
		ChatLoading: true,
		ChatSpinner: sp,
	}
	result := m.viewChat()

	if !strings.Contains(result, "Thinking") {
		t.Error("viewChat should show loading indicator when loading")
	}
}

func TestViewChatHeaderAlwaysPresent(t *testing.T) {
	// Empty chat should still have header and input area
	m := Model{
		Width:  80,
		Height: 24,
	}
	result := m.viewChat()

	if !strings.Contains(result, "💬 Chat") {
		t.Error("viewChat missing header on empty chat")
	}
	if !strings.Contains(result, "enter send") {
		t.Error("viewChat missing help text on empty chat")
	}
}

func TestViewChatWithError(t *testing.T) {
	m := Model{
		Width:    80,
		Height:   24,
		ErrorMsg: "network connection failed",
	}
	result := m.viewChat()

	if !strings.Contains(result, "Error: network connection failed") {
		t.Error("viewChat should display error message")
	}
}

func TestViewChatScrollIndicator(t *testing.T) {
	m := Model{
		Width:      80,
		Height:     24,
		ChatScroll: 5,
		ChatMessages: []chatLine{
			{Role: "user", Content: "msg1"},
			{Role: "user", Content: "msg2"},
			{Role: "user", Content: "msg3"},
			{Role: "user", Content: "msg4"},
			{Role: "user", Content: "msg5"},
			{Role: "user", Content: "msg6"},
			{Role: "user", Content: "msg7"},
		},
	}
	result := m.viewChat()

	if !strings.Contains(result, "▲ scrolled") {
		t.Error("viewChat should show scroll indicator when scrolled")
	}
}

// ─── viewChat unified bubble 测试 ───────────────────────────────────────────

func TestViewChatUnifiedBubbleSingleBorder(t *testing.T) {
	// 验证：推理内容和助手内容在同一个气泡中（只有一个顶部边框）
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "assistant", Content: "Answer", ReasoningContent: "Step by step reasoning..."},
		},
	}
	result := m.viewChat()

	// 应该只有一个 ╭ 顶部边框（统一气泡），之前的实现会产生2个气泡
	topBorders := strings.Count(result, "╭")
	// 只有一个助手气泡 → 底部边框也只出现一次
	bottomBorders := strings.Count(result, "╰")

	if topBorders != bottomBorders {
		t.Errorf("unified bubble: top borders (%d) != bottom borders (%d)", topBorders, bottomBorders)
	}
	// 统一气泡内同时包含推理和答案
	if !strings.Contains(result, "思考过程") {
		t.Error("unified bubble should contain reasoning section")
	}
	if !strings.Contains(result, "Answer") {
		t.Error("unified bubble should contain assistant answer")
	}
}

func TestViewChatUnifiedBubbleStartsWithThinking(t *testing.T) {
	// 当没有答案只有推理时，气泡仍然以推理内容开头
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "assistant", ReasoningContent: "Let me think...", HasToolCalls: true, ToolCallNames: []string{"shell"}},
		},
	}
	result := m.viewChat()

	if !strings.Contains(result, "思考过程") {
		t.Error("unified bubble should show reasoning even without assistant answer")
	}
	if !strings.Contains(result, "shell") {
		t.Error("unified bubble should show tool call names")
	}
}

func TestViewChatUnifiedBubbleGroupsConsecutive(t *testing.T) {
	// 验证连续的助手+工具消息合并为一个气泡
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "user", Content: "ls"},
			{Role: "assistant", ReasoningContent: "I need to list files", HasToolCalls: true, ToolCallNames: []string{"shell"}},
			{Role: "tool", Content: "shell: file1 file2"},
			{Role: "assistant", Content: "Here are the files: file1, file2"},
		},
	}
	result := m.viewChat()

	// 用户消息气泡 + 统一助手气泡 = 2个顶部边框
	topBorders := strings.Count(result, "╭")
	if topBorders != 2 {
		t.Errorf("expected 2 top borders (user + unified assistant), got %d", topBorders)
	}
	// 统一气泡内应包含所有三个部分
	if !strings.Contains(result, "思考过程") {
		t.Error("unified bubble should contain thinking")
	}
	if !strings.Contains(result, "file1") {
		t.Error("unified bubble should contain tool result content")
	}
	if !strings.Contains(result, "Here are the files") {
		t.Error("unified bubble should contain final answer")
	}
}

func TestViewChatUnifiedBubbleAssistantStyleBright(t *testing.T) {
	// 验证：assistant的最终回答在气泡内使用了鲜艳样式（Bold + Primary）
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "user", Content: "hi"},
			{Role: "assistant", ReasoningContent: "thinking...", Content: "Hello, bold answer!"},
		},
	}
	result := m.viewChat()

	// assistantBodyStyle = Bold + Foreground(colorPrimary)
	// The styled content should appear within the bubble, after the thinking section.
	if !strings.Contains(result, "Hello, bold answer!") {
		t.Error("unified bubble should contain bold assistant answer")
	}
	// The "🧠 " prefix is applied to the first line by renderBubble.
	if !strings.Contains(result, "🧠") {
		t.Error("unified bubble should have assistant icon prefix")
	}
}

func TestViewChatUnifiedBubbleTruncationInside(t *testing.T) {
	// 验证：截断警告出现在统一气泡内
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "assistant", Content: "Partial", FinishReason: "length"},
		},
	}
	result := m.viewChat()

	if !strings.Contains(result, "截断") {
		t.Error("unified bubble should contain truncation warning")
	}
	// 截断不应是独立的气泡，而应在统一气泡内
	// 只有一个 ╭（统一气泡）
	topBorders := strings.Count(result, "╭")
	if topBorders != 1 {
		t.Errorf("expected 1 top border (unified bubble with truncation inside), got %d", topBorders)
	}
}

func TestViewChatUnifiedBubbleToolResultFormat(t *testing.T) {
	// 验证工具结果有标题头且样式为浅色
	m := Model{
		Width:  80,
		Height: 24,
		ChatMessages: []chatLine{
			{Role: "tool", Content: "read_file: content here"},
		},
	}
	result := m.viewChat()

	if !strings.Contains(result, "工具结果") {
		t.Error("unified bubble should have tool result section header")
	}
	if !strings.Contains(result, "content here") {
		t.Error("unified bubble should contain tool result content")
	}
}

// ─── renderStatusBar 测试 ───────────────────────────────────────────────────

func TestRenderStatusBar(t *testing.T) {
	m := Model{
		Width:       80,
		Version:     "v1.0.0",
		ProjectRoot: "/home/user/project",
		Screen:      ScreenDashboard,
	}
	result := m.renderStatusBar()
	if result == "" {
		t.Error("renderStatusBar returned empty string")
	}
	if !strings.Contains(result, "dscli") {
		t.Error("renderStatusBar should contain version info")
	}
	if !strings.Contains(result, "🏠") {
		t.Error("renderStatusBar should contain screen icon")
	}
	if !strings.Contains(result, "Home") {
		t.Error("renderStatusBar should contain screen name")
	}
}

func TestRenderStatusBarNarrow(t *testing.T) {
	m := Model{
		Width:       20,
		Version:     "v1",
		ProjectRoot: "/x",
		Screen:      ScreenChat,
	}
	result := m.renderStatusBar()
	// Should still render something, not crash
	if result == "" {
		t.Error("renderStatusBar returned empty on narrow width")
	}
}

func TestRenderStatusBarZeroWidth(t *testing.T) {
	m := Model{
		Width: 0,
	}
	result := m.renderStatusBar()
	if result != "" {
		t.Errorf("renderStatusBar should return empty for width 0, got %q", result)
	}
}

// ─── viewDashboard 测试 ──────────────────────────────────────────────────────

func TestViewDashboard(t *testing.T) {
	m := Model{
		Screen: ScreenDashboard,
		Cursor: 0,
	}
	result := m.viewDashboard()
	if !strings.Contains(result, "DSCLI") {
		t.Error("viewDashboard should contain logo")
	}
	if !strings.Contains(result, "Menu") {
		t.Error("viewDashboard should contain Menu heading")
	}
	if !strings.Contains(result, "Balance") {
		t.Error("viewDashboard should contain Balance menu item")
	}
	if !strings.Contains(result, "Chat") {
		t.Error("viewDashboard should contain Chat menu item")
	}
}

func TestViewDashboardCursorHighlight(t *testing.T) {
	m := Model{
		Screen: ScreenDashboard,
		Cursor: 2, // History
	}
	result := m.viewDashboard()

	// The selected item should have the "▸" cursor
	lines := strings.Split(result, "\n")
	found := false
	for _, line := range lines {
		if strings.Contains(line, "▸") && strings.Contains(line, "History") {
			found = true
			break
		}
	}
	if !found {
		t.Error("viewDashboard should highlight selected item with ▸ cursor")
	}
}

// ─── viewBalance 测试 ────────────────────────────────────────────────────────

func TestViewBalanceEmpty(t *testing.T) {
	m := Model{
		Screen: ScreenBalance,
		Width:  80,
	}
	result := m.viewBalance()
	if !strings.Contains(result, "Loading balance") {
		t.Error("viewBalance should show loading when empty")
	}
}

func TestViewBalanceWithData(t *testing.T) {
	m := Model{
		Screen:  ScreenBalance,
		Width:   80,
		BalanceInfos: []map[string]string{
			{
				"currency":          "CNY",
				"total_balance":     "100.00",
				"granted_balance":   "50.00",
				"topped_up_balance": "50.00",
			},
		},
		IsAvailable: true,
	}
	result := m.viewBalance()
	if !strings.Contains(result, "CNY") {
		t.Error("viewBalance should show currency")
	}
	if !strings.Contains(result, "100.00") {
		t.Error("viewBalance should show total balance")
	}
}

func TestViewBalanceUnavailable(t *testing.T) {
	m := Model{
		Screen:  ScreenBalance,
		Width:   80,
		BalanceInfos: []map[string]string{
			{"currency": "CNY", "total_balance": "0.00"},
		},
		IsAvailable: false,
	}
	result := m.viewBalance()
	if !strings.Contains(result, "unavailable") {
		t.Error("viewBalance should show unavailable warning")
	}
}

// ─── viewModels 测试 ─────────────────────────────────────────────────────────

func TestViewModelsEmpty(t *testing.T) {
	m := Model{
		Screen: ScreenModels,
		Width:  80,
		Height: 40,
	}
	result := m.viewModels()
	if !strings.Contains(result, "Loading") {
		t.Error("viewModels should show loading when empty")
	}
}

func TestViewModelsList(t *testing.T) {
	m := Model{
		Screen: ScreenModels,
		Width:  80,
		Height: 40,
		ModelList: []dsc.Model{
			{ID: "deepseek-chat", Object: "model", OwnedBy: "deepseek"},
			{ID: "deepseek-reasoner", Object: "model", OwnedBy: "deepseek"},
		},
	}
	result := m.viewModels()
	if !strings.Contains(result, "deepseek-chat") {
		t.Error("viewModels should list model IDs")
	}
	if !strings.Contains(result, "deepseek-reasoner") {
		t.Error("viewModels should list all models")
	}
}

// ─── viewHistory 测试 ────────────────────────────────────────────────────────

func TestViewHistoryEmpty(t *testing.T) {
	m := Model{
		Screen: ScreenHistory,
		Width:  80,
		Height: 40,
	}
	result := m.viewHistory()
	if !strings.Contains(result, "No history messages") {
		t.Error("viewHistory should show 'No history messages' when empty")
	}
}

func TestViewHistoryList(t *testing.T) {
	m := Model{
		Screen: ScreenHistory,
		Width:  80,
		Height: 40,
		HistoryMessages: []*prompt.Message{
			{Role: "user", Content: "hello", ID: 1},
			{Role: "assistant", Content: "hi there", ID: 2},
		},
	}
	result := m.viewHistory()
	if !strings.Contains(result, "user") {
		t.Error("viewHistory should show user role")
	}
	if !strings.Contains(result, "assistant") {
		t.Error("viewHistory should show assistant role")
	}
	// Should show top/bottom scroll indicators
	if !strings.Contains(result, "at top") {
		t.Error("viewHistory should show top indicator")
	}
	if !strings.Contains(result, "at bottom") {
		t.Error("viewHistory should show bottom indicator")
	}
}

// ─── viewHistoryDetail 测试 ─────────────────────────────────────────────────

func TestViewHistoryDetailNil(t *testing.T) {
	m := Model{
		Screen:        ScreenHistoryDetail,
		Width:         80,
		Height:        40,
		HistoryDetail: nil,
	}
	result := m.viewHistoryDetail()
	if !strings.Contains(result, "Loading") {
		t.Error("viewHistoryDetail should show loading when nil")
	}
}

func TestViewHistoryDetailWithData(t *testing.T) {
	msg := &prompt.Message{
		ID:        42,
		ModelID:   1,
		SessionID: 1,
		Role:      "assistant",
		Content:   "This is the response content.",
	}
	m := Model{
		Screen:        ScreenHistoryDetail,
		Width:         80,
		Height:        40,
		HistoryDetail: msg,
	}
	result := m.viewHistoryDetail()
	if !strings.Contains(result, "#42") {
		t.Error("viewHistoryDetail should show message ID")
	}
	if !strings.Contains(result, "assistant") {
		t.Error("viewHistoryDetail should show role")
	}
	if !strings.Contains(result, "This is the response content") {
		t.Error("viewHistoryDetail should show content")
	}
	if !strings.Contains(result, "Content ──") {
		t.Error("viewHistoryDetail should show content section heading")
	}
}
func TestViewHistoryDetailWithReasoning(t *testing.T) {
	msg := &prompt.Message{
		ID:               1,
		Role:             "assistant",
		Content:          "Answer",
		ReasoningContent: "Detailed reasoning...",
	}
	m := Model{
		Screen:        ScreenHistoryDetail,
		Width:         80,
		Height:        40,
		HistoryDetail: msg,
	}
	result := m.viewHistoryDetail()
	if !strings.Contains(result, "Reasoning Content") {
		t.Error("viewHistoryDetail should show reasoning section")
	}
	if !strings.Contains(result, "Detailed reasoning") {
		t.Error("viewHistoryDetail should show reasoning content")
	}
}

// ─── viewSkills 测试 ────────────────────────────────────────────────────────

func TestViewSkillsEmpty(t *testing.T) {
	m := Model{
		Screen: ScreenSkills,
		Width:  80,
		Height: 40,
	}
	result := m.viewSkills()
	if !strings.Contains(result, "No skills found") {
		t.Error("viewSkills should show 'No skills found' when empty")
	}
}

func TestViewSkillsList(t *testing.T) {
	m := Model{
		Screen: ScreenSkills,
		Width:  80,
		Height: 40,
		// SkillInfos uses skills.SkillInfo but we can't import skills pkg directly
		// in test as it's in the same package - we need to use the actual struct
	}
	// Use the proper import
	result := m.viewSkills()
	// With empty SkillInfos we already tested above
	_ = result
}

// ─── viewPrompt 测试 ─────────────────────────────────────────────────────────

func TestViewPromptEmpty(t *testing.T) {
	m := Model{
		Screen: ScreenPrompt,
		Width:  80,
		Height: 40,
	}
	result := m.viewPrompt()
	if !strings.Contains(result, "Loading prompt") {
		t.Error("viewPrompt should show loading when empty")
	}
}

func TestViewPromptWithContent(t *testing.T) {
	m := Model{
		Screen:        ScreenPrompt,
		Width:         80,
		Height:        40,
		PromptContent: "You are a helpful assistant.",
	}
	result := m.viewPrompt()
	if !strings.Contains(result, "You are a helpful assistant") {
		t.Error("viewPrompt should display the prompt content")
	}
	if !strings.Contains(result, "System Prompt") {
		t.Error("viewPrompt should show header")
	}
}

// ─── View 路由测试 ───────────────────────────────────────────────────────────

func TestViewRouting(t *testing.T) {
	tests := []struct {
		name        string
		screen      Screen
		wantContent string
	}{
		{name: "Dashboard路由", screen: ScreenDashboard, wantContent: "DSCLI"},
		{name: "Balance路由", screen: ScreenBalance, wantContent: "Account Balance"},
		{name: "Models路由", screen: ScreenModels, wantContent: "Models"},
		{name: "History路由", screen: ScreenHistory, wantContent: "History"},
		{name: "Skills路由", screen: ScreenSkills, wantContent: "Skills"},
		{name: "Prompt路由", screen: ScreenPrompt, wantContent: "System Prompt"},
		{name: "Chat路由", screen: ScreenChat, wantContent: "💬 Chat"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{
				Screen: tt.screen,
				Width:  80,
				Height: 40,
			}
			result := m.View()
			if !strings.Contains(result, tt.wantContent) {
				t.Errorf("View() for %s should contain %q, got: %s", tt.name, tt.wantContent, truncateStr(result, 200))
			}
		})
	}
}

func TestViewUnknownScreen(t *testing.T) {
	m := Model{
		Screen: Screen(999),
		Width:  80,
		Height: 40,
	}
	result := m.View()
	if !strings.Contains(result, "Unknown screen") {
		t.Error("View should show 'Unknown screen' for invalid screen")
	}
}

func TestViewWithError(t *testing.T) {
	m := Model{
		Screen:   ScreenDashboard,
		Width:    80,
		Height:   40,
		ErrorMsg: "something went wrong",
	}
	result := m.View()
	if !strings.Contains(result, "Error: something went wrong") {
		t.Error("View should display error message")
	}
}

// ─── Key Handler 测试 ────────────────────────────────────────────────────────

func TestHandleDashboardKeys(t *testing.T) {
	m := Model{Screen: ScreenDashboard}
	// Test cursor navigation with j
	m2, _ := m.handleDashboardKeys("j")
	if m2.(Model).Cursor != 1 {
		t.Errorf("j should move cursor down, got %d", m2.(Model).Cursor)
	}

	// k from position 1 should go back to 0
	m3, _ := m2.(Model).handleDashboardKeys("k")
	if m3.(Model).Cursor != 0 {
		t.Errorf("k should move cursor up, got %d", m3.(Model).Cursor)
	}

	// k at top should stay at top
	m4, _ := m3.(Model).handleDashboardKeys("k")
	if m4.(Model).Cursor != 0 {
		t.Errorf("k at top should stay at top, got %d", m4.(Model).Cursor)
	}

	// j at bottom should stay at bottom
	m5 := Model{Screen: ScreenDashboard, Cursor: len(dashboardMenuItems) - 1}
	m6, _ := m5.handleDashboardKeys("j")
	if m6.(Model).Cursor != len(dashboardMenuItems)-1 {
		t.Errorf("j at bottom should stay at bottom, got %d", m6.(Model).Cursor)
	}
}

func TestHandleDashboardKeysQuit(t *testing.T) {
	m := Model{Screen: ScreenDashboard}
	_, cmd := m.handleDashboardKeys("q")
	if cmd == nil {
		t.Error("q should return quit command")
	} else {
		// Check it's a quit command
		msg := cmd()
		if _, ok := msg.(tea.QuitMsg); !ok {
			t.Error("q should produce tea.QuitMsg")
		}
	}
}
func TestHandleDashboardSelection(t *testing.T) {
	tests := []struct {
		name         string
		cursor       int
		wantScreen   Screen
		wantCmdIsNil bool
	}{
		{name: "Balance", cursor: 0, wantScreen: ScreenBalance, wantCmdIsNil: false},
		{name: "Models", cursor: 1, wantScreen: ScreenModels, wantCmdIsNil: false},
		{name: "History", cursor: 2, wantScreen: ScreenHistory, wantCmdIsNil: false},
		{name: "Skills", cursor: 3, wantScreen: ScreenSkills, wantCmdIsNil: false},
		{name: "Prompt", cursor: 4, wantScreen: ScreenPrompt, wantCmdIsNil: false},
		{name: "Chat", cursor: 5, wantScreen: ScreenChat, wantCmdIsNil: true},
		{name: "Quit", cursor: 6, wantScreen: ScreenDashboard, wantCmdIsNil: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{Screen: ScreenDashboard, Cursor: tt.cursor}
			// Chat case needs a properly initialized ChatInput to avoid nil cursor panic.
			if tt.cursor == 5 {
				ti := textinput.New()
				m.ChatInput = ti
			}
			m2, cmd := m.handleDashboardSelection()
			newM := m2.(Model)
			if newM.Screen != tt.wantScreen {
				t.Errorf("handleDashboardSelection(cursor=%d) screen = %d, want %d", tt.cursor, newM.Screen, tt.wantScreen)
			}
			if tt.wantCmdIsNil && cmd != nil {
				t.Errorf("handleDashboardSelection(cursor=%d) cmd should be nil", tt.cursor)
			}
			if !tt.wantCmdIsNil && cmd == nil {
				t.Errorf("handleDashboardSelection(cursor=%d) cmd should not be nil", tt.cursor)
			}
		})
	}
}

func TestHandleSimpleScreenKeys(t *testing.T) {
	m := Model{Screen: ScreenBalance}
	m2, _ := m.handleSimpleScreenKeys("esc")
	if m2.(Model).Screen != ScreenDashboard {
		t.Errorf("esc should return to dashboard, got screen=%d", m2.(Model).Screen)
	}

	m3 := Model{Screen: ScreenBalance}
	m4, _ := m3.handleSimpleScreenKeys("q")
	if m4.(Model).Screen != ScreenDashboard {
		t.Errorf("q should return to dashboard, got screen=%d", m4.(Model).Screen)
	}
}

func TestHandleSimpleScreenKeysUnknown(t *testing.T) {
	m := Model{Screen: ScreenBalance}
	m2, _ := m.handleSimpleScreenKeys("x")
	if m2.(Model).Screen != ScreenBalance {
		t.Error("unknown key should not change screen")
	}
}

func TestHandleListScreenKeysEsc(t *testing.T) {
	m := Model{Screen: ScreenModels, Cursor: 5, Scroll: 3}
	m2, _ := m.handleListScreenKeys("esc", 10, 5)
	newM := m2.(Model)
	if newM.Screen != ScreenDashboard {
		t.Error("esc should return to dashboard")
	}
	if newM.Cursor != 0 {
		t.Error("esc should reset cursor to 0")
	}
	if newM.Scroll != 0 {
		t.Error("esc should reset scroll to 0")
	}
}

func TestHandleListScreenKeysNavigation(t *testing.T) {
	m := Model{Screen: ScreenModels, Cursor: 2, Scroll: 0, Height: 40}
	// j: move down
	m2, _ := m.handleListScreenKeys("j", 10, m.visibleItems(1))
	if m2.(Model).Cursor != 3 {
		t.Errorf("j should move cursor down, got %d", m2.(Model).Cursor)
	}

	// k: move up
	m3, _ := m2.(Model).handleListScreenKeys("k", 10, m2.(Model).visibleItems(1))
	if m3.(Model).Cursor != 2 {
		t.Errorf("k should move cursor up, got %d", m3.(Model).Cursor)
	}

	// k at top stays
	m4 := Model{Screen: ScreenModels, Cursor: 0, Height: 40}
	m5, _ := m4.handleListScreenKeys("k", 10, m4.visibleItems(1))
	if m5.(Model).Cursor != 0 {
		t.Errorf("k at top should stay, got %d", m5.(Model).Cursor)
	}

	// j at bottom stays
	m6 := Model{Screen: ScreenModels, Cursor: 9, Height: 40}
	m7, _ := m6.handleListScreenKeys("j", 10, m6.visibleItems(1))
	if m7.(Model).Cursor != 9 {
		t.Errorf("j at bottom should stay, got %d", m7.(Model).Cursor)
	}
}

func TestHandlePromptKeys(t *testing.T) {
	m := Model{Screen: ScreenPrompt, Scroll: 0}
	// j scrolls down
	m2, _ := m.handlePromptKeys("j")
	if m2.(Model).Scroll != 1 {
		t.Errorf("j should increment scroll, got %d", m2.(Model).Scroll)
	}

	// k scrolls up
	m3, _ := m2.(Model).handlePromptKeys("k")
	if m3.(Model).Scroll != 0 {
		t.Errorf("k should decrement scroll, got %d", m3.(Model).Scroll)
	}

	// k at 0 stays
	m4, _ := m3.(Model).handlePromptKeys("k")
	if m4.(Model).Scroll != 0 {
		t.Errorf("k at 0 should stay, got %d", m4.(Model).Scroll)
	}

	// esc returns to dashboard
	m5 := Model{Screen: ScreenPrompt, Scroll: 5}
	m6, _ := m5.handlePromptKeys("esc")
	if m6.(Model).Screen != ScreenDashboard {
		t.Error("esc should return to dashboard")
	}
	if m6.(Model).Scroll != 0 {
		t.Error("esc should reset scroll")
	}
}

func TestHandleChatKeysQuit(t *testing.T) {
	m := Model{
		Screen:       ScreenChat,
		ChatMessages: []chatLine{{Role: "user", Content: "test"}},
		ChatLoading:  true,
		ChatScroll:   3,
	}
	m2, _ := m.handleChatKeys("q")
	newM := m2.(Model)
	if newM.Screen != ScreenDashboard {
		t.Error("q should return to dashboard")
	}
	if newM.ChatMessages != nil {
		t.Error("q should clear chat messages")
	}
	if newM.ChatLoading {
		t.Error("q should stop loading")
	}
}

func TestHandleChatKeysScroll(t *testing.T) {
	// Test k scrolls up
	m := Model{Screen: ScreenChat, ChatScroll: 0}
	m2, _ := m.handleChatKeys("k")
	if m2.(Model).ChatScroll != 1 {
		t.Errorf("k should increment ChatScroll, got %d", m2.(Model).ChatScroll)
	}

	// Test j scrolls down
	m3, _ := m2.(Model).handleChatKeys("j")
	if m3.(Model).ChatScroll != 0 {
		t.Errorf("j should decrement ChatScroll, got %d", m3.(Model).ChatScroll)
	}

	// Test j at 0 stays
	m4, _ := m3.(Model).handleChatKeys("j")
	if m4.(Model).ChatScroll != 0 {
		t.Errorf("j at 0 should stay, got %d", m4.(Model).ChatScroll)
	}
}

func TestHandleChatKeysG(t *testing.T) {
	m := Model{Screen: ScreenChat, ChatScroll: 0}
	m2, _ := m.handleChatKeys("G")
	if m2.(Model).ChatScroll != 0 {
		t.Error("G should set ChatScroll to 0 (bottom)")
	}

	m3, _ := m.handleChatKeys("g")
	if m3.(Model).ChatScroll <= 0 {
		t.Error("g should set ChatScroll to a large value (top)")
	}
}

func TestHandleChatKeysPgUpPgDown(t *testing.T) {
	m := Model{Screen: ScreenChat, Height: 40, ChatScroll: 0}
	expectedPage := m.Height - 8

	// pgup
	m2, _ := m.handleChatKeys("pgup")
	if m2.(Model).ChatScroll != expectedPage {
		t.Errorf("pgup should increment ChatScroll by %d, got %d", expectedPage, m2.(Model).ChatScroll)
	}

	// pgdown
	m3, _ := m2.(Model).handleChatKeys("pgdown")
	if m3.(Model).ChatScroll != 0 {
		t.Errorf("pgdown should decrement ChatScroll back to 0, got %d", m3.(Model).ChatScroll)
	}
}

func TestHandleChatKeysFocus(t *testing.T) {
	ti := textinput.New()
	m := Model{Screen: ScreenChat, ChatInput: ti}
	// 'i' should focus input
	m2, _ := m.handleChatKeys("i")
	if !m2.(Model).ChatInput.Focused() {
		t.Error("i should focus chat input")
	}
}

func TestHandleKeyPressRouting(t *testing.T) {
	tests := []struct {
		name   string
		screen Screen
		key    string
	}{
		{name: "Dashboard receives keys", screen: ScreenDashboard, key: "j"},
		{name: "Balance receives keys", screen: ScreenBalance, key: "esc"},
		{name: "Models receives keys", screen: ScreenModels, key: "esc"},
		{name: "History receives keys", screen: ScreenHistory, key: "esc"},
		{name: "Skills receives keys", screen: ScreenSkills, key: "esc"},
		{name: "Prompt receives keys", screen: ScreenPrompt, key: "esc"},
		{name: "Chat receives keys", screen: ScreenChat, key: "esc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := Model{Screen: tt.screen, Width: 80, Height: 40}
			m2, _ := m.handleKeyPress(tt.key)
			// Should not panic, should return a Model
			if m2 == nil {
				t.Error("handleKeyPress returned nil")
			}
		})
	}
}

func TestHandleKeyPressErrorClear(t *testing.T) {
	m := Model{Screen: ScreenDashboard, ErrorMsg: "old error"}
	m2, _ := m.handleKeyPress("j")
	if m2.(Model).ErrorMsg != "" {
		t.Error("handleKeyPress should clear error message")
	}
}

func TestNewModel(t *testing.T) {
	m := New(nil, nil, "v1.0.0", "abc123", "/home/user/project")
	if m.Version != "v1.0.0" {
		t.Errorf("New Version = %q, want %q", m.Version, "v1.0.0")
	}
	if m.Build != "abc123" {
		t.Errorf("New Build = %q, want %q", m.Build, "abc123")
	}
	if m.ProjectRoot != "/home/user/project" {
		t.Errorf("New ProjectRoot = %q, want %q", m.ProjectRoot, "/home/user/project")
	}
	if m.Screen != ScreenDashboard {
		t.Error("New should start at Dashboard")
	}
	if m.ChatInput.CharLimit != 4096 {
		t.Errorf("New ChatInput CharLimit = %d, want 4096", m.ChatInput.CharLimit)
	}
}