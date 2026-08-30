package toolcall

import (
	`strings`
	`testing`
)

// TestDSMLStrayCloseWithTruncation covers a truncated emission (an unclosed
// invoke open) combined with a stray close BEFORE the open: the stray is
// removed and the open chops the tail, so the stripped output carries no
// stray-fragment prefix (a leaked `/invoke` piece) and is empty.
func TestDSMLStrayCloseWithTruncation(t *testing.T) {
	text := invokeClose() + nl + invokeOpen(`shell`)
	got := StripDSMLToolCalls(text)
	if strings.Contains(got, lt+`/invoke`) {
		t.Errorf(`StripDSMLToolCalls leaked stray fragment: %q`, got)
	}
	if got != `` {
		t.Errorf(`StripDSMLToolCalls = %q, want empty`, got)
	}
	calls, err := ParseDSMLToolCalls(text)
	if err == nil {
		t.Errorf(`ParseDSMLToolCalls = %v, want truncation error (unclosed open)`, err)
	}
	if calls != nil {
		t.Errorf(`calls = %v, want nil (truncated emission)`, calls)
	}
}

// TestDSMLStrayCloseWithTruncationTrailing pins the trailing-stray variant:
// a stray close after a COMPLETE block is stripped, and a separate truncated
// open never confuses the stray into a stray-fragment leak.
func TestDSMLStrayCloseWithTruncationTrailing(t *testing.T) {
	text := invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, `echo A`) + nl +
		invokeClose() + nl +
		invokeClose() + nl +
		invokeOpen(`shell`)
	got := StripDSMLToolCalls(text)
	if strings.Contains(got, lt+`/invoke`) {
		t.Errorf(`StripDSMLToolCalls leaked stray fragment: %q`, got)
	}
	if got != `` {
		t.Errorf(`StripDSMLToolCalls = %q, want empty`, got)
	}
}
