package lp

// Tests for the webChatMaxInputRunes truncation layer: user messages are cut
// before sending. The site rejects inputs past its CHARACTER (rune) limit
// with a toast (超出字数限制) that the wait loop would otherwise misread as a
// send failure. Live measurement (probe, 2026-08-29): ASCII 162000 runes
// pass, ~165339 runes are rejected; mixed Chinese+code 180000 BYTES (~105k
// runes) passes — the limit counts runes, not bytes.

import (
	"strings"
	"testing"
	"unicode/utf8"
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

// tail returns the last n runes of s (diagnostics for truncation asserts).
func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
