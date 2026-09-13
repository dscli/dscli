package lp

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

// fakeClock drives continueRecovery's injected now() so cooldowns and resume
// deadlines are exercised deterministically (no sleeps).
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time      { return c.t }
func (c *fakeClock) adv(d time.Duration) { c.t = c.t.Add(d) }

// recoveryHarness bundles the injectable seams with observable call counts.
type recoveryHarness struct {
	clock    *fakeClock
	active   bool
	detect   continueDetect
	detErr   error
	clickErr error

	detects int
	clicks  int
}

func newRecoveryHarness() *recoveryHarness {
	return &recoveryHarness{clock: &fakeClock{t: time.Unix(1_700_000_000, 0)}}
}

// newRecovery wires a continueRecovery to the harness.
func (h *recoveryHarness) newRecovery() *continueRecovery {
	return &continueRecovery{
		now:    h.clock.now,
		active: func(context.Context) bool { return h.active },
		detect: func(context.Context) (continueDetect, error) {
			h.detects++
			return h.detect, h.detErr
		},
		clickAt: func(context.Context, float64, float64) error {
			h.clicks++
			return h.clickErr
		},
	}
}

// visible is the common "button present and clickable" detector outcome.
func visible() continueDetect {
	return continueDetect{present: true, clickable: true, label: "继续生成", x: 10, y: 20}
}

// blocked is a present but occluded button.
func blocked() continueDetect {
	return continueDetect{present: true, clickable: false, label: "继续生成", x: 10, y: 20}
}

