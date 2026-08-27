package lp

// Tests for the webChatMaxInputBytes truncation layer: user messages are cut
// before sending, and tool feedback is cut block-wise so <tool_result> blocks
// stay well-formed. The site rejects inputs past its byte limit with a toast
// (超出字数限制) that the wait loop would otherwise misread as a send failure.
// Live measurement: 185537 bytes pass, 185538 bytes are rejected (wc -c).

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/toolcall"
)

func TestHeadBytesRuneAligned(t *testing.T) {
	// ASCII prefix: safe cut.
	if got := headBytes("abcdef", 3); got != "abc" {
		t.Errorf("ascii head = %q, want abc", got)
	}
	// Chinese (3 bytes per rune): a byte budget in the middle of a rune must
	// back off to the rune boundary, never split the UTF-8 sequence.
	cn := strings.Repeat("深", 100) // 300 bytes
	if got := headBytes(cn, 299); len(got) != 297 {
		t.Errorf("rune-aligned head = %d bytes, want 297 (99 runes)", len(got))
	} else if !utf8.ValidString(got) {
		t.Errorf("head is not valid UTF-8: %q", got)
	}
	// Chinese budget exactly on a rune boundary.
	if got := headBytes(cn, 300); got != cn {
		t.Errorf("exact boundary head differs from input")
	}
	// Zero/negative budget.
	if got := headBytes(cn, 0); got != "" {
		t.Errorf("zero budget head = %q, want empty", got)
	}
	// Budget larger than input.
	if got := headBytes(cn, 9999); got != cn {
		t.Errorf("oversized budget head differs from input")
	}
}

func TestTruncateWebChatMessageShortStays(t *testing.T) {
	in := "hello, world"
	if got := truncateWebChatMessage(in); got != in {
		t.Errorf("short message changed: %q", got)
	}
	// Live-measured safe size passes through untouched (slightly under cap).
	in2 := strings.Repeat("a", 185217)
	if got := truncateWebChatMessage(in2); got != in2 {
		t.Errorf("measured-safe message changed: len=%d", len(got))
	}
}

func TestTruncateWebChatMessageLongASCII(t *testing.T) {
	in := strings.Repeat("a", webChatMaxInputBytes+100)
	got := truncateWebChatMessage(in)
	if len(got) > webChatMaxInputBytes {
		t.Fatalf("truncated message %d bytes, want <= %d", len(got), webChatMaxInputBytes)
	}
	if !strings.Contains(got, "[已截断]") {
		t.Errorf("truncated message lacks marker: %q", got[len(got)-60:])
	}
	if !strings.HasPrefix(got, "aaaa") {
		t.Errorf("truncated message does not keep the head")
	}
}

