package lp

// Endless-repetition detection and recovery for the WebChat wait loop.
//
// A model that falls into an endless repetition (the same block emitted over
// and over, e.g. `</parameter> </invoke>`) never stops on its own. The body
// text keeps GROWING, so the stability stage never sees three identical polls
// and never extracts; the site renders no 「继续生成」 button while the
// generation is still active, so the auto-continue recovery has nothing to
// click. The round then polls until the budget runs out and fails with
// `response timeout`, while the model keeps generating until it hits a
// resource limit.
//
// The escape hatch is the one users apply by hand: reply
// `CALM DOWN! NO PANIC!`. The state machine below detects the repetition,
// stops the generation (best effort), sends that nudge, and hands the nudge's
// answer back to the normal wait loop as this round's reply.

import (
	"context"
	"fmt"
	"os"
	"strings"
	"unicode"

	"github.com/chromedp/chromedp"
)

// Detection thresholds. The detector is deliberately CONSERVATIVE: a false
// positive interrupts a healthy generation, which costs more than a missed
// detection (the round then fails exactly as it does today), so every
// threshold sits well above what normal prose or code reaches.
const (
	// repeatWindowBytes bounds the scanned tail. Repetition always happens at
	// the newest output, so an older, non-repeating prefix is irrelevant.
	repeatWindowBytes = 32 << 10
	// repeatMaxUnit bounds the detected block length: periods longer than
	// this are (deliberately) not detected.
	repeatMaxUnit = 1024
	// Rich block (>= 3 distinct non-space runes): 2KB of exact repetition in
	// at least 24 copies is already far outside normal output.
	repeatRichMinRun    = 2048
	repeatRichMinCopies = 24
	// Uniform block (1-2 distinct non-space runes, e.g. a single-character
	// run): much cheaper to produce by accident (a long dash rule, a wide
	// table border), so it must be both longer and copied more often.
	repeatUniformMinRun    = 8192
	repeatUniformMinCopies = 64

	// webChatRepeatStopGracePolls bounds the wait for the stop click to take
	// effect (~6s at the poll interval). The nudge send has its own ack
	// window, so a missed stop must not stall the recovery.
	webChatRepeatStopGracePolls = 3

	// webChatCalmDownNudge is the message that breaks the loop: the exact
	// phrase users report working by hand in the web UI.
	webChatCalmDownNudge = "CALM DOWN! NO PANIC!"
)

// webChatMaxRepeatInterrupts caps how many CALM DOWN nudges one wait may send.
// A var rather than a const so tests can widen it. Exhausting the budget with
// the repetition still running ends the round as ErrTruncated (the retryable
// continue-from-here path) instead of polling out.
//
// A repeatRecovery may override it through its maxInterrupts field, which is
// how tests inject a budget without mutating this global (same convention as
// continueRecovery's seams).
var webChatMaxRepeatInterrupts = 1

// detectRepeatTail reports whether the text ends in an exact repetition of a
// block: unitLen is the block length in bytes and copies the number of
// consecutive copies. A whitespace-only block never triggers, and the match
// is tail-anchored (repetition happens at the newest output).
//
// The comparison is byte-exact. Multibyte UTF-8 is harmless: the comparison
// is between equal-length slices of the same stream, and periodicity aligns
// them, so a unit cut mid-rune still compares correctly.
func detectRepeatTail(s string) (unitLen, copies int, ok bool) {
	if len(s) > repeatWindowBytes {
		s = s[len(s)-repeatWindowBytes:]
	}
	for p := 1; p <= repeatMaxUnit && p*repeatRichMinCopies <= len(s); p++ {
		unit := s[len(s)-p:]
		k := 1
		for off := 2 * p; off <= len(s); off += p {
			if s[len(s)-off:len(s)-off+p] != unit {
				break
			}
			k++
		}
		// Cheap gate first: both branches need at least the rich thresholds,
		// so a block that fails them cannot qualify as uniform either.
		if k < repeatRichMinCopies || k*p < repeatRichMinRun {
			continue
		}
		switch distinct := distinctNonSpaceRunes(unit); {
		case distinct >= 3:
			return p, k, true
		case distinct >= 1 && k >= repeatUniformMinCopies && k*p >= repeatUniformMinRun:
			return p, k, true
		}
	}
	return 0, 0, false
}

