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

	body, md string
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
	return &repeatHarness{body: "BODY-TEXT", md: "MD-BASELINE", detectOK: true}
}

// newRecovery wires a repeatRecovery to the harness.
func (h *repeatHarness) newRecovery() *repeatRecovery {
	return &repeatRecovery{
		active:   func(context.Context) bool { return h.active },
		stop:     func(context.Context) (stopDetect, error) { h.stops++; return h.stop, h.stopErr },
		clickAt:  func(context.Context, float64, float64) error { h.clicks++; return h.clickErr },
		bodyText: func(context.Context) string { return h.body },
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
	if act.baseline != "BODY-TEXT" || act.mdBaseline != "MD-BASELINE" {
		t.Errorf("baselines = (%q, %q), want the snapshot values", act.baseline, act.mdBaseline)
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
	act, err := r.step(context.Background(), repeatTestInput())
	if err == nil {
		t.Fatalf("step 2 returned no error (kind=%v), want ErrTruncated", act.kind)
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
	h.detectOK = false
	act, err := r.step(context.Background(), repeatTestInput())
	if err != nil {
		t.Fatalf("step 2: %v", err)
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

// TestRepeatRecoveryBudgetIsReadFromVar: the interrupt budget is a package
// var so tests (and a future policy change) can retune it; with the budget at
// zero the first trigger gives up without sending anything.
func TestRepeatRecoveryBudgetIsReadFromVar(t *testing.T) {
	orig := webChatMaxRepeatInterrupts
	webChatMaxRepeatInterrupts = 0
	t.Cleanup(func() { webChatMaxRepeatInterrupts = orig })

	h := newRepeatHarness()
	h.active = false
	r := h.newRecovery()
	_, err := r.step(context.Background(), repeatTestInput())
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("error = %v, want ErrTruncated", err)
	}
	if h.sets != 0 || h.enters != 0 {
		t.Errorf("a zero budget must not send: sets=%d enters=%d", h.sets, h.enters)
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