func TestTruncateWebChatMessageLongChinese(t *testing.T) {
	// 70000 汉字 = 210000 bytes, over the cap; the cut must land on a rune
	// boundary so the result stays valid UTF-8.
	in := strings.Repeat("深", 70000)
	got := truncateWebChatMessage(in)
	if n := len(got); n > webChatMaxInputBytes {
		t.Fatalf("truncated message %d bytes, want <= %d", n, webChatMaxInputBytes)
	}
	if !strings.Contains(got, "[已截断]") {
		t.Errorf("truncated message lacks marker")
	}
	if !strings.HasPrefix(got, "深深深") {
		t.Errorf("truncated message does not keep the head")
	}
	if !utf8.ValidString(got) {
		t.Errorf("truncated message is not valid UTF-8")
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
	if len(got) > 10000 {
		t.Errorf("truncated block %d bytes, want <= 10000", len(got))
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
	// Blocks = 60027 / 130027 / 70027 bytes with the cap at 185536: the first
	// block is kept whole; the second does not fit the remaining budget and
	// is cut in place (tags + body + marker); the third is omitted and
	// summarized in the trailing note.
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }
	a := blk(strings.Repeat("a", 60000))
	b := blk(strings.Repeat("b", 130000))
	c := blk(strings.Repeat("c", 70000))
	got := buildWebChatFeedback([]string{a, b, c})
	if len(got) > webChatMaxInputBytes {
		t.Fatalf("feedback %d bytes, want <= %d", len(got), webChatMaxInputBytes)
	}
	if !strings.HasPrefix(got, a) {
		t.Errorf("first block should be kept whole")
	}
	if !strings.Contains(got, "bbb") {
		t.Errorf("feedback lost the second (truncated) block")
	}
	if !strings.Contains(got, "⚠️[工具结果过长，已截断]") {
		t.Errorf("feedback lacks per-block truncation marker")
	}
	if !strings.Contains(got, "已省略 1 个工具结果") {
		t.Errorf("feedback lacks omission note")
	}
	if !strings.HasSuffix(got, "</tool_result>") {
		t.Errorf("feedback does not end with a closed block")
	}
	// Omitted-block case: A fits, B/C dropped.
	small := blk(strings.Repeat("s", 100))
	huge := blk(strings.Repeat("h", webChatMaxInputBytes))
	more := blk(strings.Repeat("m", 120000))
	got2 := buildWebChatFeedback([]string{small, huge, more})
	if len(got2) > webChatMaxInputBytes {
		t.Fatalf("feedback2 %d bytes, want <= %d", len(got2), webChatMaxInputBytes)
	}
	if !strings.HasPrefix(got2, small) {
		t.Errorf("first small block should be kept whole")
	}
	if !strings.Contains(got2, "已省略 1 个工具结果") {
		t.Errorf("want '已省略 1 个工具结果' in note: %q", tail(got2, 120))
	}
}

// TestBuildWebChatFeedbackNewlineBudget pins the newline accounting: the
// "\n" separators that strings.Join inserts are part of the sent text and
// must be charged against the cap. Two blocks totalling exactly the cap
// would otherwise round-trip as cap+1 bytes and trigger the site rejection.
func TestBuildWebChatFeedbackNewlineBudget(t *testing.T) {
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }

	// Blocks of exactly cap/2 bytes each: raw total == cap, plus one newline
	// -> over the cap, must take the truncation path.
	pad := webChatMaxInputBytes/2 - len("<tool_result></tool_result>")
	a := blk(strings.Repeat("a", pad))
	b := blk(strings.Repeat("b", pad))
	if ga, gb := len(a), len(b); ga != webChatMaxInputBytes/2 || gb != webChatMaxInputBytes/2 {
		t.Fatalf("setup: block sizes %d %d, want %d each", ga, gb, webChatMaxInputBytes/2)
	}
	got := buildWebChatFeedback([]string{a, b})
	if len(got) > webChatMaxInputBytes {
		t.Fatalf("boundary feedback %d bytes, want <= %d", len(got), webChatMaxInputBytes)
	}
	if !strings.HasPrefix(got, a) {
		t.Errorf("first block should be kept whole at boundary")
	}

	// Blocks that fit exactly with the newline: raw total == cap-1, one
	// newline -> exactly cap: must round-trip verbatim (short path).
	pad2 := webChatMaxInputBytes/2 - len("<tool_result></tool_result>") - 1
	a2 := blk(strings.Repeat("a", pad2))
	b2 := blk(strings.Repeat("b", pad2))
	if got := buildWebChatFeedback([]string{a2, b2}); got != strings.Join([]string{a2, b2}, "\n") {
		t.Errorf("fitting block pair should pass through unchanged")
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
	in := strings.Repeat("字", 70000) // 210000 bytes
	if _, err := HandleWebChat(context.Background(), in, WebChatOptions{}); err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if len(sent) > webChatMaxInputBytes {
		t.Fatalf("sent %d bytes, want <= %d", len(sent), webChatMaxInputBytes)
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
	big := strings.Repeat("Z", webChatMaxInputBytes)
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
	if len(feedback) > webChatMaxInputBytes {
		t.Fatalf("feedback %d bytes, want <= %d", len(feedback), webChatMaxInputBytes)
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