// distinctNonSpaceRunes counts the distinct non-whitespace runes in s, capped
// at 4: the detector only needs to distinguish 0, 1-2 ("uniform") and >= 3
// ("rich"), so scanning further would be wasted work.
func distinctNonSpaceRunes(s string) int {
	var seen [4]rune
	n := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		dup := false
		for i := 0; i < n; i++ {
			if seen[i] == r {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		if n == len(seen) {
			return n
		}
		seen[n] = r
		n++
	}
	return n
}

// repeatInput is one poll's worth of input for repeatRecovery.step: the
// whole-page body text and the message this round is anchored on.
type repeatInput struct {
	current     string
	sentMessage string
}

// repeatActionKind tells webchatWait how to treat the poll.
type repeatActionKind int

const (
	// repeatNone: no endless repetition in sight; the normal continue and
	// stability stages may run.
	repeatNone repeatActionKind = iota
	// repeatBusy: the interrupt is in flight (the stop click was dispatched,
	// its effect is unconfirmed); keep polling and extract nothing.
	repeatBusy
	// repeatNudged: the CALM DOWN nudge was sent; the round's reply is now
	// the nudge's answer and the caller must re-anchor on it.
	repeatNudged
	// repeatAwaiting: the nudge is submitted but has not shown up in the page
	// yet; keep polling and run NOTHING else - neither the generic send-ack
	// (it would be satisfied trivially while a generation is active) nor the
	// content stages.
	repeatAwaiting
)

// repeatAction is the outcome of one repeatRecovery step.
type repeatAction struct {
	kind    repeatActionKind
	message string // repeatNudged: the nudge text, i.e. the new round anchor
	// mdBaseline is a fresh .ds-markdown baseline captured just before the
	// nudge was sent, so the extraction fallback anchors on the nudge round.
	// The body innerText baseline is deliberately NOT carried: the nudge gets
	// its own confirmation (see stepAwaiting), so nothing else compares
	// against it.
	mdBaseline string
}

// repeatPhase is the state machine's position.
type repeatPhase int

const (
	repeatPhaseIdle repeatPhase = iota
	repeatPhaseStopping
	repeatPhaseSending
	repeatPhaseAwaiting
	repeatPhaseSpent
)

// repeatRecovery tracks the endless-repetition interrupt across polls of a
// single webchatWait call: how many nudges were spent, whether a dispatched
// stop click has been confirmed, and where the machine currently is.
//
// It is deliberately per-call rather than shared state: a new wait gets a
// fresh budget, exactly like continueRecovery.
//
// The active/stop/clickAt/mdText/setValue/sendEnter/detect fields are
// injectable seams: nil means the production implementation. Tests substitute
// deterministic stand-ins to exercise the state machine without a browser.
type repeatRecovery struct {
	phase      repeatPhase
	used       int // nudges sent (budget: maxInterrupts / webChatMaxRepeatInterrupts)
	stopPolls  int // polls spent waiting for the stop click to take effect
	nudgePolls int // polls spent waiting for the nudge to appear in the page
	// nudgeSeenBefore is the marker count sampled just before the last nudge
	// was submitted. stepAwaiting requires the count to GROW, so an earlier
	// nudge's bubble cannot satisfy a later nudge's confirmation.
	nudgeSeenBefore int

	// maxInterrupts overrides webChatMaxRepeatInterrupts when non-zero. Tests
	// inject a budget here instead of mutating the package var.
	maxInterrupts int

	active    func(context.Context) bool
	stop      func(context.Context) (stopDetect, error)
	clickAt   func(context.Context, float64, float64) error
	mdText    func(context.Context) string
	setValue  func(context.Context, string) error
	sendEnter func(context.Context) error
	detect    func(string) (int, int, bool)
}

// budget is the effective CALM DOWN nudge budget for this wait.
func (r *repeatRecovery) budget() int {
	if r.maxInterrupts > 0 {
		return r.maxInterrupts
	}
	return webChatMaxRepeatInterrupts
}

// step runs one poll's worth of endless-repetition recovery. See the phase
// methods for the transition table.
func (r *repeatRecovery) step(ctx context.Context, in repeatInput) (repeatAction, error) {
	switch r.phase {
	case repeatPhaseStopping:
		return r.stepStopping(ctx, in)
	case repeatPhaseAwaiting:
		return r.stepAwaiting(in)
	case repeatPhaseSpent:
		return r.stepSpent(ctx, in)
	case repeatPhaseSending:
		// stepSending resolves within one call: a persisted sending phase
		// means the previous call returned an error, and the caller returns
		// on that error. Fail safe by starting over.
		r.phase = repeatPhaseIdle
	}
	return r.stepIdle(ctx, in)
}

// stepIdle looks for the repetition, and starts the interrupt when it finds
// one: stop the generation, then send the nudge.
func (r *repeatRecovery) stepIdle(ctx context.Context, in repeatInput) (repeatAction, error) {
	if _, _, ok := r.detectFn()(r.tailText(in)); !ok {
		return repeatAction{kind: repeatNone}, nil
	}
	return r.startInterrupt(ctx, in)
}

// startInterrupt runs the interrupt for a repetition that is ALREADY known:
// budget check, best-effort stop, then the nudge send. Split out of stepIdle
// so stepSpent - which has just detected - reuses it without scanning the
// tail a second time on the same input.
func (r *repeatRecovery) startInterrupt(ctx context.Context, in repeatInput) (repeatAction, error) {
	if r.used >= r.budget() {
		// The budget is already spent and the repetition is back: there is
		// nothing left to try.
		r.phase = repeatPhaseSpent
		return r.giveUp(ctx)
	}
	fmt.Fprintln(os.Stderr, "🛑 检测到输出无限重复（endless repetition），正在停止生成…")
	d, err := r.stopFn()(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ 检测「停止生成」按钮失败（改为直接发送 CALM DOWN 提示）: %v\n", err)
		r.phase = repeatPhaseSending
		return r.stepSending(ctx, in)
	}
	if !r.activeFn()(ctx) {
		// The generation ended on its own between the sample and the probe:
		// there is nothing to stop.
		r.phase = repeatPhaseSending
		return r.stepSending(ctx, in)
	}
	if !d.present || !d.clickable {
		fmt.Fprintf(os.Stderr, "⚠️ 「停止生成」按钮不可用（present=%v clickable=%v），直接发送 CALM DOWN 提示\n", d.present, d.clickable)
		r.phase = repeatPhaseSending
		return r.stepSending(ctx, in)
	}
	if err := r.clickFn()(ctx, d.x, d.y); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ 点击「停止生成」失败（改为直接发送 CALM DOWN 提示）: %v\n", err)
		r.phase = repeatPhaseSending
		return r.stepSending(ctx, in)
	}
	r.phase = repeatPhaseStopping
	r.stopPolls = 0
	return repeatAction{kind: repeatBusy}, nil
}

