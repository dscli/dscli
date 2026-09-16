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
)

// repeatAction is the outcome of one repeatRecovery step.
type repeatAction struct {
	kind    repeatActionKind
	message string // repeatNudged: the nudge text, i.e. the new round anchor
	// baseline/mdBaseline are fresh baselines captured just before the nudge
	// was sent, so the ack and extraction stages anchor on the nudge round.
	baseline, mdBaseline string
}

// repeatPhase is the state machine's position.
type repeatPhase int

const (
	repeatIdle repeatPhase = iota
	repeatStopping
	repeatSending
	repeatSpent
)

// repeatRecovery tracks the endless-repetition interrupt across polls of a
// single webchatWait call: how many nudges were spent, whether a dispatched
// stop click has been confirmed, and where the machine currently is.
//
// It is deliberately per-call rather than shared state: a new wait gets a
// fresh budget, exactly like continueRecovery.
//
// The active/stop/clickAt/bodyText/mdText/setValue/sendEnter/detect fields
// are injectable seams: nil means the production implementation. Tests
// substitute deterministic stand-ins to exercise the state machine without a
// browser.
type repeatRecovery struct {
	phase     repeatPhase
	used      int // nudges sent (budget: webChatMaxRepeatInterrupts)
	stopPolls int // polls spent waiting for the stop click to take effect

	active    func(context.Context) bool
	stop      func(context.Context) (stopDetect, error)
	clickAt   func(context.Context, float64, float64) error
	bodyText  func(context.Context) string
	mdText    func(context.Context) string
	setValue  func(context.Context, string) error
	sendEnter func(context.Context) error
	detect    func(string) (int, int, bool)
}

// step runs one poll's worth of endless-repetition recovery. See the phase
// methods for the transition table.
func (r *repeatRecovery) step(ctx context.Context, in repeatInput) (repeatAction, error) {
	switch r.phase {
	case repeatStopping:
		return r.stepStopping(ctx, in)
	case repeatSpent:
		return r.stepSpent(ctx, in)
	case repeatSending:
		// stepSending resolves within one call: a persisted sending phase
		// means the previous call returned an error, and the caller returns
		// on that error. Fail safe by starting over.
		r.phase = repeatIdle
	}
	return r.stepIdle(ctx, in)
}

// stepIdle looks for the repetition, and starts the interrupt when it finds
// one: stop the generation, then send the nudge.
func (r *repeatRecovery) stepIdle(ctx context.Context, in repeatInput) (repeatAction, error) {
	if _, _, ok := r.detectFn()(r.tailText(in)); !ok {
		return repeatAction{kind: repeatNone}, nil
	}
	if r.used >= webChatMaxRepeatInterrupts {
		// The budget is already spent and the repetition is back: there is
		// nothing left to try.
		r.phase = repeatSpent
		return r.giveUp(ctx)
	}
	fmt.Fprintln(os.Stderr, "🛑 检测到输出无限重复（endless repetition），正在停止生成…")
	d, err := r.stopFn()(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ 检测「停止生成」按钮失败（改为直接发送 CALM DOWN 提示）: %v\n", err)
		r.phase = repeatSending
		return r.stepSending(ctx)
	}
	if !r.activeFn()(ctx) {
		// The generation ended on its own between the sample and the probe:
		// there is nothing to stop.
		r.phase = repeatSending
		return r.stepSending(ctx)
	}
	if !d.present || !d.clickable {
		fmt.Fprintf(os.Stderr, "⚠️ 「停止生成」按钮不可用（present=%v clickable=%v），直接发送 CALM DOWN 提示\n", d.present, d.clickable)
		r.phase = repeatSending
		return r.stepSending(ctx)
	}
	if err := r.clickFn()(ctx, d.x, d.y); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️ 点击「停止生成」失败（改为直接发送 CALM DOWN 提示）: %v\n", err)
		r.phase = repeatSending
		return r.stepSending(ctx)
	}
	r.phase = repeatStopping
	r.stopPolls = 0
	return repeatAction{kind: repeatBusy}, nil
}

// stepStopping waits for the stop click to take effect, bounded by
// webChatRepeatStopGracePolls: the nudge must not depend on the stop ever
// being confirmed.
func (r *repeatRecovery) stepStopping(ctx context.Context, _ repeatInput) (repeatAction, error) {
	r.stopPolls++
	if r.activeFn()(ctx) && r.stopPolls < webChatRepeatStopGracePolls {
		return repeatAction{kind: repeatBusy}, nil
	}
	r.phase = repeatSending
	return r.stepSending(ctx)
}

// stepSending sends the CALM DOWN nudge and re-anchors the round on it. The
// confirmation is left to the existing send-ack machinery: the caller resets
// sendAck/ackPolls, and sendAckStep/ackLoopEffectFor confirm the send exactly
// as they do for an initial message.
func (r *repeatRecovery) stepSending(ctx context.Context) (repeatAction, error) {
	baseline := r.bodyFn()(ctx)
	mdBaseline := r.mdFn()(ctx)
	if err := r.setValueFn()(ctx, webChatCalmDownNudge); err != nil {
		return repeatAction{}, fmt.Errorf("%w: 发送 CALM DOWN 提示失败: %w", ErrTruncated, err)
	}
	if err := r.sendEnterFn()(ctx); err != nil {
		return repeatAction{}, fmt.Errorf("%w: 发送 CALM DOWN 提示失败: %w", ErrTruncated, err)
	}
	r.used++
	r.phase = repeatSpent
	fmt.Fprintf(os.Stderr, "📨 已回复 `CALM DOWN! NO PANIC!`，等待模型继续（第 %d/%d 次）…\n", r.used, webChatMaxRepeatInterrupts)
	return repeatAction{
		kind:       repeatNudged,
		message:    webChatCalmDownNudge,
		baseline:   baseline,
		mdBaseline: mdBaseline,
	}, nil
}

// stepSpent only re-detects. A repetition that survived the nudge ends the
// round as ErrTruncated (the retryable continue-from-here path); a repetition
// that stopped lets the normal stages resume.
func (r *repeatRecovery) stepSpent(ctx context.Context, in repeatInput) (repeatAction, error) {
	if _, _, ok := r.detectFn()(r.tailText(in)); !ok {
		return repeatAction{kind: repeatNone}, nil
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

func (r *repeatRecovery) bodyFn() func(context.Context) string {
	if r.bodyText != nil {
		return r.bodyText
	}
	return bodyInnerText
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

// bodyInnerText reads the whole-page body text - the same expression the poll
// loop uses for its `current` sample.
func bodyInnerText(ctx context.Context) string {
	var s string
	if err := chromedp.Evaluate("document.body ? document.body.innerText : ''", &s).Do(ctx); err != nil {
		return ""
	}
	return s
}

// sendEnterJS dispatches the Enter sequence on the composer, exactly as the
// initial send does.
func sendEnterJS(ctx context.Context) error {
	return chromedp.Run(ctx, chromedp.Evaluate(jsSendEnter, nil))
}