func TestContinueRecoveryStep(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(h *recoveryHarness, r *continueRecovery)
		body       string
		answer     string
		hasAnswer  bool
		wantAction continueAction
		wantErr    error
		// wantAttempts counts clickAt dispatches (successful + failed);
		// wantClicks counts clicks that landed in the recovery state. A
		// dispatch failure must show up in the first and NOT the second.
		wantAttempts int
		wantClicks   int
	}{
		{
			name:         "click then hold while pending",
			setup:        func(h *recoveryHarness, r *continueRecovery) { h.detect = visible() },
			body:         "半截回复",
			hasAnswer:    true,
			answer:       "半截回复",
			wantAction:   continueClicked,
			wantAttempts: 1,
			wantClicks:   1,
		},
		{
			name:         "pending cleared when assistant text changes",
			wantAttempts: 1,
			wantClicks:   1,
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				// First step clicks and records the baseline.
				if _, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				h.clock.adv(webChatContinueCooldown)
				// The button is gone and the answer grew: resume confirmed.
				h.detect = continueDetect{}
			},
			body:       "半截回复 续写内容",
			hasAnswer:  true,
			answer:     "半截回复 续写内容",
			wantAction: continueNone,
		},
		{
			name:         "pending cleared when generation becomes active",
			wantAttempts: 1,
			wantClicks:   1,
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				if _, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				h.detect = continueDetect{}
				h.active = true
			},
			body:       "半截回复",
			hasAnswer:  true,
			answer:     "半截回复",
			wantAction: continueNone,
		},
		{
			name:         "pending past deadline is ErrTruncated",
			wantAttempts: 1,
			wantClicks:   1,
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				if _, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				h.detect = continueDetect{}
				h.clock.adv(webChatContinueResumeWindow + time.Second)
			},
			body:       "半截回复",
			hasAnswer:  true,
			answer:     "半截回复",
			wantErr:    ErrTruncated,
			wantAction: continueNone,
		},
		{
			name: "budget exhausted while button stays visible",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				for i := 0; i < webChatMaxContinues; i++ {
					body := fmt.Sprintf("半截回复 %d", i)
					if _, err := r.step(context.Background(), body, func() string { return body }); err != nil {
						t.Fatalf("seed step %d: %v", i, err)
					}
					// The resume is confirmed each time (text moved), so the
					// gate clears pending; the button stays visible.
					h.clock.adv(webChatContinueCooldown)
				}
			},
			body:         "半截回复 最后",
			hasAnswer:    true,
			answer:       "半截回复 最后",
			wantErr:      ErrTruncated,
			wantAction:   continueNone,
			wantAttempts: webChatMaxContinues,
			wantClicks:   webChatMaxContinues,
		},
		{
			name: "cooldown not elapsed holds without clicking",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				if _, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				// No clock advance: the cooldown is still running.
			},
			body:         "半截回复 变化",
			hasAnswer:    true,
			answer:       "半截回复 变化",
			wantAction:   continueHold,
			wantAttempts: 1,
			wantClicks:   1,
		},
		{
			name: "dispatch failure holds and does not consume budget",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				h.clickErr = errors.New("cdp boom")
			},
			body:         "半截回复",
			hasAnswer:    true,
			answer:       "半截回复",
			wantAction:   continueHold,
			wantAttempts: 1,
			wantClicks:   0,
		},
		{
			name: "consecutive dispatch failures end in ErrTruncated",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				h.clickErr = errors.New("cdp boom")
				for i := 0; i < webChatMaxContinueClickFailures-1; i++ {
					if _, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" }); err != nil {
						t.Fatalf("seed failure step %d: %v", i, err)
					}
					h.clock.adv(webChatContinueCooldown)
				}
			},
			body:         "半截回复",
			hasAnswer:    true,
			answer:       "半截回复",
			wantErr:      ErrTruncated,
			wantAction:   continueNone,
			wantAttempts: webChatMaxContinueClickFailures,
			wantClicks:   0,
		},
		{
			name: "dispatch failure then success resets failures",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				h.clickErr = errors.New("cdp boom")
				// One failure (below the cap) leaves failures=1.
				if _, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" }); err != nil {
					t.Fatalf("failure step: %v", err)
				}
				if r.failures != 1 {
					t.Fatalf("failures = %d, want 1 after one dispatch failure", r.failures)
				}
				// The next dispatch succeeds: failures must reset to 0 and
				// the click budget must advance by exactly one.
				h.clickErr = nil
				h.clock.adv(webChatContinueCooldown)
			},
			body:         "半截回复",
			hasAnswer:    true,
			answer:       "半截回复",
			wantAction:   continueClicked,
			wantAttempts: 2,
			wantClicks:   1,
		},
		{
			name:         "present but blocked holds",
			setup:        func(h *recoveryHarness, r *continueRecovery) { h.detect = blocked() },
			body:         "半截回复",
			hasAnswer:    true,
			answer:       "半截回复",
			wantAction:   continueHold,
			wantAttempts: 0,
			wantClicks:   0,
		},
		{
			name:         "detect error holds instead of reporting no interruption",
			wantAttempts: 0,
			wantClicks:   0,
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detErr = errors.New("evaluate failed")
			},
			body:       "半截回复",
			hasAnswer:  true,
			answer:     "半截回复",
			wantAction: continueHold,
		},
		{
			name:         "no button and no pending proceeds",
			wantAttempts: 0,
			wantClicks:   0,
			setup:        func(h *recoveryHarness, r *continueRecovery) {},
			body:         "完整回复",
			hasAnswer:    true,
			answer:       "完整回复",
			wantAction:   continueNone,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRecoveryHarness()
			r := h.newRecovery()
			if tc.setup != nil {
				tc.setup(h, r)
			}
			answer := func() string {
				if !tc.hasAnswer {
					return ""
				}
				return tc.answer
			}
			action, err := r.step(context.Background(), tc.body, answer)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("step error = %v, want nil", err)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("step error = %v, want %v", err, tc.wantErr)
			}
			if action != tc.wantAction {
				t.Errorf("action = %v, want %v", action, tc.wantAction)
			}
			// Assert both layers, unconditionally (no zero-value skip): a
			// dispatch attempt is not a click, so a failed dispatch must show
			// in wantAttempts while wantClicks stays at the successful count.
			if h.clicks != tc.wantAttempts {
				t.Errorf("clickAt calls = %d, want %d", h.clicks, tc.wantAttempts)
			}
			if r.clicks != tc.wantClicks {
				t.Errorf("r.clicks = %d, want %d", r.clicks, tc.wantClicks)
			}
		})
	}
}

// TestContinueRecoveryBaselineFromAnswer pins item 7: when the assistant
// content is readable, the baseline comes from it, so whole-page body churn
// (the click's own toast/button updates) cannot clear pending early.
func TestContinueRecoveryBaselineFromAnswer(t *testing.T) {
	h := newRecoveryHarness()
	h.detect = visible()
	r := h.newRecovery()

	if _, err := r.step(context.Background(), "page body v1", func() string { return "answer v1" }); err != nil {
		t.Fatalf("step: %v", err)
	}
	if !r.pending || !r.baseFromAnswer || r.base != "answer v1" {
		t.Fatalf("baseline = (%q, answer=%v, pending=%v), want (answer v1, true, true)", r.base, r.baseFromAnswer, r.pending)
	}

	// Body churn alone must NOT count as a resume.
	h.detect = continueDetect{}
	h.clock.adv(webChatContinueCooldown)
	action, err := r.step(context.Background(), "page body v2 churned", func() string { return "answer v1" })
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if action != continueHold || !r.pending {
		t.Errorf("action = %v, pending = %v; body churn must not clear pending", action, r.pending)
	}

	// A change in the SAME source does.
	action, err = r.step(context.Background(), "page body v2 churned", func() string { return "answer v2" })
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if action != continueNone || r.pending {
		t.Errorf("action = %v, pending = %v; answer change must clear pending", action, r.pending)
	}
}

