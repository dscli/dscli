package lp

// Tests for the webChatMaxInputRunes truncation layer: user messages are cut
// before sending, and tool feedback is cut block-wise so <tool_result> blocks
// stay well-formed. The site rejects inputs past its 字数 limit with a toast
// (超出字数限制) that the wait loop would otherwise misread as a send failure.

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/toolcall"
)

// repeatRunes repeats s n times (n is a rune count when s is one rune).
func repeatRunes(s string, n int) string { return strings.Repeat(s, n) }

func TestTruncateWebChatMessageShortStays(t *testing.T) {
	in := "hello, world"
	if got := truncateWebChatMessage(in); got != in {
		t.Errorf("short message changed: %q", got)
	}
}

func TestTruncateWebChatMessageLongASCII(t *testing.T) {
	in := repeatRunes("a", webChatMaxInputRunes+100)
	got := truncateWebChatMessage(in)
	if utf8.RuneCountInString(got) > webChatMaxInputRunes {
		t.Fatalf("truncated message %d runes, want <= %d", utf8.RuneCountInString(got), webChatMaxInputRunes)
	}
	if !strings.Contains(got, "[已截断]") {
		t.Errorf("truncated message lacks marker: %q", got[utf8.RuneCountInString(got)-60:])
	}
	if !strings.HasPrefix(got, "aaaa") {
		t.Errorf("truncated message does not keep the head")
	}
}

func TestTruncateWebChatMessageLongChinese(t *testing.T) {
	in := repeatRunes("深", webChatMaxInputRunes*2)
	got := truncateWebChatMessage(in)
	if n := utf8.RuneCountInString(got); n > webChatMaxInputRunes {
		t.Fatalf("truncated message %d runes, want <= %d", n, webChatMaxInputRunes)
	}
	if !strings.Contains(got, "[已截断]") {
		t.Errorf("truncated message lacks marker")
	}
	if !strings.HasPrefix(got, "深深深") {
		t.Errorf("truncated message does not keep the head")
	}
}

func TestTruncateToolResultBlock(t *testing.T) {
	// Well-formed block that fits: unchanged, ok=true.
	body := strings.Repeat("x", 1000)
	if got, ok := truncateToolResultBlock("<tool_result>"+body+"</tool_result>", 5000); !ok || !strings.Contains(got, body) {
		t.Errorf("fitting block: ok=%v got=%q", ok, got)
	}
	// Over-long block: tags preserved, marker added, fits the budget.
	long := "<tool_result>" + strings.Repeat("x", 20000) + "</tool_result>"
	got, ok := truncateToolResultBlock(long, 10000)
	if !ok {
		t.Fatalf("over-long block not truncated (ok=false)")
	}
	if !strings.HasPrefix(got, "<tool_result>") || !strings.HasSuffix(got, "</tool_result>") {
		t.Errorf("block tags broken: %q", got[:40]+"...")
	}
	if utf8.RuneCountInString(got) > 10000 {
		t.Errorf("truncated block %d runes, want <= 10000", utf8.RuneCountInString(got))
	}
	if !strings.Contains(got, "已截断") {
		t.Errorf("truncated block lacks marker")
	}
	// Not a well-formed block: ok=false.
	if _, ok := truncateToolResultBlock("no tags here", 10000); ok {
		t.Errorf("non-block input reported ok=true")
	}
	// Budget too small even for tags: ok=false.
	if _, ok := truncateToolResultBlock(long, 30); ok {
		t.Errorf("unfit budget reported ok=true")
	}
}

func TestBuildWebChatFeedbackShortJoins(t *testing.T) {
	outputs := []string{
		"<tool_result>{\"result\":\"first\"}</tool_result>",
		"<tool_result>{\"result\":\"second\"}</tool_result>",
	}
	got := buildWebChatFeedback(outputs)
	if got != strings.Join(outputs, "\n") {
		t.Errorf("short feedback altered: %q", got)
	}
}

