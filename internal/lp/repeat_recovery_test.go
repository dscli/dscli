package lp

import (
	"context"
	"errors"
	"testing"
)

// repeatHarness bundles the injectable seams of repeatRecovery with observable
// call counts, mirroring recoveryHarness for continueRecovery.
type repeatHarness struct {
	active   bool
	stop     stopDetect
	stopErr  error
	clickErr error

	md       string
	setErr   error
	enterErr error

	// detectOK is what the fake detector reports; detects records every tail
	// text the state machine handed to it.
	detectOK bool
	detects  []string

	stops  int
	clicks int
	sets   int
	enters int
}

func newRepeatHarness() *repeatHarness {
	return &repeatHarness{md: "MD-BASELINE", detectOK: true}
}

// newRecovery wires a repeatRecovery to the harness.
func (h *repeatHarness) newRecovery() *repeatRecovery {
	return &repeatRecovery{
		active:   func(context.Context) bool { return h.active },
		stop:     func(context.Context) (stopDetect, error) { h.stops++; return h.stop, h.stopErr },
		clickAt:  func(context.Context, float64, float64) error { h.clicks++; return h.clickErr },
		mdText:   func(context.Context) string { return h.md },
		setValue: func(context.Context, string) error { h.sets++; return h.setErr },
		sendEnter: func(context.Context) error {
			h.enters++
			return h.enterErr
		},
		detect: func(s string) (int, int, bool) {
			h.detects = append(h.detects, s)
			return 1, 100, h.detectOK
		},
	}
}

// input is a minimal repeatInput; the detect seam ignores it beyond recording
// the derived tail text.
func repeatTestInput() repeatInput {
	return repeatInput{current: "body", sentMessage: "sent"}
}

// TestRepeatRecoveryIdle: no repetition means no action and no side effect.
func TestRepeatRecoveryIdle(t *testing.T) {
	h := newRepeatHarness()
	h.detectOK = false
	r := h.newRecovery()
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if act.kind != repeatNone {
		t.Errorf("kind = %v, want repeatNone", act.kind)
	}
	if h.clicks != 0 || h.sets != 0 || h.enters != 0 {
		t.Errorf("idle must not touch the page: clicks=%d sets=%d enters=%d", h.clicks, h.sets, h.enters)
	}
}

// TestRepeatRecoveryStopThenNudge: active generation + clickable stop button →
// exactly one click, then one nudge once the generation is confirmed stopped.
func TestRepeatRecoveryStopThenNudge(t *testing.T) {
	h := newRepeatHarness()
	h.active = true
	h.stop = stopDetect{present: true, clickable: true, label: "停止", x: 10, y: 20}
	r := h.newRecovery()

	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step 1: %v", err)
	}
	if act.kind != repeatBusy {
		t.Fatalf("step 1 kind = %v, want repeatBusy", act.kind)
	}
	if h.clicks != 1 {
		t.Fatalf("clicks = %d, want 1", h.clicks)
	}
	if h.sets != 0 || h.enters != 0 {
		t.Fatalf("nudge sent while still stopping: sets=%d enters=%d", h.sets, h.enters)
	}

	h.active = false
	act, err = r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step 2: %v", err)
	}
	if act.kind != repeatNudged {
		t.Fatalf("step 2 kind = %v, want repeatNudged", act.kind)
	}
	if act.message != webChatCalmDownNudge {
		t.Errorf("message = %q, want %q", act.message, webChatCalmDownNudge)
	}
	if act.mdBaseline != "MD-BASELINE" {
		t.Errorf("mdBaseline = %q, want the snapshot value", act.mdBaseline)
	}
	if h.clicks != 1 {
		t.Errorf("clicks = %d, want exactly 1", h.clicks)
	}
	if h.sets != 1 || h.enters != 1 {
		t.Errorf("sets=%d enters=%d, want 1 and 1", h.sets, h.enters)
	}
}

// TestRepeatRecoveryDirectSendWhenIdle: an already-finished generation is not
// clicked, the nudge goes straight out.
func TestRepeatRecoveryDirectSendWhenIdle(t *testing.T) {
	h := newRepeatHarness()
	h.active = false
	h.stop = stopDetect{present: true, clickable: true}
	r := h.newRecovery()
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if act.kind != repeatNudged {
		t.Fatalf("kind = %v, want repeatNudged", act.kind)
	}
	if h.clicks != 0 {
		t.Errorf("clicks = %d, want 0 (nothing to stop)", h.clicks)
	}
	if h.sets != 1 || h.enters != 1 {
		t.Errorf("sets=%d enters=%d, want 1 and 1", h.sets, h.enters)
	}
}