// TestContinueRecoveryResumeBodyFallback pins item 3: once a click recorded an
// answer baseline, a LATER unreadable answer must fall back to the body
// comparison instead of stalling pending until the deadline.
func TestContinueRecoveryResumeBodyFallback(t *testing.T) {
	h := newRecoveryHarness()
	h.detect = visible()
	r := h.newRecovery()

	bodyBase := "page body v1"
	if _, err := r.step(context.Background(), bodyBase, func() string { return "answer v1" }); err != nil {
		t.Fatalf("step: %v", err)
	}
	if !r.baseFromAnswer || r.bodyBase != bodyBase {
		t.Fatalf("baseline = (answer=%v, bodyBase=%q), want (true, %q)", r.baseFromAnswer, r.bodyBase, bodyBase)
	}

	// The answer becomes unreadable, but the body moved: the body fallback
	// must clear pending.
	h.detect = continueDetect{}
	h.clock.adv(webChatContinueCooldown)
	action, err := r.step(context.Background(), "page body v2", func() string { return "" })
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if action != continueNone || r.pending {
		t.Errorf("action = %v, pending = %v; body fallback must clear pending when the answer is unreadable", action, r.pending)
	}
}

// TestContinueRecoveryResumeStaysPendingWithoutEvidence pins the other half:
// if BOTH sources are unchanged (answer unreadable, body identical), pending
// must survive.
func TestContinueRecoveryResumeStaysPendingWithoutEvidence(t *testing.T) {
	h := newRecoveryHarness()
	h.detect = visible()
	r := h.newRecovery()

	if _, err := r.step(context.Background(), "page body v1", func() string { return "answer v1" }); err != nil {
		t.Fatalf("step: %v", err)
	}
	h.detect = continueDetect{}
	h.clock.adv(webChatContinueCooldown)
	action, err := r.step(context.Background(), "page body v1", func() string { return "" })
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if action != continueHold || !r.pending {
		t.Errorf("action = %v, pending = %v; no evidence must keep pending", action, r.pending)
	}
}

// TestContinueRecoveryResumeGuardWithoutBodyBase pins the bodyBase != ""
// guard: a recovery that somehow carries an answer baseline but no body
// baseline must NOT treat any body text as a resume (that would clear pending
// on the very first poll). Constructing the struct directly is the only way to
// reach this state, which is exactly why the guard exists.
func TestContinueRecoveryResumeGuardWithoutBodyBase(t *testing.T) {
	h := newRecoveryHarness()
	r := h.newRecovery()
	// Hand-built state: answer baseline set, bodyBase deliberately empty.
	r.pending = true
	r.base = "answer v1"
	r.baseFromAnswer = true
	r.bodyBase = ""
	r.deadline = h.clock.t.Add(webChatContinueResumeWindow)

	// answer unreadable + body non-empty must NOT resume without a bodyBase.
	if r.resumed(context.Background(), "some body", func() string { return "" }) {
		t.Error("resumed() = true with an empty bodyBase; the guard must block the comparison")
	}
	// With a bodyBase set the comparison works again.
	r.bodyBase = "body v1"
	if !r.resumed(context.Background(), "body v2", func() string { return "" }) {
		t.Error("resumed() = false for a moved body once bodyBase is set")
	}
}

// TestContinueRecoveryBaselineFallback pins the fallback: with no readable
// assistant content, the body text is the baseline and its change counts.
func TestContinueRecoveryBaselineFallback(t *testing.T) {
	h := newRecoveryHarness()
	h.detect = visible()
	r := h.newRecovery()

	if _, err := r.step(context.Background(), "body v1", func() string { return "" }); err != nil {
		t.Fatalf("step: %v", err)
	}
	if r.baseFromAnswer || r.base != "body v1" {
		t.Fatalf("baseline = (%q, answer=%v), want (body v1, false)", r.base, r.baseFromAnswer)
	}

	h.detect = continueDetect{}
	action, err := r.step(context.Background(), "body v2", func() string { return "" })
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if action != continueNone || r.pending {
		t.Errorf("action = %v, pending = %v; body change must clear pending in fallback mode", action, r.pending)
	}
}
