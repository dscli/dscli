package lp

import (
	"context"
	"encoding/json"
	"errors"
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
		// as a tool-call fragment (regression guard for the gate).
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

func TestValidateWebChatOptions(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "a.png")
	if err := os.WriteFile(img, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	txt := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(txt, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid := []WebChatOptions{
		{},
		{Attachments: []string{img}},
		{Attachments: []string{txt}}, // text attachments are supported
		{Keep: "last"},
	}
	for _, opts := range valid {
		if err := validateWebChatOptions(opts); err != nil {
			t.Errorf("validateWebChatOptions(%+v) = %v, want nil", opts, err)
		}
	}
	// Role/System/SkipPromptInjection/ShellTool are HandleWebChat-only: the
	// transport must reject them explicitly instead of silently ignoring
	// them.
	for _, opts := range []WebChatOptions{
		{Role: "review"},
		{System: "persona"},
		{SkipPromptInjection: true},
		{ShellTool: true},
	} {
		if err := validateWebChatOptions(opts); err == nil {
			t.Errorf("validateWebChatOptions(%+v) must fail (handle-only field)", opts)
		}
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
	files := make([]string, WebUploadMaxFiles+1)
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
	if err := os.Truncate(big, WebUploadMaxTotal+1); err != nil {
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

	// Text attachments pass the same limits (the page accepts them too).
	notes := filepath.Join(dir, "notes.md")
	if err := os.WriteFile(notes, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWebAttachments([]string{notes}); err != nil {
		t.Errorf("text attachment must pass: %v", err)
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

func TestResolveWithFallback(t *testing.T) {
	reg := testRegistry(map[string]string{"aaa": "https://chat.deepseek.com/a/chat/s/aaa"})

	// A conversation ID the local registry has never seen still resolves to
	// the site's canonical URL: conversations created in the user's own
	// browser never enter the registry, and the site is the authority - the
	// registry must not gate access.
	got, constructed, err := resolveWithFallback(reg, "3f0c88a1-1717-4091-8702-40a9bf10ad8e")
	if err != nil || !constructed {
		t.Fatalf("unregistered id: got %q constructed=%v err=%v", got, constructed, err)
	}
	if want := "https://chat.deepseek.com/a/chat/s/3f0c88a1-1717-4091-8702-40a9bf10ad8e"; got != want {
		t.Errorf("unregistered id URL = %q, want %q", got, want)
	}

	// Registered values keep the stored URL: nothing is reconstructed.
	if got, constructed, err = resolveWithFallback(reg, "aaa"); err != nil || constructed || got != "https://chat.deepseek.com/a/chat/s/aaa" {
		t.Errorf("registered id = %q constructed=%v err=%v; want the stored URL", got, constructed, err)
	}

	// "list" is the CLI listing action, never a conversation URL.
	if _, _, err = resolveWithFallback(reg, "list"); err == nil {
		t.Error("list must not resolve to a URL")
	}

	// Values outside the ID character class keep failing.
	if _, _, err = resolveWithFallback(reg, "no/pe"); err == nil {
		t.Error("a non-ID-shaped value must keep failing")
	}

	// An ambiguous suffix still fails loudly; it must not be constructed
	// into a bogus URL.
	amb := testRegistry(map[string]string{
		"one": "https://chat.deepseek.com/a/chat/s/dup",
		"two": "https://chat.deepseek.com/a/chat/s/other/dup",
	})
	if _, constructed, err = resolveWithFallback(amb, "dup"); err == nil || constructed {
		t.Errorf("ambiguous suffix: constructed=%v err=%v; want a loud failure", constructed, err)
	}

	// A constructible miss keeps the typed error so callers can detect it.
	var nf conversationNotFoundError
	if _, _, err = resolveWithFallback(reg, "no/pe"); !errors.As(err, &nf) {
		t.Errorf("missing-id error = %v, want conversationNotFoundError", err)
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

func TestRegistryIgnoresLegacyModeField(t *testing.T) {
	// webchat_sessions.json files written before the model-selection removal
	// carry a "mode" field; loading must ignore it, and a re-save must not
	// resurrect it (encoding/json drops unknown fields by default - this test
	// pins that back-compat contract).
	const legacy = `{"sessions":{"abc":{"url":"https://chat.deepseek.com/a/chat/s/abc","mode":"pro","updated_at":"2026-09-01T00:00:00Z"}}}`
	var reg conversationRegistry
	if err := json.Unmarshal([]byte(legacy), &reg); err != nil {
		t.Fatalf("legacy registry must load: %v", err)
	}
	entry, ok := reg.Sessions["abc"]
	if !ok || entry.URL != "https://chat.deepseek.com/a/chat/s/abc" || entry.UpdatedAt != "2026-09-01T00:00:00Z" {
		t.Fatalf("legacy entry = %+v (ok=%v), want URL and timestamp preserved", entry, ok)
	}
	out, err := json.Marshal(reg)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"mode"`) {
		t.Errorf("re-saved registry must not contain the removed mode field: %s", out)
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
		// Unclosed markdown code fence: the answer ends inside a block.
		{name: "cut-off json fence", s: "```json\n{\"a\": 1", want: true},
		{name: "cut-off plain fence", s: "```python\nprint('hello'", want: true},
		{name: "closed code block", s: "```json\n{\"a\": 1}\n```", want: false},
		// A complete answer may contain an odd number of ``` when a code
		// block quotes a fence. Counting occurrences flagged these as
		// truncated (the 2026-09-08 code_dev report: a complete 20-rune
		// answer judged truncated 4/4 times).
		{name: "code block quoting a fence", s: "```text\nPM0a ```\n```", want: false},
		{name: "code block quoting a fence with prefix prose", s: "结果如下：\n\n```\nPM0a ```\n```\n", want: false},
		{name: "fence-only answer", s: "```\nfoo\n```", want: false},
		{name: "inline fence only", s: "PM0a ```", want: false},
		// An inline fence never opens a block, so a trailing one is prose,
		// not a cut-off block (the old odd-count heuristic flagged it).
		{name: "inline fence in trailing prose", s: "text\n```\ncode\n```\nthen ``` more", want: false},
		// An unclosed fence AFTER a closed block still truncates.
		{name: "unclosed fence after closed block", s: "text\n```\ncode\n```\nthen\n```", want: true},
		// An unclosed fence inside a DSML parameter value: the reply was cut
		// off mid-call and must never reach the parser/executor (2026-09-08
		// report: the call's own close tags were swallowed into the value
		// and written into the target file).
		{name: "unclosed fence inside a DSML call", s: "<tool_calls>\n<invoke name=\"write_file\">\n<parameter name=\"content\">\n```bash\ncmd\n</parameter>\n</invoke>\n</tool_calls>", want: true},
		{name: "unclosed tilde fence", s: "~~~\ncode", want: true},
		{name: "closed tilde fence", s: "~~~\ncode\n~~~", want: false},
		{name: "tilde does not close a backtick fence", s: "```\ncode\n~~~", want: true},
		{name: "shorter close does not close", s: "````\ncode\n```", want: true},
		{name: "longer close closes", s: "```\ncode\n````", want: false},
		{name: "indented fence is code block content", s: "prose\n    ```\ncode", want: false},
		{name: "tab-indented fence is code block content", s: "prose\n\t```\ncode", want: false},
		// CommonMark: a backtick fence's info string cannot contain backticks.
		{name: "backtick in info string is not a fence", s: "```foo`bar\ncode", want: false},
		// ...but a tilde fence's info string may contain tildes (spec
		// Example 146), so this line DOES open a block.
		{name: "tilde in tilde fence info opens a block", s: "prose\n~~~foo~bar\ncode", want: true},
		// A lone fence inside prose explains the syntax, not truncation.
		{name: "lone fence in prose", s: "Use ``` to open a code block.", want: false},
		// JSON that never terminates.
		{name: "truncated object", s: "{\"question\": \"1\", \"answer\": \"A\"", want: true},
		{name: "truncated array of objects", s: "[{\"id\": 1}, {\"id\": 2}", want: true},
		{name: "complete object", s: "{\"question\": \"1\", \"answer\": \"A\"}", want: false},
		{name: "complete array", s: "[{\"id\": 1}, {\"id\": 2}]", want: false},
		// JSON followed by prose is not a JSON document: the old json.Valid
		// check flagged any complete answer that merely starts with one.
		{name: "complete json followed by prose", s: "{\"a\": 1} is the config object.", want: false},
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
		{name: "complete answer quoting a fence accepted", s: "```text\nPM0a ```\n```", want: true},
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
	// NOTE: these are substring guards only — they catch a dropped
	// keyword but not a logic regression (e.g. a disabled check flipped,
	// or the icon branch moved into a dead path). Matching correctness
	// is covered by the manual end-to-end check against the real
	// logged-in page (injected retry button + decoys).
	for _, want := range []string{
		"ds-button--warning", "querySelector('svg')", "b.disabled",
		"重新发送", "发送失败",
	} {
		if !strings.Contains(jsResendFailedFmt, want) {
			t.Errorf("jsResendFailedFmt must contain %q (icon-path regression)", want)
		}
	}
}

func TestJsContinueGeneration(t *testing.T) {
	// Regression guard on the continue-generation detector. The site
	// renders a 「继续生成」 button when a busy server stops a generation
	// mid-flight; without this matcher webchatWait extracts the truncated
	// fragment as the round's final answer.
	for _, want := range []string{
		"继续生成", "continue",
		"b.disabled", "aria-disabled", "offsetParent",
		"getBoundingClientRect", "scrollIntoView", "elementFromPoint",
		"ds-message",
	} {
		if !strings.Contains(jsContinueGeneration, want) {
			t.Errorf("jsContinueGeneration must contain %q (matcher regression)", want)
		}
	}
	// isTrusted trap: the continue button is the only element in the whole
	// bundle whose onClick validates isTrusted, so ANY synthetic click
	// (el.click(), dispatchEvent(new MouseEvent(...))) is silently ignored
	// and the round would keep returning the interrupted fragment. The
	// snippet must stay detection-only; the real click is dispatched by
	// clickTrustedAt through CDP. These literals therefore must NOT appear
	// in the JS body (they are named here in Go comments only, so the
	// guards themselves cannot become the string they forbid).
	forbidden := []string{".click()", "dispatchEvent("}
	for _, bad := range forbidden {
		if strings.Contains(jsContinueGeneration, bad) {
			t.Errorf("jsContinueGeneration must not contain %q: a synthetic click is silently ignored by the isTrusted guard (use clickTrustedAt)", bad)
		}
	}
	// The detector must return usable coordinates on the found path, not just
	// a boolean: clickTrustedAt dispatches at the returned x/y, so a dropped
	// coordinate field would silently click (0,0).
	for _, want := range []string{"x: x, y: y", "clickable"} {
		if !strings.Contains(jsContinueGeneration, want) {
			t.Errorf("jsContinueGeneration must contain %q (coordinate/clickability regression)", want)
		}
	}
}

// TestJsRegenerateStopped is the regression guard on the stopped-state
// detector. The site renders 「已停止」 with no answer body and no
// 「继续生成」 button when a busy server kills a round during thinking; without
// this matcher webchatWait polls out (or the caller re-sends the whole
// request, redoing the model's work).
func TestJsRegenerateStopped(t *testing.T) {
	for _, want := range []string{
		"已停止", "stopped",
		"ds-message", "ds-assistant-message-main-content",
		// The every-block scan: querySelectorAll over ALL main-content
		// blocks (querySelector alone would stop at the first one).
		"querySelectorAll", "mains.length",
		"aria-disabled", "offsetParent",
		"getBoundingClientRect", "scrollIntoView", "elementFromPoint",
	} {
		if !strings.Contains(jsRegenerateStopped, want) {
			t.Errorf("jsRegenerateStopped must contain %q (matcher regression)", want)
		}
	}
	// The detector must return usable coordinates and the button count on the
	// found path: clickTrustedAt dispatches at the returned x/y, so a dropped
	// coordinate field would silently click (0,0).
	for _, want := range []string{"x: x, y: y", "buttons: bar.length", "clickable"} {
		if !strings.Contains(jsRegenerateStopped, want) {
			t.Errorf("jsRegenerateStopped must contain %q (coordinate/field regression)", want)
		}
	}
	// Detection-only discipline: this snippet must never gain a click. The
	// regenerate button has no isTrusted guard today, but the click still
	// goes through clickTrustedAt so the event travels the browser's input
	// pipeline - and a future guard must not be able to catch it.
	for _, bad := range []string{".click(", "dispatchEvent("} {
		if strings.Contains(jsRegenerateStopped, bad) {
			t.Errorf("jsRegenerateStopped must not contain %q: the real click is dispatched by clickTrustedAt", bad)
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

func TestJsFetchTailFmt(t *testing.T) {
	// The resume tail must come from the SERVER's own sync endpoint, never
	// from the local cache: the fetch carries the page's user token, asks
	// for the messages after the local cursor, and picks the newest
	// message by the largest message_id.
	for _, want := range []string{
		"localStorage.getItem('userToken')",
		"/api/v0/chat/history_messages",
		"&cache_version=",
		"authorization: auth",
		"best.message_id",
		"msgs[msgs.length - 1]",
		"delta(baseV - 1, true)",
		"delta(0, false)",
	} {
		if !strings.Contains(jsFetchTailFmt, want) {
			t.Errorf("jsFetchTailFmt must contain %q (regression)", want)
		}
	}
	// Exactly three formatting verbs: the id and the two cursor numbers. A
	// stray %-sign would corrupt the Sprintf and silently break the read.
	if got := strings.Count(jsFetchTailFmt, "%"); got != 3 {
		t.Errorf("format verbs = %d, want 3 (id, cache_version, cache_reset_at)", got)
	}
}

func TestParseTailResult(t *testing.T) {
	// The server-tail result parser must never turn a malformed site
	// response into a success with empty fields, and must surface the
	// refusal reason verbatim.
	tests := []struct {
		name     string
		result   map[string]any
		wantRole string
		wantOK   bool
		wantWhy  string
	}{
		{
			name:     "complete tail",
			result:   map[string]any{"found": true, "tail": map[string]any{"content": "hi", "status": "FINISHED", "role": "ASSISTANT"}},
			wantRole: "ASSISTANT",
			wantOK:   true,
		},
		{name: "missing tail object", result: map[string]any{"found": true}, wantOK: false, wantWhy: "no tail object"},
		{name: "tail without role", result: map[string]any{"found": true, "tail": map[string]any{"content": "hi"}}, wantOK: false, wantWhy: "without a role"},
		{name: "not found carries the reason", result: map[string]any{"found": false, "reason": "chat code 40003: INVALID_TOKEN"}, wantOK: false, wantWhy: "INVALID_TOKEN"},
		{name: "not found without a reason", result: map[string]any{"found": false}, wantOK: false, wantWhy: "returned nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, role, why, ok := parseTailResult(tt.result)
			if ok != tt.wantOK || role != tt.wantRole {
				t.Errorf("parseTailResult(%v) = ok=%v role=%q, want ok=%v role=%q", tt.result, ok, role, tt.wantOK, tt.wantRole)
			}
			if tt.wantWhy != "" && !strings.Contains(why, tt.wantWhy) {
				t.Errorf("reason = %q, want it to contain %q", why, tt.wantWhy)
			}
		})
	}
}

func TestJsIDBTailCursorFmt(t *testing.T) {
	// The cursor snippet reads ONLY the sync cursor (version and reset
	// time), never message content: the tail is server-side.
	for _, want := range []string{"rec.version", "rec.cacheResetAt"} {
		if !strings.Contains(jsIDBTailCursorFmt, want) {
			t.Errorf("jsIDBTailCursorFmt must contain %q (regression)", want)
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
	// tokens must be the accumulated_token_usage difference between the
	// round's ASSISTANT and USER messages (the site's own running counter),
	// and never a negative count.
	if !strings.Contains(jsIDBGetAnswerFmt, "accumulated_token_usage") {
		t.Error("jsIDBGetAnswerFmt must read accumulated_token_usage for token counts")
	}
	if !strings.Contains(jsIDBGetAnswerFmt, "tokens: tokens") {
		t.Error("jsIDBGetAnswerFmt must return the derived token count")
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

// TestJsAssistantTextAfterFmt is the regression guard on the scoped DOM
// extraction. After a recovery (auto-continue / regenerate) the previous,
// unanchored extraction started at the PREVIOUS round - whose already
// executed block made the shell judge report "the tags appear more than
// once". The snippet must therefore scope on the .ds-message bubble holding
// the sent message, return innerHTML (markdown fidelity, not innerText),
// and stay click-free.
func TestJsAssistantTextAfterFmt(t *testing.T) {
	for _, want := range []string{
		"ds-message",
		"ds-assistant-message-main-content",
		"closest",
		"innerHTML",
		"ord",
		// The empty-want guard: an empty anchor must return no parts so the
		// caller falls through to the legacy paths instead of matching the
		// first bubble on the page.
		"if (!want || !want.trim()) return []",
		// The anchor walk, back to front: the LAST matching bubble is the
		// chat copy of this round.
		"bubbles.length - 1; i >= 0; i--",
		// The echo guard: an answered assistant bubble is never the sent
		// message, so a reply that restates it cannot steal the anchor.
		"querySelector('.ds-assistant-message-main-content')) continue",
		"indexOf(want) === 0",
		// Only blocks strictly AFTER the anchor bubble.
		"(ord.get(b) || 0) <= anchor",
		// The .ds-markdown fallback must drop the deep-think block.
		"已思考",
	} {
		if !strings.Contains(jsAssistantTextAfterFmt, want) {
			t.Errorf("jsAssistantTextAfterFmt must contain %q (regression)", want)
		}
	}
	// Exactly one format verb: the quoted sent message.
	if got := strings.Count(jsAssistantTextAfterFmt, "%"); got != 1 {
		t.Errorf("format verbs = %d, want 1 (sentMessage)", got)
	}
	// Detection-only discipline: the real clicks go through clickTrustedAt,
	// and a synthetic click would be silently dropped by the site's
	// isTrusted guard anyway.
	for _, bad := range []string{".click(", "dispatchEvent("} {
		if strings.Contains(jsAssistantTextAfterFmt, bad) {
			t.Errorf("jsAssistantTextAfterFmt must not contain %q (detection only)", bad)
		}
	}
}

// TestJsIDBGetAnswerFmtRegeneratedRound pins the candidate selection for a
// regenerated round: the record keeps every attempt as a separate ASSISTANT
// message right after the round's USER, and the stopped first attempt
// carries no RESPONSE fragments (live record, 2026-09-15). The reader must
// walk that group and take the LAST attempt with a non-empty RESPONSE -
// the old "first assistant from index 0" rule returned the empty stopped
// shell, so every post-recovery read failed and degraded to the DOM
// fallback.
func TestJsIDBGetAnswerFmtRegeneratedRound(t *testing.T) {
	for _, want := range []string{
		"lastUserIdx",
		"for (let i = lastUserIdx + 1; i < msgs.length; i++)",
		"if (msgs[i].role === 'USER') break",
		"f.content.trim() !== ''",
		"const chosen = lastWithText || last",
		"chosen.accumulated_token_usage",
		"status: chosen.status",
	} {
		if !strings.Contains(jsIDBGetAnswerFmt, want) {
			t.Errorf("jsIDBGetAnswerFmt must contain %q (regenerated-round candidate walk)", want)
		}
	}
}

// TestClassifyReply pins the shared classification the wait's main path and
// fallback chain use: busy text is always a server-busy error (independent
// of canExtract), the truncation/completeness judgement is gated by
// canExtract, and a fragment that is not classifiable keeps the caller
// polling.
func TestClassifyReply(t *testing.T) {
	long := strings.Repeat("字", minCompleteResponseLen+10)
	toolAt := func(n int) string {
		return "<read_file " + strings.Repeat("x", n-len("<read_file "))
	}
	cases := []struct {
		name       string
		resp       string
		canExtract bool
		wantDone   bool
		wantText   string
		wantErr    error
	}{
		{"busy text is always an error", "服务器繁忙，请稍后再试", false, true, "", ErrServerBusy},
		{"truncated gated off keeps polling", "```go\nfunc broken() {", false, false, "", nil},
		{"truncated is retryable when gated on", "```go\nfunc broken() {", true, true, "", ErrTruncated},
		{"tool call opening is not an answer", `<read_file path="internal/lp/webchat.go">`, true, false, "", nil},
		{"tool call opening at the boundary still polls", toolAt(minCompleteResponseLen), true, false, "", nil},
		{"tool call opening past the boundary counts as a body", toolAt(minCompleteResponseLen + 1), true, true, toolAt(minCompleteResponseLen + 1), nil},
		{"long complete text is done", long, true, true, long, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			done, out, err := classifyReply(c.resp, c.canExtract)
			if done != c.wantDone {
				t.Fatalf("done = %v, want %v (err %v)", done, c.wantDone, err)
			}
			if c.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			} else if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if out != c.wantText {
				t.Errorf("out = %q, want %q", out, c.wantText)
			}
		})
	}
}