// TestRepeatRecoveryProbeMissFallsBack: a detector that finds no stop button
// must not block the nudge.
func TestRepeatRecoveryProbeMissFallsBack(t *testing.T) {
	h := newRepeatHarness()
	h.active = true
	h.stop = stopDetect{} // found=false
	r := h.newRecovery()
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if act.kind != repeatNudged {
		t.Fatalf("kind = %v, want repeatNudged", act.kind)
	}
	if h.clicks != 0 {
		t.Errorf("clicks = %d, want 0", h.clicks)
	}
}

// TestRepeatRecoveryProbeErrorFallsBack: a probe error is not fatal either.
func TestRepeatRecoveryProbeErrorFallsBack(t *testing.T) {
	h := newRepeatHarness()
	h.active = true
	h.stopErr = errors.New("cdp hiccup")
	r := h.newRecovery()
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if act.kind != repeatNudged {
		t.Fatalf("kind = %v, want repeatNudged", act.kind)
	}
	if h.clicks != 0 {
		t.Errorf("clicks = %d, want 0", h.clicks)
	}
}

// TestRepeatRecoveryNotClickableFallsBack: a present but blocked stop button
// must not be clicked; the nudge still goes out.
func TestRepeatRecoveryNotClickableFallsBack(t *testing.T) {
	h := newRepeatHarness()
	h.active = true
	h.stop = stopDetect{present: true, clickable: false}
	r := h.newRecovery()
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if act.kind != repeatNudged {
		t.Fatalf("kind = %v, want repeatNudged", act.kind)
	}
	if h.clicks != 0 {
		t.Errorf("clicks = %d, want 0", h.clicks)
	}
}

// TestRepeatRecoveryClickErrorFallsBack: a failed click dispatch degrades to
// sending the nudge.
func TestRepeatRecoveryClickErrorFallsBack(t *testing.T) {
	h := newRepeatHarness()
	h.active = true
	h.stop = stopDetect{present: true, clickable: true}
	h.clickErr = errors.New("dispatch failed")
	r := h.newRecovery()
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if act.kind != repeatNudged {
		t.Fatalf("kind = %v, want repeatNudged", act.kind)
	}
	if h.sets != 1 {
		t.Errorf("sets = %d, want 1", h.sets)
	}
}

// TestRepeatRecoveryStopGraceExhausted: a stop that never takes effect must
// not stall the recovery - the grace window ends it and the nudge is sent.
func TestRepeatRecoveryStopGraceExhausted(t *testing.T) {
	h := newRepeatHarness()
	h.active = true // never stops
	h.stop = stopDetect{present: true, clickable: true}
	r := h.newRecovery()

	// Step 1 clicks and holds; the following polls hold until the grace is
	// spent, then the nudge goes out.
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step 1: %v", err)
	}
	if act.kind != repeatBusy {
		t.Fatalf("step 1 kind = %v, want repeatBusy", act.kind)
	}
	var nudged bool
	for i := 0; i < webChatRepeatStopGracePolls+1; i++ {
		act, err = r.step(context.Background(), repeatTestInput())
		if err != nil {
			t.Fatalf("step %d: %v", i+2, err)
		}
		if act.kind == repeatNudged {
			nudged = true
			break
		}
		if act.kind != repeatBusy {
			t.Fatalf("step %d kind = %v, want repeatBusy", i+2, act.kind)
		}
	}
	if !nudged {
		t.Fatalf("grace exhausted without sending the nudge")
	}
	if h.clicks != 1 {
		t.Errorf("clicks = %d, want 1 (click once, never loop on it)", h.clicks)
	}
	if h.sets != 1 {
		t.Errorf("sets = %d, want 1", h.sets)
	}
}

