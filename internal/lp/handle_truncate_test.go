package lp

// Tests for the webChatMaxInputRunes truncation layer: user messages are cut
// before sending, and tool feedback is cut block-wise so <tool_result> blocks
// stay well-formed. The site rejects inputs past its CHARACTER (rune) limit
// with a toast (超出字数限制) that the wait loop would otherwise misread as a
// send failure. Live measurement (probe, 2026-08-29): ASCII 162000 runes
// pass, ~165339 runes are rejected; mixed Chinese+code 180000 BYTES (~105k
// runes) passes — the limit counts runes, not bytes.

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/dscli/dscli/internal/toolcall"
)

func TestHeadRunes(t *testing.T) {
	// ASCII prefix: safe cut.
	if got := headRunes("abcdef", 3); got != "abc" {
		t.Errorf("ascii head = %q, want abc", got)
	}
	// Chinese: a rune budget cuts on rune boundaries, never splitting the
	// multi-byte UTF-8 sequence.
	cn := strings.Repeat("深", 100) // 100 runes, 300 bytes
	if got := headRunes(cn, 99); got != strings.Repeat("深", 99) {
		t.Errorf("rune head = %d runes, want 99", countRunes(got))
	} else if !utf8.ValidString(got) {
		t.Errorf("head is not valid UTF-8: %q", got)
	}
	// Budget exactly on a rune boundary.
	if got := headRunes(cn, 100); got != cn {
		t.Errorf("exact boundary head differs from input")
	}
	// Zero/negative budget.
	if got := headRunes(cn, 0); got != "" {
		t.Errorf("zero budget head = %q, want empty", got)
	}
	// Budget larger than input.
	if got := headRunes(cn, 9999); got != cn {
		t.Errorf("oversized budget head differs from input")
	}
	// Invalid bytes inside the input: []rune conversion replaces a lone
	// invalid byte with U+FFFD (Go's decoder behavior); the head counts
	// runes (the invalid byte is one rune) and is always valid UTF-8.
	raw := "ab\xffcdef" // 7 bytes, 6 runes (0xff becomes one U+FFFD rune)
	if got := headRunes(raw, 3); got != "ab\uFFFD" {
		t.Errorf("invalid-byte rune head = %q, want abU+FFFD", got)
	}
	if got := headRunes(raw, 0); got != "" {
		t.Errorf("zero budget on raw = %q, want empty", got)
	}
	// Full input passes through.
	if got := headRunes(raw, 99); got != raw {
		t.Errorf("full input should pass through")
	}
}

func TestTruncateWebChatMessageShortStays(t *testing.T) {
	in := "hello, world"
	if got := truncateWebChatMessage(in); got != in {
		t.Errorf("short message changed: %q", got)
	}
	// Measured-safe size passes through untouched (slightly under cap).
	in2 := strings.Repeat("a", 154000)
	if got := truncateWebChatMessage(in2); got != in2 {
		t.Errorf("measured-safe message changed: len=%d", len(got))
	}
}

func TestTruncateWebChatMessageLongASCII(t *testing.T) {
	in := strings.Repeat("a", webChatMaxInputRunes+100)
	got := truncateWebChatMessage(in)
	if n := countRunes(got); n > webChatMaxInputRunes {
		t.Fatalf("truncated message %d runes, want <= %d", n, webChatMaxInputRunes)
	}
	if !strings.Contains(got, "[已截断]") {
		t.Errorf("truncated message lacks marker: %q", tail(got, 60))
	}
	if !strings.HasPrefix(got, "aaaa") {
		t.Errorf("truncated message does not keep the head")
	}
}

