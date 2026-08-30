package dsml

import (
	"strings"
	"testing"
)

// Angle bracket, double quote, backtick, and newline characters are built at
// runtime so this file stays transportable through DSML tool calls: literal
// brackets or quotes in a write_file content would be misread as markup and
// truncate the payload.
var (
	lt = string(rune(60))
	gt = string(rune(62))
	q  = string(rune(34))
	bt = string(rune(96))
	nl = string(rune(10))
)

func invokeOpen(name string) string {
	return lt + `invoke name=` + q + name + q + gt
}

func invokeClose() string {
	return lt + `/invoke` + gt
}

func dsmlTestParam(name, value string) string {
	return lt + `parameter name=` + q + name + q + ` string=` + q + `true` + q + gt + value + lt + `/parameter` + gt
}

// TestDSMLStrayCloseAfterCompleteCall is the regression for a real QA round
// (2026-08-30): the model emitted a complete invoke block and then a second
// closing invoke tag on its own line (a repeated close tag, token artifact).
// ParseDSMLToolCalls already ignored the stray (an empty-stack close is
// noise), but StripDSMLToolCalls left it behind, so IsPureDSMLToolCalls was
// false and the tool loop never ran - the call was silently dropped.
func TestDSMLStrayCloseAfterCompleteCall(t *testing.T) {
	one := invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, `sed -n '190,225p' runewidth.go`) + nl +
		dsmlTestParam(`summary`, `Read Condition.Truncate body`) + nl +
		invokeClose() + nl +
		invokeClose()
	two := one + nl + invokeClose()
	for name, text := range map[string]string{`one-stray`: one, `two-strays`: two} {
		t.Run(name, func(t *testing.T) {
			calls, err := ParseDSMLToolCalls(text)
			if err != nil {
				t.Fatalf(`ParseDSMLToolCalls: %v, want nil`, err)
			}
			if len(calls) != 1 {
				t.Fatalf(`calls = %d, want 1`, len(calls))
			}
			if calls[0].Name != `shell` {
				t.Errorf(`name = %q, want shell`, calls[0].Name)
			}
			if !IsPureDSMLToolCalls(text) {
				t.Error(`IsPureDSMLToolCalls = false, want true`)
			}
			if got := StripDSMLToolCalls(text); got != `` {
				t.Errorf(`StripDSMLToolCalls = %q, want empty`, got)
			}
			if !IsDSMLToolCallReply(text) {
				t.Error(`IsDSMLToolCallReply = false, want true`)
			}
		})
	}
}

// TestDSMLStrayCloseMultiCalls covers multiple complete calls followed by a
// trailing stray close: all calls parse, the stray is stripped, and the
// reply is still pure.
func TestDSMLStrayCloseMultiCalls(t *testing.T) {
	text := invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, `echo A`) + nl +
		invokeClose() + nl +
		invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, `echo B`) + nl +
		invokeClose() + nl +
		invokeClose()
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf(`ParseDSMLToolCalls: %v`, err)
	}
	if len(calls) != 2 {
		t.Fatalf(`calls = %d, want 2`, len(calls))
	}
	if calls[0].Name != `shell` || calls[1].Name != `shell` {
		t.Errorf(`names = %q, %q`, calls[0].Name, calls[1].Name)
	}
	if !IsPureDSMLToolCalls(text) {
		t.Error(`IsPureDSMLToolCalls = false, want true`)
	}
	if got := StripDSMLToolCalls(text); got != `` {
		t.Errorf(`StripDSMLToolCalls = %q, want empty`, got)
	}
}

// TestDSMLStrayCloseMidText covers a stray close BETWEEN two complete calls:
// the generalized tolerance removes it too, so the reply stays pure.
func TestDSMLStrayCloseMidText(t *testing.T) {
	text := invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, `echo A`) + nl +
		invokeClose() + nl +
		invokeClose() + nl +
		invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, `echo B`) + nl +
		invokeClose()
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf(`ParseDSMLToolCalls: %v`, err)
	}
	if len(calls) != 2 {
		t.Fatalf(`calls = %d, want 2`, len(calls))
	}
	if got := StripDSMLToolCalls(text); got != `` {
		t.Errorf(`StripDSMLToolCalls = %q, want empty`, got)
	}
	if !IsPureDSMLToolCalls(text) {
		t.Error(`IsPureDSMLToolCalls = false, want true`)
	}
}

// TestDSMLStrayCloseSafetyBoundaries pins the boundary the fix must NOT
// cross: a literal closing invoke tag inside a parameter value or quoted
// code is content, and a lone stray in plain prose still leaves prose
// residue so the reply is not a tool-call reply.
func TestDSMLStrayCloseSafetyBoundaries(t *testing.T) {
	innerVal := `echo ` + q + `a ` + invokeClose() + ` b` + q
	inValue := invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, innerVal) + nl +
		invokeClose()
	calls, err := ParseDSMLToolCalls(inValue)
	if err != nil {
		t.Fatalf(`ParseDSMLToolCalls(inValue): %v`, err)
	}
	if len(calls) != 1 {
		t.Fatalf(`calls = %d, want 1`, len(calls))
	}
	cmd, _ := calls[0].Args[`script`].(string)
	if want := `echo ` + q + `a ` + invokeClose() + ` b` + q; cmd != want {
		t.Errorf(`script = %q, want verbatim value`, cmd)
	}
	if got := StripDSMLToolCalls(inValue); got != `` {
		t.Errorf(`StripDSMLToolCalls(inValue) = %q, want empty`, got)
	}

	fence := bt + bt + bt
	fenced := `example:` + nl + fence + nl +
		invokeOpen(`shell`) + nl +
		dsmlTestParam(`script`, `ls`) + nl +
		invokeClose() + nl + fence + nl
	if IsPureDSMLToolCalls(fenced) {
		t.Error(`IsPureDSMLToolCalls(fenced) = true, want false (citation)`)
	}
	if got := StripDSMLToolCalls(fenced); got != strings.TrimSpace(fenced) {
		t.Errorf(`StripDSMLToolCalls(fenced) = %q, want unchanged quote`, got)
	}

	prose := `just a note` + nl + invokeClose()
	if got := StripDSMLToolCalls(prose); got != `just a note` {
		t.Errorf(`StripDSMLToolCalls(prose) = %q, want just a note`, got)
	}
	if IsPureDSMLToolCalls(prose) {
		t.Error(`IsPureDSMLToolCalls(prose) = true, want false`)
	}
	if IsDSMLToolCallReply(prose) {
		t.Error(`IsDSMLToolCallReply(prose) = true, want false`)
	}
}