// TestRepeatRecoverySpentGivesUp: a repetition that survives the nudge ends the
// round as ErrTruncated instead of polling out.
func TestRepeatRecoverySpentGivesUp(t *testing.T) {
	h := newRepeatHarness()
	h.active = false // nudge path; no click needed
	r := h.newRecovery()

	if _, err := r.step(context.Background(), repeatTestInput()); err != nil {
		t.Fatalf("step 1: %v", err)
	}
	// The nudge is visible in the page: acknowledged, machine is spent.
	confirmed := repeatInput{current: "page shows " + webChatCalmDownNudge}
	if act, err := r.step(context.Background(), confirmed); err != nil {
		t.Fatalf("confirm: %v", err)
	} else if act.kind != repeatNone {
		t.Fatalf("confirm kind = %v, want repeatNone", act.kind)
	}
	// The repetition came back with the budget exhausted: give up.
	act, err := r.step(context.Background(), repeatTestInput())
	if err == nil {
		t.Fatalf("step 3 returned no error (kind=%v), want ErrTruncated", act.kind)
	}
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("error = %v, want ErrTruncated", err)
	}
	if h.sets != 1 || h.enters != 1 {
		t.Errorf("sets=%d enters=%d, want exactly one nudge", h.sets, h.enters)
	}
}

// TestRepeatRecoverySpentRecovers: once the repetition stops, the spent
// machine stands down and normal extraction may resume.
func TestRepeatRecoverySpentRecovers(t *testing.T) {
	h := newRepeatHarness()
	h.active = false
	r := h.newRecovery()
	if _, err := r.step(context.Background(), repeatTestInput()); err != nil {
		t.Fatalf("step 1: %v", err)
	}
	confirmed := repeatInput{current: "page shows " + webChatCalmDownNudge}
	if _, err := r.step(context.Background(), confirmed); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	h.detectOK = false
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step 3: %v", err)
	}
	if act.kind != repeatNone {
		t.Errorf("kind = %v, want repeatNone", act.kind)
	}
}

// TestRepeatRecoverySendError: a failed nudge submit is ErrTruncated, not a
// silent stall.
func TestRepeatRecoverySendError(t *testing.T) {
	h := newRepeatHarness()
	h.active = false
	h.setErr = errors.New("textarea gone")
	r := h.newRecovery()
	_, err := r.step(context.Background(), repeatTestInput())
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("error = %v, want ErrTruncated", err)
	}
	if h.enters != 0 {
		t.Errorf("enters = %d, want 0 after a failed setValue", h.enters)
	}
}

// TestRepeatRecoveryBudgetField: the budget is a struct field defaulting to
// webChatMaxRepeatInterrupts, so a test injects it instead of mutating the
// package global. A zero budget gives up on the first trigger.
func TestRepeatRecoveryBudgetField(t *testing.T) {
	h := newRepeatHarness()
	h.active = false
	r := h.newRecovery()
	// The default budget comes from the package var; simulate it exhausted.
	r.used = webChatMaxRepeatInterrupts
	_, err := r.step(context.Background(), repeatTestInput())
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("error = %v, want ErrTruncated", err)
	}
	if h.sets != 0 || h.enters != 0 {
		t.Errorf("an exhausted budget must not send: sets=%d enters=%d", h.sets, h.enters)
	}
}

// TestRepeatRecoveryBudgetOverride: a non-zero maxInterrupts overrides the
// package default, letting one wait send more nudges than the global allows.
func TestRepeatRecoveryBudgetOverride(t *testing.T) {
	h := newRepeatHarness()
	h.active = false
	r := h.newRecovery()
	r.maxInterrupts = 2

	for i := 1; i <= 2; i++ {
		if _, err := r.step(context.Background(), repeatTestInput()); err != nil {
			t.Fatalf("nudge %d: %v", i, err)
		}
		// Confirm the nudge so the machine returns to spent.
		confirmed := repeatInput{current: "page shows " + webChatCalmDownNudge}
		if _, err := r.step(context.Background(), confirmed); err != nil {
			t.Fatalf("confirm %d: %v", i, err)
		}
	}
	if h.sets != 2 || h.enters != 2 {
		t.Errorf("sets=%d enters=%d, want 2 and 2", h.sets, h.enters)
	}
	if _, err := r.step(context.Background(), repeatTestInput()); !errors.Is(err, ErrTruncated) {
		t.Errorf("after the budget the machine must give up, got %v", err)
	}
}

