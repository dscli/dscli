package lp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExtractAfterMessage(t *testing.T) {
	msg := "请只回复四个字：在线确认"
	tests := []struct {
		name    string
		current string
		message string
		want    string
	}{
		{
			name:    "message followed by marker and answer",
			current: "在线确认\n快速模式\n" + msg + "\n已思考（用时 1 秒）\n\n我们根据要求只回复四个字。\n\n在线确认\n\n深度思考\n智能搜索\n内容由 AI 生成，请仔细甄别",
			message: msg,
			want:    "我们根据要求只回复四个字。\n\n在线确认\n\n深度思考\n智能搜索\n内容由 AI 生成，请仔细甄别",
		},
		{
			name:    "no marker, answer directly after message",
			current: "sidebar\n" + msg + "\n答案是：你好世界",
			message: msg,
			want:    "答案是：你好世界",
		},
		{
			name: "continued conversation: old rounds above must not anchor",
			current: "old msg\n已思考（用时 3 秒）\n旧答案\n" + msg +
				"\n已思考（用时 2 秒）\n新答案",
			message: msg,
			want:    "新答案",
		},
		{
			name:    "answer quoting the message still extracts from the chat copy",
			current: "标题\n" + msg + "\n已思考（用时 1 秒）\n你问的是“" + msg + "”，答案是 42",
			message: msg,
			want:    "你问的是“" + msg + "”，答案是 42",
		},
		{
			name:    "answer begins with the message text after the marker",
			current: msg + "\n已思考（用时 1 秒）\n" + msg + "\n答案：42",
			message: msg,
			want:    msg + "\n答案：42",
		},
		{
			name:    "no match",
			current: "completely different page",
			message: msg,
			want:    "",
		},
		{
			name:    "empty current",
			current: "",
			message: msg,
			want:    "",
		},
		{
			name:    "crlf normalization",
			current: "x\r\n" + msg + "\r\n已思考 12s\r\n答案",
			message: "x\n" + msg,
			want:    "答案",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAfterMessage(tt.current, tt.message); got != tt.want {
				t.Errorf("extractAfterMessage() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCleanBodyResponse(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "strips footer labels and thinking marker",
			in:   "已思考（用时 1 秒）\n\n这是答案正文。\n\n深度思考\n智能搜索\n内容由 AI 生成，请仔细甄别",
			want: "这是答案正文。",
		},
		{
			name: "citation reference line stripped",
			in:   "答案\n- 2\n继续",
			want: "答案\n继续",
		},
		{
			name: "no noise",
			in:   "直接答案，没有任何噪声。",
			want: "直接答案，没有任何噪声。",
		},
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "english think label variants",
			in:   "已思考 12s\nHello world",
			want: "Hello world",
		},
		{
			name: "quoted marker inside answer is kept",
			in:   "模型说“已思考（用时 1 秒）”不能省略",
			want: "模型说“已思考（用时 1 秒）”不能省略",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanBodyResponse(tt.in); got != tt.want {
				t.Errorf("cleanBodyResponse(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsCompleteResponse(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{name: "empty", s: "", want: false},
		{name: "replacement char", s: "\uFFFD", want: false},
		// The model pauses after emitting a simulated tool call that the
		// web UI cannot execute; the fragment must not be returned.
		{name: "tool call fragment", s: "<read_file path=\"AGENTS.md\" />", want: false},
		{name: "quoted tool call fragment", s: "> <read_file path=\"AGENTS.md\" />", want: false},
		// A full answer that includes the simulated call plus body is fine.
		{name: "full review with tool call", s: "> <read_file path=\"AGENTS.md\" />\n\n> <tool_result>\n# AGENTS.md\n\n## Overall Assessment\nSolid.", want: true},
		{name: "short with tool result", s: "<read_file />\n<tool_result>ok</tool_result>", want: true},
		// A genuine short answer with an XML-like tag must not be rejected
		// as a tool-call fragment (regression guard for the whitelist).
		{name: "short answer with html tag", s: "<b>bold</b> is fine", want: true},
		{name: "plain answer", s: "The change looks correct.", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCompleteResponse(tt.s); got != tt.want {
				t.Errorf("isCompleteResponse(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStripBaselinePrefix(t *testing.T) {
	tests := []struct {
		name string
		resp string
		base string
		want string
	}{
		{name: "prefix stripped", resp: "history\nnew", base: "history", want: "new"},
		{name: "no prefix", resp: "history\nnew", base: "other", want: "history\nnew"},
		{name: "empty baseline", resp: "new", base: "", want: "new"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripBaselinePrefix(tt.resp, tt.base); got != tt.want {
				t.Errorf("stripBaselinePrefix(%q, %q) = %q, want %q", tt.resp, tt.base, got, tt.want)
			}
		})
	}
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"photo.png", true},
		{"photo.PNG", true}, // case-insensitive
		{"photo.jpg", true},
		{"photo.jpeg", true},
		{"photo.gif", true},
		{"photo.webp", true},
		{"photo.bmp", true},
		{"doc.txt", false},
		{"archive.tar.gz", false},
		{"noext", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := IsImageFile(tt.path); got != tt.want {
				t.Errorf("IsImageFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestNormalizeWebChatOptions(t *testing.T) {
	tests := []struct {
		name string
		opts WebChatOptions
		want Mode
	}{
		{"new conversation defaults to pro", WebChatOptions{}, ModePro},
		{"attachments imply vision", WebChatOptions{Attachments: []string{"a.png"}}, ModeVision},
		{"continued conversation preserves mode", WebChatOptions{Keep: "last"}, ""},
		{"explicit mode wins", WebChatOptions{Mode: ModeFlash, Attachments: []string{"a.png"}}, ModeFlash},
		{"explicit pro with keep", WebChatOptions{Mode: ModePro, Keep: "last"}, ModePro},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeWebChatOptions(tt.opts).Mode; got != tt.want {
				t.Errorf("normalizeWebChatOptions(%+v).Mode = %q, want %q", tt.opts, got, tt.want)
			}
		})
	}
}

func TestValidateWebChatOptions(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	if err := os.WriteFile(img, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := []WebChatOptions{
		{Mode: ModePro},
		{Mode: ModeFlash, Attachments: []string{img}},
		{Mode: ModeVision, Attachments: []string{img}},
		{Mode: ""}, // auto
	}
	for _, opts := range valid {
		if err := validateWebChatOptions(opts); err != nil {
			t.Errorf("validateWebChatOptions(%+v) = %v, want nil", opts, err)
		}
	}
	if err := validateWebChatOptions(WebChatOptions{Mode: "turbo"}); err == nil {
		t.Error("unknown mode must fail")
	}
	if err := validateWebChatOptions(WebChatOptions{Mode: ModePro, Attachments: []string{img}}); err == nil {
		t.Error("pro with attachments must fail")
	}
}

func TestResolveWebAttachments(t *testing.T) {
	// Empty input stays untouched (nil stays nil, empty stays empty).
	if got := mustResolve(t, nil); got != nil {
		t.Errorf("resolveWebAttachments(nil) = %v, want nil", got)
	}
	if got := mustResolve(t, []string{}); len(got) != 0 {
		t.Errorf("resolveWebAttachments([]) = %v, want empty", got)
	}

	// Absolute paths are kept as-is.
	abs := filepath.Join(t.TempDir(), "shot.png")
	if got := mustResolve(t, []string{abs}); got[0] != abs {
		t.Errorf("resolveWebAttachments(%q) = %q, want unchanged", abs, got[0])
	}

	// Relative paths are resolved against the process cwd — Chrome's CDP
	// upload reads files with Chrome's working directory, not dscli's.
	rel := "relative-shot.png"
	got := mustResolve(t, []string{rel})
	want, _ := filepath.Abs(rel)
	if got[0] != want {
		t.Errorf("resolveWebAttachments(%q) = %q, want %q", rel, got[0], want)
	}
	if !filepath.IsAbs(got[0]) {
		t.Errorf("resolveWebAttachments(%q) = %q, want absolute path", rel, got[0])
	}
}

func mustResolve(t *testing.T, files []string) []string {
	t.Helper()
	resolved, err := resolveWebAttachments(files)
	if err != nil {
		t.Fatalf("resolveWebAttachments(%v): %v", files, err)
	}
	return resolved
}

func TestValidateWebAttachments(t *testing.T) {
	// Too many files (count check runs before the stat loop).
	files := make([]string, webUploadMaxFiles+1)
	for i := range files {
		files[i] = fmt.Sprintf("f%d.png", i)
	}
	if err := validateWebAttachments(files); err == nil {
		t.Error("too many attachments must fail")
	}

	// Missing file.
	if err := validateWebAttachments([]string{"no-such-file.png"}); err == nil {
		t.Error("missing attachment must fail")
	}

	dir := t.TempDir()
	// Total size limit: a sparse file reports its logical size without
	// using disk space.
	big := filepath.Join(dir, "big.png")
	if err := os.WriteFile(big, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Truncate(big, webUploadMaxTotal+1); err != nil {
		t.Fatal(err)
	}
	if err := validateWebAttachments([]string{big}); err == nil {
		t.Error("oversized attachments must fail")
	}

	// Valid single small file.
	small := filepath.Join(dir, "small.png")
	if err := os.WriteFile(small, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWebAttachments([]string{small}); err != nil {
		t.Errorf("small attachment must pass: %v", err)
	}
}

func TestConversationIDFromURL(t *testing.T) {
	tests := []struct {
		url  string
		want string
	}{
		{"https://chat.deepseek.com/a/chat/s/abc123", "abc123"},
		{"https://chat.deepseek.com/a/chat/s/a1b2-c3d4_e5", "a1b2-c3d4_e5"},
		{"https://chat.deepseek.com/a/chat/s/abc123?extra=1", "abc123"},
		{"https://chat.deepseek.com/a/chat/s/abc123#frag", "abc123"},
		{"https://chat.deepseek.com/", ""},
		{"", ""},
		{"not a url", ""},
		// Non-DeepSeek hosts and lookalike paths must not match.
		{"https://evil.example.com/a/chat/s/abc123", ""},
		{"https://chat.deepseek.com/other/s/abc123", ""},
		// Weird input must not panic or match.
		{"https://chat.deepseek.com/a/chat/s/", ""},
		{"https://chat.deepseek.com/a/chat/s/%2e%2e", ""},
	}
	for _, tt := range tests {
		if got := ConversationIDFromURL(tt.url); got != tt.want {
			t.Errorf("ConversationIDFromURL(%q) = %q, want %q", tt.url, got, tt.want)
		}
	}
}

// testRegistry builds a registry with the given id → url entries.
func testRegistry(entries map[string]string) *conversationRegistry {
	reg := &conversationRegistry{Sessions: map[string]conversationEntry{}}
	for id, url := range entries {
		reg.Sessions[id] = conversationEntry{URL: url}
	}
	return reg
}

func TestRegistryResolve(t *testing.T) {
	reg := testRegistry(map[string]string{
		"aaa": "https://chat.deepseek.com/a/chat/s/aaa",
		"bbb": "https://chat.deepseek.com/a/chat/s/bbb",
	})
	reg.Sessions["aaa"] = conversationEntry{
		URL:       "https://chat.deepseek.com/a/chat/s/aaa",
		UpdatedAt: "2026-08-01T00:00:00Z",
	}
	reg.Sessions["bbb"] = conversationEntry{
		URL:       "https://chat.deepseek.com/a/chat/s/bbb",
		UpdatedAt: "2026-08-02T00:00:00Z",
	}

	tests := []struct {
		name string
		keep string
		want string
	}{
		{"empty = new conversation", "", ""},
		{"last picks most recent", "last", "https://chat.deepseek.com/a/chat/s/bbb"},
		{"exact id", "aaa", "https://chat.deepseek.com/a/chat/s/aaa"},
		{"full url passthrough", "https://chat.deepseek.com/a/chat/s/zzz", "https://chat.deepseek.com/a/chat/s/zzz"},
		{"url suffix match", "s/bbb", "https://chat.deepseek.com/a/chat/s/bbb"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := reg.resolve(tt.keep)
			if err != nil {
				t.Fatalf("resolve(%q): %v", tt.keep, err)
			}
			if got != tt.want {
				t.Errorf("resolve(%q) = %q, want %q", tt.keep, got, tt.want)
			}
		})
	}

	if _, err := reg.resolve("nope"); err == nil {
		t.Error("unknown id must fail")
	}
}

func TestRegistryResolveAmbiguousSuffix(t *testing.T) {
	// Two entries whose URLs both end in /s/dup: a bare "dup" shorthand is
	// ambiguous and must fail loudly instead of picking one arbitrarily.
	reg := testRegistry(map[string]string{
		"one": "https://chat.deepseek.com/a/chat/s/dup",
		"two": "https://chat.deepseek.com/a/chat/s/other/dup",
	})
	_, err := reg.resolve("dup")
	if err == nil {
		t.Fatal("ambiguous suffix must fail")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %q, want ambiguous message", err)
	}
	// The full ID still resolves.
	got, err := reg.resolve("one")
	if err != nil || got != "https://chat.deepseek.com/a/chat/s/dup" {
		t.Errorf("resolve(one) = %q, %v; want exact URL, nil", got, err)
	}
}

func TestRegistryResolveEmptyRegistry(t *testing.T) {
	reg := testRegistry(nil)
	if _, err := reg.resolve("last"); err == nil {
		t.Error("last on empty registry must fail")
	}
	if got, err := reg.resolve(""); err != nil || got != "" {
		t.Errorf("resolve(\"\") = %q, %v; want \"\", nil", got, err)
	}
}

func TestRegistryTrim(t *testing.T) {
	reg := &conversationRegistry{Sessions: map[string]conversationEntry{}}
	// Fill beyond the cap; later timestamps are newer and must survive.
	for i := 0; i < maxSavedConversations+10; i++ {
		id := fmt.Sprintf("id%03d", i)
		reg.Sessions[id] = conversationEntry{
			URL:       "https://chat.deepseek.com/a/chat/s/" + id,
			UpdatedAt: fmt.Sprintf("2026-08-01T%02d:%02d:00Z", i/60, i%60),
		}
	}
	reg.trim()
	if len(reg.Sessions) != maxSavedConversations {
		t.Fatalf("trim: %d entries, want %d", len(reg.Sessions), maxSavedConversations)
	}
	// The 10 oldest (id000..id009) must be gone.
	if _, ok := reg.Sessions["id000"]; ok {
		t.Error("oldest entry id000 survived trim")
	}
	if _, ok := reg.Sessions["id010"]; !ok {
		t.Error("newest entry id010 was trimmed away")
	}
}

func TestIsBusyErrorText(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		// Short texts matching known overload phrases are busy errors.
		{name: "zh server busy", s: "服务器忙，请稍后再试", want: true},
		{name: "zh server busy variant", s: "服务器繁忙，请稍后再试", want: true},
		{name: "zh system busy", s: "系统繁忙，请稍后重试", want: true},
		{name: "zh rate limit", s: "请求过于频繁，请稍后再试", want: true},
		{name: "zh network error", s: "网络异常，请稍后再试", want: true},
		{name: "zh send failed", s: "发送失败，请稍后重试", want: true},
		{name: "zh unavailable", s: "服务暂不可用，请稍候再试", want: true},
		{name: "en server busy", s: "The server is busy. Please try again later.", want: true},
		{name: "en 429", s: "Too Many Requests: rate limit exceeded", want: true},
		{name: "en 503", s: "503 Service Unavailable", want: true},
		{name: "en overloaded", s: "The server is overloaded. Retry after a brief wait.", want: true},
		// A long response is a real answer even if it mentions a phrase.
		{name: "long answer mentioning phrase", s: "The recommendation is to try again later. " + strings.Repeat("详细分析：", 100), want: false},
		// Normal answers are not busy errors.
		{name: "normal answer", s: "The change looks correct. Ship it.", want: false},
		{name: "empty", s: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBusyErrorText(tt.s); got != tt.want {
				t.Errorf("isBusyErrorText(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestIsTruncated(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		// Unclosed markdown code fence: cut-off code block.
		{name: "cut-off json fence", s: "```json\n{\"a\": 1", want: true},
		{name: "cut-off plain fence", s: "```python\nprint('hello'", want: true},
		{name: "closed code block", s: "```json\n{\"a\": 1}\n```", want: false},
		{name: "interior fence never closed", s: "text\n```\ncode\n```\nthen ``` more", want: true},
		// A lone fence inside prose explains the syntax, not truncation.
		{name: "lone fence in prose", s: "Use ``` to open a code block.", want: false},
		// JSON that never terminates.
		{name: "truncated object", s: "{\"question\": \"1\", \"answer\": \"A\"", want: true},
		{name: "truncated array of objects", s: "[{\"id\": 1}, {\"id\": 2}", want: true},
		{name: "complete object", s: "{\"question\": \"1\", \"answer\": \"A\"}", want: false},
		{name: "complete array", s: "[{\"id\": 1}, {\"id\": 2}]", want: false},
		// DeepSeek renders a ```json fence as a code-block toolbar whose
		// labels (language name + Copy/Download buttons) prefix the code.
		// The noise must not defeat the JSON check (#30 blind spot).
		{name: "truncated json with toolbar prefix", s: "json\nCopy\nDownload\n{\"questions\": [1], \"answers\": {\"3\": \"C\"", want: true},
		{name: "complete json with toolbar prefix", s: "json\nCopy\nDownload\n{\"questions\": [1], \"answers\": {\"3\": \"C\"}}", want: false},
		{name: "truncated array with toolbar prefix", s: "json\nCopy\nDownload\n[{\"id\": 1}, {\"id\": 2}", want: true},
		{name: "lowercase toolbar labels", s: "json\ncopy\ndownload\n{\"a\": 1", want: true},
		{name: "chinese toolbar labels", s: "json\n复制\n下载\n{\"a\": 1", want: true},
		{name: "toolbar prefix with quoted key only", s: "json\nCopy\nDownload\n{\"a\": 1", want: true},
		// Prose that starts with a toolbar-like word must stay untouched.
		{name: "prose starting with Download", s: "Download the release from the link below.", want: false},
		{name: "prose starting with json", s: "json is a data interchange format.", want: false},
		{name: "plain answer", s: "The change looks correct. Ship it.", want: false},
		{name: "empty", s: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTruncated(tt.s); got != tt.want {
				t.Errorf("isTruncated(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

// TestAnswerUsable covers the shared acceptance gate used by BOTH the DOM
// path (webchatWait) and the IDB path (webchatExtractReloadedIDB): a server
// overload notice or a truncated reply must never be accepted as an answer.
func TestAnswerUsable(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want bool
	}{
		{name: "busy notice rejected", s: "服务器繁忙，请稍后再试", want: false},
		{name: "english busy notice rejected", s: "Service is busy, please try again later.", want: false},
		{name: "cut-off code fence rejected", s: "```python\nprint('hello'", want: false},
		{name: "truncated json rejected", s: "{\"question\": \"1\", \"answer\": \"A\"", want: false},
		{name: "short plain answer accepted", s: "好的", want: true},
		{name: "long answer accepted", s: "这是一段完整的回答。" + strings.Repeat("内容", 300), want: true},
		{name: "empty rejected", s: "", want: false},
		{name: "tool-call opener without result rejected", s: "<read_file src=\"x.go\">", want: false},
		// Combined: busy wins on short-circuit regardless of the
		// truncation signal; the gate must never accept either.
		{name: "busy plus cut-off fence rejected", s: "服务器繁忙 ```python\nprint('x'", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := answerUsable(tt.s); got != tt.want {
				t.Errorf("answerUsable(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestStripUIChromePrefix(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"toolbar prefix stripped", "json\nCopy\nDownload\n{code", "{code"},
		{"partial toolbar stripped", "json\nCopy\n{code", "{code"},
		{"only language label", "json\n{code", "{code"},
		{"no noise unchanged", "{\"a\": 1}", "{\"a\": 1}"},
		{"prose unchanged", "Copy the file to /tmp.", "Copy the file to /tmp."},
		{"mid-text noise kept", "head\njson\nCopy\n{code", "head\njson\nCopy\n{code"},
		{"case insensitive", "JSON\nCOPY\nDOWNLOAD\n{code", "{code"},
		{"chinese labels", "json\n复制\n下载\n{code", "{code"},
		{"go toolbar stripped", "go\n复制\n下载\nfmt.Println(42)", "fmt.Println(42)"},
		{"python toolbar stripped", "python\nCopy\nDownload\nprint(1)", "print(1)"},
		{"empty unchanged", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripUIChromePrefix(tt.s); got != tt.want {
				t.Errorf("stripUIChromePrefix(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}

func TestWebChatPollBudget(t *testing.T) {
	t.Run("no deadline defaults to webChatMaxPolls", func(t *testing.T) {
		if got := webChatPollBudget(context.Background()); got != webChatMaxPolls {
			t.Errorf("webChatPollBudget() = %d, want %d", got, webChatMaxPolls)
		}
	})

	t.Run("deadline extends budget to the full timeout", func(t *testing.T) {
		// 1200s timeout (ask_expert timeout=1200) → 600 polls × 2s.
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
		defer cancel()
		if got, want := webChatPollBudget(ctx), 600; got != want {
			t.Errorf("webChatPollBudget() = %d, want %d", got, want)
		}
	})

	t.Run("default 10m deadline maps to legacy 300 polls", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if got, want := webChatPollBudget(ctx), 300; got != want {
			t.Errorf("webChatPollBudget() = %d, want %d", got, want)
		}
	})

	t.Run("sub-interval deadline clamps to one poll", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if got := webChatPollBudget(ctx); got != 1 {
			t.Errorf("webChatPollBudget() = %d, want 1", got)
		}
	})

	t.Run("expired deadline clamps to one poll", func(t *testing.T) {
		ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		if got := webChatPollBudget(ctx); got != 1 {
			t.Errorf("webChatPollBudget() = %d, want 1", got)
		}
	})
}

func TestJsResendFailedFmt(t *testing.T) {
	// Regression guard on the auto-resend matcher: it must keep BOTH the
	// failure words (automatic recovery breaks if dropped) and the
	// exclusion list (losing it would click "重新回答/重新生成" buttons
	// that sit on COMPLETED answers, corrupting a good round into a
	// duplicate or a re-generation).
	for _, want := range []string{
		"重发", "重试", "再次发送", "重发消息", "resend", "retry",
		"重新(回答|生成|思考|加载)", "regenerate",
	} {
		if !strings.Contains(jsResendFailedFmt, want) {
			t.Errorf("jsResendFailedFmt must contain %q (word list regression)", want)
		}
	}
	// The current DeepSeek UI renders the failed-send retry as an
	// icon-only filled yellow circle (ds-button--warning + SVG, no text,
	// no aria-label/title — the tooltip is a CSS overlay). The matcher
	// must keep the icon path: dropping it breaks auto-resend silently.
	for _, want := range []string{
		"ds-button--warning", "querySelector('svg')", "b.disabled",
		"重新发送", "发送失败",
	} {
		if !strings.Contains(jsResendFailedFmt, want) {
			t.Errorf("jsResendFailedFmt must contain %q (icon-path regression)", want)
		}
	}
}

func TestJsChatReadyState(t *testing.T) {
	// Regression guard on the page-classification snippet: it must detect
	// the sign-in page (fast ErrLoginRequired instead of a 30s poll) and
	// the visible textarea (the "ok" state waitForChatTextarea needs).
	for _, want := range []string{
		"textarea", "offsetParent", "ds-sign-in-form-wrapper", "'ok'", "'login'",
	} {
		if !strings.Contains(jsChatReadyState, want) {
			t.Errorf("jsChatReadyState must contain %q (regression)", want)
		}
	}
}

func TestJsIDBGetAnswerFmtNewestFirst(t *testing.T) {
	// Regression guard on the IDB extraction direction. chat_messages is
	// newest-first (the current round's USER/ASSISTANT messages sit at the
	// front), so the last-message scan MUST start at index 0: scanning
	// from the tail would match the FIRST round of a continued
	// conversation and fail the user-message guard, silently degrading to
	// DOM extraction (verified live 2026-08-26, then fixed).
	if !strings.Contains(jsIDBGetAnswerFmt, "for (let i = 0; i < msgs.length; i++)") {
		t.Error("jsIDBGetAnswerFmt must scan chat_messages from index 0 (newest-first)")
	}
	if strings.Contains(jsIDBGetAnswerFmt, "msgs.length - 1; i >= 0") {
		t.Error("jsIDBGetAnswerFmt must not scan chat_messages from the tail")
	}
	// thinkCount must count THINK fragments, not all fragments.
	if !strings.Contains(jsIDBGetAnswerFmt, "thinkCount: thinkParts.length") {
		t.Error("jsIDBGetAnswerFmt thinkCount must count thinkParts")
	}
}

func TestJsSendEnterAsyncFallback(t *testing.T) {
	// The send-button fallback MUST be deferred: React 18 batches state
	// updates, so a synchronous value check after dispatchEvent still sees
	// the pre-send text and clicks the button on top of the Enter —
	// observed as two identical user messages in a real session.
	if !strings.Contains(jsSendEnter, "setTimeout") {
		t.Error("jsSendEnter must schedule the send-button fallback via setTimeout")
	}
}