func TestBuildWebChatFeedbackTruncatesBlocks(t *testing.T) {
	// Three blocks of 30k runes each: first block kept whole, second kept
	// whole, third cut (tags preserved), omission note appended.
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }
	a := blk(strings.Repeat("a", 30000))
	b := blk(strings.Repeat("b", 30000))
	c := blk(strings.Repeat("c", 30000))
	got := buildWebChatFeedback([]string{a, b, c})
	if utf8.RuneCountInString(got) > webChatMaxInputRunes {
		t.Fatalf("feedback %d runes, want <= %d", utf8.RuneCountInString(got), webChatMaxInputRunes)
	}
	if !strings.Contains(got, "aaa") || !strings.Contains(got, "bbb") {
		t.Errorf("feedback lost complete blocks")
	}
	if !strings.Contains(got, "⚠️[工具结果过长，已截断]") {
		t.Errorf("feedback lacks per-block truncation marker")
	}
	if !strings.Contains(got, "已省略 0 个工具结果") && !strings.Contains(got, "输入超过长度限制") {
		t.Errorf("feedback lacks omission note")
	}
	if !strings.HasSuffix(got, "</tool_result>") {
		t.Errorf("feedback does not end with a closed block")
	}
	// Omitted-block case: A fits (no), B/C dropped. Make A exactly 30k and
	// B + C big so the loop keeps A then cuts B... verify counts instead:
	// simpler: two blocks, first small, second huge, plus a third.
	small := blk(strings.Repeat("s", 100))
	huge := blk(strings.Repeat("h", webChatMaxInputRunes))
	more := blk(strings.Repeat("m", 120000))
	got2 := buildWebChatFeedback([]string{small, huge, more})
	if utf8.RuneCountInString(got2) > webChatMaxInputRunes {
		t.Fatalf("feedback2 %d runes, want <= %d", utf8.RuneCountInString(got2), webChatMaxInputRunes)
	}
	if !strings.HasPrefix(got2, small) {
		t.Errorf("first small block should be kept whole")
	}
	if !strings.Contains(got2, "已省略 1 个工具结果") {
		t.Errorf("want '已省略 1 个工具结果' in note: %q", tail(got2, 120))
	}
}

func TestHandleWebChatTruncatesLongInput(t *testing.T) {
	orig := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = orig })
	var sent string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		sent = msg
		return WebChatResult{Content: "final answer", URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	in := strings.Repeat("字", webChatMaxInputRunes+2000)
	if _, err := HandleWebChat(context.Background(), in, WebChatOptions{}); err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if utf8.RuneCountInString(sent) > webChatMaxInputRunes {
		t.Fatalf("sent %d runes, want <= %d", utf8.RuneCountInString(sent), webChatMaxInputRunes)
	}
	if !strings.Contains(sent, "[已截断]") {
		t.Errorf("sent message lacks truncation marker")
	}
}

func TestHandleWebChatToolLoopTruncatesFeedback(t *testing.T) {
	origSend := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origSend })

	const (
		url1 = "https://chat.deepseek.com/a/chat/s/convAAA"
		url2 = "https://chat.deepseek.com/a/chat/s/convBBB"
	)
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		switch len(messages) {
		case 1:
			return WebChatResult{Content: dsmlReply, URL: url1}, nil
		case 2:
			return WebChatResult{Content: "final answer", URL: url2}, nil
		}
		return WebChatResult{Content: "unexpected extra round", URL: url2}, nil
	}

	// Executor returns two tool results far beyond the cap (e.g. two
	// read_file dumps). The first block is cut in place, the second is
	// omitted in the note.
	big := strings.Repeat("Z", webChatMaxInputRunes)
	origExec := handleWebChatExecDSML
	var seenCalls []toolcall.DSMLCall
	handleWebChatExecDSML = func(_ context.Context, calls []toolcall.DSMLCall) []string {
		seenCalls = append(seenCalls, calls...)
		return []string{
			"<tool_result>{\"result\":\"" + big + "\"}</tool_result>",
			"<tool_result>{\"result\":\"second\"}</tool_result>",
		}
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	if _, err := HandleWebChat(context.Background(), "check this file", WebChatOptions{Role: "review"}); err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if len(seenCalls) != 1 {
		t.Fatalf("executed calls = %d, want 1", len(seenCalls))
	}
	if len(messages) != 2 {
		t.Fatalf("messages = %d, want 2", len(messages))
	}
	feedback := messages[1]
	if utf8.RuneCountInString(feedback) > webChatMaxInputRunes {
		t.Fatalf("feedback %d runes, want <= %d", utf8.RuneCountInString(feedback), webChatMaxInputRunes)
	}
	if !strings.Contains(feedback, "已截断") {
		t.Errorf("feedback lacks truncation marker")
	}
	if !strings.HasSuffix(feedback, "</tool_result>") {
		t.Errorf("feedback does not end with a closed block")
	}
	if !strings.Contains(feedback, "已省略 1 个工具结果") {
		t.Errorf("want '已省略 1 个工具结果' in note: %q", tail(feedback, 120))
	}
}

func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