// TestRepeatRecoveryTailText: the detector is fed this round's reply tail
// (body text after the sent message, UI chrome stripped), not the raw page.
func TestRepeatRecoveryTailText(t *testing.T) {
	h := newRepeatHarness()
	h.detectOK = false
	r := h.newRecovery()
	current := "some history\n" + webChatCalmDownNudge + "\n" + "REPLY-TAIL"
	in := repeatInput{current: current, sentMessage: webChatCalmDownNudge}
	if _, err := r.step(context.Background(), in); err != nil {
		t.Fatalf("step: %v", err)
	}
	if len(h.detects) != 1 {
		t.Fatalf("detects = %d, want 1", len(h.detects))
	}
	if got := h.detects[0]; got != "REPLY-TAIL" {
		t.Errorf("detector input = %q, want %q", got, "REPLY-TAIL")
	}
}

// TestRepeatRecoveryAwaiting pins the nudge-specific confirmation: the
// generic send-ack cannot be used here (an active generation and a growing
// body satisfy it trivially), so the nudge's own marker in the page text is
// the evidence, within the send-ack window.
func TestRepeatRecoveryAwaiting(t *testing.T) {
	tests := []struct {
		name string
		// pages lists the body texts seen AFTER the nudge was sent, one per
		// poll.
		pages     []string
		wantErr   bool
		wantKinds []repeatActionKind
	}{
		{
			name:      "marker on the first poll acknowledges immediately",
			pages:     []string{"重复中…\n" + webChatCalmDownNudge + "\n"},
			wantKinds: []repeatActionKind{repeatNone},
		},
		{
			name: "marker appearing late still acknowledges",
			pages: []string{
				"重复中…", "重复中…", webChatCalmDownNudge,
			},
			wantKinds: []repeatActionKind{repeatAwaiting, repeatAwaiting, repeatNone},
		},
		{
			name: "no marker within the window is ErrTruncated",
			// One page per poll of webChatConfirmPolls; the last poll trips
			// the window and carries the error (its kind is the zero value).
			pages:     []string{"重复中…", "重复中…", "重复中…", "重复中…", "重复中…"},
			wantErr:   true,
			wantKinds: []repeatActionKind{repeatAwaiting, repeatAwaiting, repeatAwaiting, repeatAwaiting},
		},
		{
			name: "marker on the very last poll of the window still counts",
			pages: []string{
				"a", "b", "c", "d", webChatCalmDownNudge,
			},
			wantKinds: []repeatActionKind{
				repeatAwaiting, repeatAwaiting, repeatAwaiting, repeatAwaiting, repeatNone,
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRepeatHarness()
			h.active = false // straight to sending
			r := h.newRecovery()

			// The nudge is sent on the first step.
			act, err := r.step(context.Background(), repeatTestInput())
			if err != nil {
				t.Fatalf("send: %v", err)
			}
			if act.kind != repeatNudged {
				t.Fatalf("send kind = %v, want repeatNudged", act.kind)
			}
			if act.mdBaseline != "MD-BASELINE" {
				t.Errorf("mdBaseline = %q, want the snapshot", act.mdBaseline)
			}

			var gotErr error
			for i, page := range tc.pages {
				var got repeatAction
				got, gotErr = r.step(context.Background(), repeatInput{current: page})
				if gotErr != nil {
					// The window is closed: the error is the outcome and the
					// action kind is the zero value.
					break
				}
				if i >= len(tc.wantKinds) {
					t.Fatalf("unexpected extra step %d (kind=%v)", i, got.kind)
				}
				if want := tc.wantKinds[i]; got.kind != want {
					t.Errorf("step %d kind = %v, want %v", i+1, got.kind, want)
				}
			}
			// Every expected poll before the error must have been checked.
			if !tc.wantErr && len(tc.wantKinds) != len(tc.pages) {
				t.Fatalf("test fixture mismatch: %d kinds for %d pages", len(tc.wantKinds), len(tc.pages))
			}
			if tc.wantErr {
				if gotErr == nil {
					t.Fatalf("no error after the window, want ErrTruncated")
				}
				if !errors.Is(gotErr, ErrTruncated) {
					t.Errorf("error = %v, want ErrTruncated", gotErr)
				}
			} else if gotErr != nil {
				t.Fatalf("unexpected error: %v", gotErr)
			}
			// Whatever the outcome, exactly one nudge was ever dispatched.
			if h.sets != 1 || h.enters != 1 {
				t.Errorf("sets=%d enters=%d, want exactly 1 nudge", h.sets, h.enters)
			}
		})
	}
}