// stepStopping waits for the stop click to take effect, bounded by
// webChatRepeatStopGracePolls: the nudge must not depend on the stop ever
// being confirmed.
func (r *repeatRecovery) stepStopping(ctx context.Context, in repeatInput) (repeatAction, error) {
	r.stopPolls++
	if r.activeFn()(ctx) && r.stopPolls < webChatRepeatStopGracePolls {
		return repeatAction{kind: repeatBusy}, nil
	}
	r.phase = repeatPhaseSending
	return r.stepSending(ctx, in)
}

// stepSending sends the CALM DOWN nudge and re-anchors the round on it. The
// confirmation is left to the existing send-ack machinery: the caller resets
// sendAck/ackPolls, and sendAckStep/ackLoopEffectFor confirm the send exactly
// as they do for an initial message.
// stepSending SUBMITS the nudge and nothing more: it snapshots the markdown
// baseline and dispatches the textarea write plus Enter. Confirmation is
// stepAwaiting's job - the generic send-ack stage is deliberately NOT used
// for the nudge, because while a repetition runs a generation is active and
// the body keeps growing, so every one of its signals is satisfied trivially.
func (r *repeatRecovery) stepSending(ctx context.Context, in repeatInput) (repeatAction, error) {
	mdBaseline := r.mdFn()(ctx)
	if err := r.setValueFn()(ctx, webChatCalmDownNudge); err != nil {
		return repeatAction{}, fmt.Errorf("%w: 发送 CALM DOWN 提示失败: %w", ErrTruncated, err)
	}
	if err := r.sendEnterFn()(ctx); err != nil {
		return repeatAction{}, fmt.Errorf("%w: 发送 CALM DOWN 提示失败: %w", ErrTruncated, err)
	}
	r.used++
	r.phase = repeatPhaseAwaiting
	r.nudgePolls = 0
	// Count, do not test presence: with more than one nudge budget the page
	// still holds the PREVIOUS nudge's bubble, which would otherwise satisfy
	// this nudge's confirmation instantly.
	r.nudgeSeenBefore = strings.Count(in.current, webChatCalmDownNudge)
	fmt.Fprintf(os.Stderr, "📨 已回复 `CALM DOWN! NO PANIC!`，等待模型继续（第 %d/%d 次）…\n", r.used, r.budget())
	return repeatAction{
		kind:       repeatNudged,
		message:    webChatCalmDownNudge,
		mdBaseline: mdBaseline,
	}, nil
}