func TestTruncateWebChatMessageLongChinese(t *testing.T) {
	// 160000 汉字 = 480000 bytes, over the cap; the cut must land on a rune
	// boundary so the result stays valid UTF-8.
	in := strings.Repeat("深", 160000)
	got := truncateWebChatMessage(in)
	if n := countRunes(got); n > webChatMaxInputRunes {
		t.Fatalf("truncated message %d runes, want <= %d", n, webChatMaxInputRunes)
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
	if n := countRunes(got); n > 10000 {
		t.Errorf("truncated block %d runes, want <= 10000", n)
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
	// Blocks = 60000 / 130000 / 70000 runes with the cap at 155000: the first
	// block is kept whole; the second does not fit the remaining budget and
	// is cut in place (tags + body + marker); the third is omitted and
	// summarized in the trailing note.
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }
	a := blk(strings.Repeat("a", 60000))
	b := blk(strings.Repeat("b", 130000))
	c := blk(strings.Repeat("c", 70000))
	got := buildWebChatFeedback([]string{a, b, c})
	if n := countRunes(got); n > webChatMaxInputRunes {
		t.Fatalf("feedback %d runes, want <= %d", n, webChatMaxInputRunes)
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
	huge := blk(strings.Repeat("h", webChatMaxInputRunes))
	more := blk(strings.Repeat("m", 120000))
	got2 := buildWebChatFeedback([]string{small, huge, more})
	if n := countRunes(got2); n > webChatMaxInputRunes {
		t.Fatalf("feedback2 %d runes, want <= %d", n, webChatMaxInputRunes)
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
// would otherwise round-trip as cap+1 runes and trigger the site rejection.
func TestBuildWebChatFeedbackNewlineBudget(t *testing.T) {
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }

	// Blocks of exactly cap/2 runes each: raw total == cap, plus one newline
	// -> over the cap, must take the truncation path.
	pad := webChatMaxInputRunes/2 - countRunes("<tool_result></tool_result>")
	a := blk(strings.Repeat("a", pad))
	b := blk(strings.Repeat("b", pad))
	if ga, gb := countRunes(a), countRunes(b); ga != webChatMaxInputRunes/2 || gb != webChatMaxInputRunes/2 {
		t.Fatalf("setup: block sizes %d %d, want %d each", ga, gb, webChatMaxInputRunes/2)
	}
	got := buildWebChatFeedback([]string{a, b})
	if n := countRunes(got); n > webChatMaxInputRunes {
		t.Fatalf("boundary feedback %d runes, want <= %d", n, webChatMaxInputRunes)
	}
	if !strings.HasPrefix(got, a) {
		t.Errorf("first block should be kept whole at boundary")
	}

	// Blocks that fit exactly with the newline: raw total == cap-1, one
	// newline -> exactly cap: must round-trip verbatim (short path).
	pad2 := webChatMaxInputRunes/2 - countRunes("<tool_result></tool_result>") - 1
	a2 := blk(strings.Repeat("a", pad2))
	b2 := blk(strings.Repeat("b", pad2))
	if got := buildWebChatFeedback([]string{a2, b2}); got != strings.Join([]string{a2, b2}, "\n") {
		t.Errorf("fitting block pair should pass through unchanged")
	}
}

// TestWebChatOutputNoteFitsReserve pins the coupling the cap invariant hangs
// on: buildWebChatFeedback's total reduces to cap - reserve + len(note), so
// a future note longer than the reserve would silently reintroduce over-cap
// sends. Worst realistic digits: many results and huge rune counts.
func TestWebChatOutputNoteFitsReserve(t *testing.T) {
	tests := []struct {
		name         string
		omittedCount int
		omittedRunes int
	}{
		{"typical", 1, 50000},
		{"many results", 1_000_000, 123},
		{"huge runes", 3, 900_000_000_000_000_000}, // int64-scale, 18 digits
		{"both extreme", 1_000_000, 900_000_000_000_000_000},
		{"none omitted", 0, 999_999_999},
	}
	for _, tt := range tests {
		note := webChatOutputNote(tt.omittedCount, tt.omittedRunes)
		if n := countRunes(note); n > webChatOutputNoteReserve {
			t.Errorf("%s: note %d runes exceeds reserve %d: %q", tt.name, n, webChatOutputNoteReserve, note)
		}
		if !strings.HasPrefix(note, "<tool_result>") || !strings.HasSuffix(note, "</tool_result>") {
			t.Errorf("%s: note tags broken: %q", tt.name, note)
		}
	}
}

func TestBuildWebChatFeedbackDropPathReservesNote(t *testing.T) {
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }
	// First block leaves 40 runes of budget after it; the cut attempt gets
	// maxRunes = 39 < tags+marker (67), so truncateToolResultBlock gives up
	// and the second block is DROPPED - the omission note still appended
	// from the up-front reserve.
	body := webChatMaxInputRunes - webChatOutputNoteReserve - countRunes("<tool_result></tool_result>") - 41
	a := blk(strings.Repeat("a", body))
	b := blk(strings.Repeat("b", 100000))
	got := buildWebChatFeedback([]string{a, b})
	if n := countRunes(got); n > webChatMaxInputRunes {
		t.Fatalf("drop-path feedback %d runes, want <= %d", n, webChatMaxInputRunes)
	}
	if !strings.HasPrefix(got, a) {
		t.Errorf("first block should be kept whole")
	}
	if !strings.Contains(got, "已省略 1 个工具结果") {
		t.Errorf("want '已省略 1 个工具结果' in note: %q", tail(got, 120))
	}
}

// TestBuildWebChatFeedbackDropPathRegression reproduces the pre-fix over-cap
// send with the OLD budget model (note runes unreserved): a first block of
// cap-100 runes was kept whole (n+1 <= cap), leaving ~99 runes - less than
// the note itself - so the second block was dropped and the note pushed the
// total over the cap. The fixed code cuts the first block instead and stays
// under cap. This test fails before the up-front reserve fix.
func TestBuildWebChatFeedbackDropPathRegression(t *testing.T) {
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }
	big := blk(strings.Repeat("x", webChatMaxInputRunes-100-countRunes("<tool_result></tool_result>")))
	if n := countRunes(big); n != webChatMaxInputRunes-100 {
		t.Fatalf("setup: big block %d runes, want cap-100", n)
	}
	got := buildWebChatFeedback([]string{big, blk(strings.Repeat("y", 1000))})
	if n := countRunes(got); n > webChatMaxInputRunes {
		t.Fatalf("regression feedback %d runes, want <= %d", n, webChatMaxInputRunes)
	}
	if !strings.Contains(got, "已截断") {
		t.Errorf("regression feedback lacks truncation marker")
	}
	if !strings.HasSuffix(got, "</tool_result>") {
		t.Errorf("regression feedback does not end with a closed block")
	}
}

// TestBuildWebChatFeedbackSingleBlockOverCap covers the single-block path: a
// block of cap+1 runes must be cut in place (never dropped, never over-cap).
func TestBuildWebChatFeedbackSingleBlockOverCap(t *testing.T) {
	blk := func(s string) string { return "<tool_result>" + s + "</tool_result>" }
	solo := blk(strings.Repeat("s", webChatMaxInputRunes-countRunes("<tool_result></tool_result>")+1))
	got := buildWebChatFeedback([]string{solo})
	if n := countRunes(got); n > webChatMaxInputRunes {
		t.Fatalf("single-block feedback %d runes, want <= %d", n, webChatMaxInputRunes)
	}
	if !strings.HasPrefix(got, "<tool_result>") || !strings.HasSuffix(got, "</tool_result>") {
		t.Errorf("single-block feedback broke tags")
	}
	if !strings.Contains(got, "已截断") {
		t.Errorf("single-block feedback lacks truncation marker")
	}
}

// TestBuildWebChatFeedbackProperty sweeps a matrix of block sizes (keep, cut,
// drop combinations) and asserts the cap invariant on every output.
func TestBuildWebChatFeedbackProperty(t *testing.T) {
	blk := func(n int) string { return "<tool_result>" + strings.Repeat("x", n) + "</tool_result>" }
	sizes := []int{
		1, 100, webChatMaxInputRunes / 4, webChatMaxInputRunes / 2,
		webChatMaxInputRunes - webChatOutputNoteReserve - 100, webChatMaxInputRunes + 7,
		2 * webChatMaxInputRunes,
	}
	for _, sa := range sizes {
		for _, sb := range sizes {
			got := buildWebChatFeedback([]string{blk(sa), blk(sb)})
			if n := countRunes(got); n > webChatMaxInputRunes {
				t.Fatalf("sizes %d,%d: feedback %d runes > cap", sa, sb, n)
			}
			if !strings.HasSuffix(got, "</tool_result>") {
				t.Errorf("sizes %d,%d: feedback does not end with a closed block", sa, sb)
			}
		}
	}
	// Three-block combinations of a few interesting sizes.
	tri := []int{1, 1000, webChatMaxInputRunes - 300, webChatMaxInputRunes / 2}
	for _, sa := range tri {
		for _, sb := range tri {
			for _, sc := range tri {
				got := buildWebChatFeedback([]string{blk(sa), blk(sb), blk(sc)})
				if n := countRunes(got); n > webChatMaxInputRunes {
					t.Fatalf("sizes %d,%d,%d: feedback %d runes > cap", sa, sb, sc, n)
				}
				if !strings.HasSuffix(got, "</tool_result>") {
					t.Errorf("sizes %d,%d,%d: feedback does not end with a closed block", sa, sb, sc)
				}
			}
		}
	}
}

// TestRunesMetricPrefersChinese: the same character budget carries ~3x more
// bytes of Chinese than ASCII, proving the cap is a rune count (as the site
// counts it) - a byte cap would cut Chinese at 1/3 of its real budget.
func TestRunesMetricPrefersChinese(t *testing.T) {
	// 155000 ASCII runes ≈ 155000 bytes; 155000 Chinese runes = 465000 bytes.
	ascii := strings.Repeat("a", webChatMaxInputRunes)
	cn := strings.Repeat("深", webChatMaxInputRunes)
	if countRunes(ascii) > webChatMaxInputRunes || countRunes(cn) > webChatMaxInputRunes {
		t.Error("both must fit the rune cap")
	}
	if len(ascii) > webChatMaxInputRunes {
		t.Errorf("ascii bytes %d exceed byte cap while fitting rune cap", len(ascii))
	}
	if len(cn) <= webChatMaxInputRunes {
		t.Errorf("chinese bytes %d should be ~3x the cap (runes metric)", len(cn))
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
	in := strings.Repeat("字", webChatMaxInputRunes+100) // 3x cap in bytes
	if _, err := HandleWebChat(context.Background(), in, WebChatOptions{}); err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if n := countRunes(sent); n > webChatMaxInputRunes {
		t.Fatalf("sent %d runes, want <= %d", n, webChatMaxInputRunes)
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
	if n := countRunes(feedback); n > webChatMaxInputRunes {
		t.Fatalf("feedback %d runes, want <= %d", n, webChatMaxInputRunes)
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
