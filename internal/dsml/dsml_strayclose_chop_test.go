package dsml

import (
	"strings"
	"testing"
)

// TestDSMLStrayCloseWithTruncation pins the interaction between stray closes
// and a truncated emission (an unclosed invoke open). Each shape asserts the
// exact stripped output, not just the absence of a leak, so a future refactor
// of the stray triage cannot silently change behavior:
//
//  1. complete block + trailing stray - the stray is removed, nothing remains.
//  2. stray BEFORE a truncated open - the stray is removed, the open chops
//     the tail, output is empty, and Parse reports truncation.
//  3. complete block + stray + truncated open - both the stray and the open
//     are removed; Parse still reports truncation.
//
// A stray that straddles the chop point (s.pos < end < s.end) cannot be
// constructed from real tag shapes: a close event and an unclosed open never
// overlap in bytes, so the defensive default branch of the triage switch is
// not exercised here by design.
func TestDSMLStrayCloseWithTruncation(t *testing.T) {
	complete := invokeOpen(`shell`) + nl + dsmlTestParam(`script`, `echo A`) + nl + invokeClose()

	tests := []struct {
		name      string
		text      string
		wantStrip string
		wantErr   bool
	}{
		{
			name:      "complete block plus trailing stray",
			text:      complete + nl + invokeClose(),
			wantStrip: ``,
			wantErr:   false,
		},
		{
			name:      "stray before truncated open",
			text:      invokeClose() + nl + invokeOpen(`shell`),
			wantStrip: ``,
			wantErr:   true,
		},
		{
			name:      "complete block plus stray plus truncated open",
			text:      complete + nl + invokeClose() + nl + invokeOpen(`shell`),
			wantStrip: ``,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripDSMLToolCalls(tt.text)
			if got != tt.wantStrip {
				t.Errorf(`StripDSMLToolCalls = %q, want %q`, got, tt.wantStrip)
			}
			if strings.Contains(got, lt+`/invoke`) {
				t.Errorf(`StripDSMLToolCalls leaked stray fragment: %q`, got)
			}
			calls, err := ParseDSMLToolCalls(tt.text)
			if tt.wantErr && err == nil {
				t.Errorf(`ParseDSMLToolCalls = %v, want truncation error`, err)
			}
			if !tt.wantErr && err != nil {
				t.Errorf(`ParseDSMLToolCalls: %v, want nil`, err)
			}
			if tt.wantErr && calls != nil {
				t.Errorf(`calls = %v, want nil (truncated emission)`, calls)
			}
		})
	}
}