// stepAwaiting confirms the nudge submission itself. The generic send-ack
// stage CANNOT do this: while a repetition runs, a generation is active and
// the body text keeps growing, so both of its evidence signals (an active
// generation, a body that differs from the baseline) hold trivially - a nudge
// that was silently dropped would be "acknowledged" forever and the round
// would fall back to the 600s poll timeout this whole design exists to
// avoid.
//
// The nudge's own marker is therefore the evidence: the site renders the
// submitted message as a user bubble, so the body text containing
// webChatCalmDownNudge proves the submit landed. The marker is COUNTED, not
// probed with Contains: an earlier nudge's bubble is still on the page, so a
// presence test would confirm a second nudge the moment it was submitted.
// The window reuses webChatConfirmPolls (the send-ack window); missing it is
// ErrTruncated, the retryable continue-from-here path.
func (r *repeatRecovery) stepAwaiting(in repeatInput) (repeatAction, error) {
	r.nudgePolls++
	if strings.Count(in.current, webChatCalmDownNudge) > r.nudgeSeenBefore {
		r.phase = repeatPhaseSpent
		return repeatAction{kind: repeatNone}, nil
	}
	if r.nudgePolls >= webChatConfirmPolls {
		return repeatAction{}, fmt.Errorf(
			"%w: CALM DOWN nudge was not acknowledged within %d polls",
			ErrTruncated, webChatConfirmPolls,
		)
	}
	return repeatAction{kind: repeatAwaiting}, nil
}

// stepSpent re-detects after a nudge was confirmed. A repetition that stopped
// lets the normal stages resume; one that came back is retried while the
// budget allows (that is what raising webChatMaxRepeatInterrupts buys) and
// otherwise ends the round as ErrTruncated, the retryable continue-from-here
// path.
func (r *repeatRecovery) stepSpent(ctx context.Context, in repeatInput) (repeatAction, error) {
	if _, _, ok := r.detectFn()(r.tailText(in)); !ok {
		return repeatAction{kind: repeatNone}, nil
	}
	if r.used < r.budget() {
		// startInterrupt, not stepIdle: the tail was just scanned, and
		// re-entering stepIdle would scan it again on the same input.
		return r.startInterrupt(ctx, in)
	}
	return r.giveUp(ctx)
}

// giveUp reports the unrecoverable repetition: a best-effort stop of the
// still-running generation, then ErrTruncated so the caller's existing
// truncated-reply policy takes over.
func (r *repeatRecovery) giveUp(ctx context.Context) (repeatAction, error) {
	fmt.Fprintf(os.Stderr, "⚠️ 无限重复未被打断（已发送 %d 次 CALM DOWN 提示）\n", r.used)
	if r.activeFn()(ctx) {
		if d, err := r.stopFn()(ctx); err == nil && d.present && d.clickable {
			if cerr := r.clickFn()(ctx, d.x, d.y); cerr != nil {
				fmt.Fprintf(os.Stderr, "⚠️ 点击「停止生成」失败: %v\n", cerr)
			}
		}
	}
	return repeatAction{}, fmt.Errorf("%w: endless repetition persisted after the CALM DOWN nudge", ErrTruncated)
}

// tailText is the detector's input: this round's reply text, i.e. the
// whole-page body after the sent-message bubble with the known UI chrome
// lines stripped.
func (r *repeatRecovery) tailText(in repeatInput) string {
	return cleanBodyResponse(extractAfterMessage(in.current, in.sentMessage))
}

func (r *repeatRecovery) activeFn() func(context.Context) bool {
	if r.active != nil {
		return r.active
	}
	return isGenerationActive
}

func (r *repeatRecovery) stopFn() func(context.Context) (stopDetect, error) {
	if r.stop != nil {
		return r.stop
	}
	return stopGenerationButton
}

func (r *repeatRecovery) clickFn() func(context.Context, float64, float64) error {
	if r.clickAt != nil {
		return r.clickAt
	}
	return clickTrustedAt
}

func (r *repeatRecovery) mdFn() func(context.Context) string {
	if r.mdText != nil {
		return r.mdText
	}
	return getAssistantText
}

func (r *repeatRecovery) setValueFn() func(context.Context, string) error {
	if r.setValue != nil {
		return r.setValue
	}
	return webchatSetValue
}

func (r *repeatRecovery) sendEnterFn() func(context.Context) error {
	if r.sendEnter != nil {
		return r.sendEnter
	}
	return sendEnterJS
}

func (r *repeatRecovery) detectFn() func(string) (int, int, bool) {
	if r.detect != nil {
		return r.detect
	}
	return detectRepeatTail
}

// sendEnterJS dispatches the Enter sequence on the composer, exactly as the
// initial send does.
func sendEnterJS(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.Evaluate(jsSendEnter, nil))
}
