package lp

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

// errProbeCDP is the injected click-dispatch failure. Using one package-level
// value (rather than an inline errors.New) lets the table assert that the
// capped error wraps exactly this error as well as ErrTruncated.
var errProbeCDP = errors.New("probe cdp failure")

// intPtr returns a pointer to v, for optional table expectations.
func intPtr(v int) *int { return &v }

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

	// regen is the stopped-state detector's outcome. regenDetected records
	// whether that seam was consulted at all, so a case can assert that the
	// priority rule really short-circuits the 「重新生成」 branch.
	regen         regenerateDetect
	regenErr      error
	regenDetected bool

	detects   int
	regenRuns int
	clicks    int
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
		detectRegen: func(context.Context) (regenerateDetect, error) {
			h.regenRuns++
			h.regenDetected = true
			return h.regen, h.regenErr
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

// regenVisible is the common "regen button present and clickable" outcome:
// the stopped state with an idle generation.
func regenVisible() regenerateDetect {
	return regenerateDetect{present: true, clickable: true, x: 30, y: 40, buttons: 5}
}

// regenBlocked is a present but disabled/occluded regen button.
func regenBlocked() regenerateDetect {
	return regenerateDetect{present: true, clickable: false, x: 30, y: 40, buttons: 5}
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
		// wantRegenClicks pins the 「重新生成」 counter (no row skips it).
		wantRegenClicks int
		// wantFailures, when set, pins the consecutive-failure counter.
		wantFailures *int
		// wantCDPErr asserts the returned error also wraps errProbeCDP.
		wantCDPErr bool
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
				h.clickErr = errProbeCDP
			},
			body:         "半截回复",
			hasAnswer:    true,
			answer:       "半截回复",
			wantAction:   continueHold,
			wantAttempts: 1,
			wantClicks:   0,
			wantFailures: intPtr(1),
		},
		{
			name: "consecutive dispatch failures end in ErrTruncated",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				h.clickErr = errProbeCDP
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
			wantFailures: intPtr(webChatMaxContinueClickFailures),
			// Pins the archive's claim that errors.Is reaches BOTH the
			// retryable sentinel and the last CDP error.
			wantCDPErr: true,
		},
		{
			name: "dispatch failure then success resets failures",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.detect = visible()
				h.clickErr = errProbeCDP
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
			// The successful click must clear the failure counter.
			wantFailures: intPtr(0),
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
			assertRecoveryOutcome(t, h, r, action, err, tc.wantAction, tc.wantErr,
				tc.wantAttempts, tc.wantClicks, tc.wantRegenClicks, tc.wantFailures, tc.wantCDPErr)
		})
	}
}

// assertRecoveryOutcome checks one table row's outcome. Extracted so the table
// loop stays simple (gocyclo) while the assertions remain exhaustive:
//
//   - the step error is nil exactly when wanted, and wraps wantedErr;
//   - the action matches;
//   - the seam's dispatch count and the recovery's successful-click count are
//     asserted separately, so a failed dispatch cannot hide in either;
//   - the consecutive-failure counter, when pinned, matches;
//   - when wanted, the error also wraps the injected CDP error (pinning the
//     double-%w claim).
func assertRecoveryOutcome(
	t *testing.T,
	h *recoveryHarness,
	r *continueRecovery,
	action continueAction,
	err error,
	wantAction continueAction,
	wantErr error,
	wantAttempts, wantClicks int,
	wantRegenClicks int,
	wantFailures *int,
	wantCDPErr bool,
) {
	t.Helper()
	if wantErr == nil {
		if err != nil {
			t.Fatalf("step error = %v, want nil", err)
		}
	} else if !errors.Is(err, wantErr) {
		t.Fatalf("step error = %v, want %v", err, wantErr)
	}
	if action != wantAction {
		t.Errorf("action = %v, want %v", action, wantAction)
	}
	if h.clicks != wantAttempts {
		t.Errorf("clickAt calls = %d, want %d", h.clicks, wantAttempts)
	}
	if r.clicks != wantClicks {
		t.Errorf("r.clicks = %d, want %d", r.clicks, wantClicks)
	}
	if r.regenClicks != wantRegenClicks {
		t.Errorf("r.regenClicks = %d, want %d", r.regenClicks, wantRegenClicks)
	}
	if wantFailures != nil && r.failures != *wantFailures {
		t.Errorf("r.failures = %d, want %d", r.failures, *wantFailures)
	}
	if wantCDPErr && !errors.Is(err, errProbeCDP) {
		t.Errorf("err = %v, want it to wrap the injected CDP error %v", err, errProbeCDP)
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

// TestContinueRecoveryRegenerateStep covers the 「重新生成」 branch: the
// stopped-with-no-output state where the site renders no 「继续生成」 button.
// The 「继续生成」 branch itself is covered by TestContinueRecoveryStep; here
// the continue detector reports absent so the step falls through.
func TestContinueRecoveryRegenerateStep(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(h *recoveryHarness, r *continueRecovery)
		body       string
		answer     string
		wantAction continueAction
		wantErr    error
		// wantAttempts counts clickAt dispatches (successful + failed).
		wantAttempts int
		// wantRegenClicks pins the successful 「重新生成」 counter.
		wantRegenClicks int
		// wantPendingKind pins the pending provenance after the step.
		wantPendingKind pendingClick
		// wantRegenRuns pins how many times the regen detector was consulted.
		wantRegenRuns int
		// wantCDPErr asserts the returned error also wraps errProbeCDP (the
		// double-%w claim for the regen dispatch-failure path).
		wantCDPErr bool
	}{
		{
			name:            "stopped state and idle clicks regenerate",
			setup:           func(h *recoveryHarness, r *continueRecovery) { h.regen = regenVisible() },
			body:            "已停止",
			wantAction:      continueClicked,
			wantAttempts:    1,
			wantRegenClicks: 1,
			wantPendingKind: pendingRegen,
			wantRegenRuns:   1,
		},
		{
			name: "active generation never clicks regenerate",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				h.active = true
			},
			body:          "已停止 但生成活跃",
			wantAction:    continueNone,
			wantAttempts:  0,
			wantRegenRuns: 0,
		},
		{
			// A pending resume whose generation turned active is CONFIRMED:
			// the gate clears pending and the round proceeds.
			name: "pending plus active clears the resume",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				if !r.pending || r.pendingKind != pendingRegen {
					t.Fatalf("seed did not arm a regenerate resume: pending=%v kind=%v", r.pending, r.pendingKind)
				}
				h.active = true
			},
			body:            "已停止",
			wantAction:      continueNone,
			wantAttempts:    1,
			wantRegenClicks: 1,
			wantPendingKind: pendingNone,
			wantRegenRuns:   1,
		},
		{
			name:            "present but blocked holds without clicking",
			setup:           func(h *recoveryHarness, r *continueRecovery) { h.regen = regenBlocked() },
			body:            "已停止",
			wantAction:      continueHold,
			wantAttempts:    0,
			wantRegenClicks: 0,
			wantPendingKind: pendingNone,
			wantRegenRuns:   1,
		},
		{
			name: "cooldown elapsed clicks again within budget",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				if _, err := r.step(context.Background(), "已停止 v1", func() string { return "" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				h.clock.adv(webChatContinueCooldown)
			},
			body:            "已停止 v2",
			wantAction:      continueClicked,
			wantAttempts:    2,
			wantRegenClicks: 2,
			wantPendingKind: pendingRegen,
			wantRegenRuns:   2,
		},
		{
			name: "cooldown not elapsed holds",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				// No clock advance: the cooldown still runs.
			},
			body:            "已停止",
			wantAction:      continueHold,
			wantAttempts:    1,
			wantRegenClicks: 1,
			wantPendingKind: pendingRegen,
			wantRegenRuns:   2,
		},
		{
			name: "regenerate budget exhausted is ErrTruncated",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				for i := 0; i < webChatMaxRegenerates; i++ {
					if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
						t.Fatalf("seed step %d: %v", i, err)
					}
					h.clock.adv(webChatContinueCooldown)
				}
			},
			body:            "已停止",
			wantErr:         ErrTruncated,
			wantAction:      continueNone,
			wantAttempts:    webChatMaxRegenerates,
			wantRegenClicks: webChatMaxRegenerates,
			wantPendingKind: pendingRegen,
			wantRegenRuns:   webChatMaxRegenerates + 1,
		},
		{
			name: "regenerate dispatch failure does not consume budget",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				h.clickErr = errProbeCDP
			},
			body:            "已停止",
			wantAction:      continueHold,
			wantAttempts:    1,
			wantRegenClicks: 0,
			wantPendingKind: pendingNone,
			wantRegenRuns:   1,
		},
		{
			name: "consecutive regenerate dispatch failures end in ErrTruncated",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				h.clickErr = errProbeCDP
				for i := 0; i < webChatMaxContinueClickFailures-1; i++ {
					if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
						t.Fatalf("seed failure step %d: %v", i, err)
					}
					h.clock.adv(webChatContinueCooldown)
				}
			},
			body:            "已停止",
			wantErr:         ErrTruncated,
			wantAction:      continueNone,
			wantAttempts:    webChatMaxContinueClickFailures,
			wantRegenClicks: 0,
			wantPendingKind: pendingNone,
			wantRegenRuns:   webChatMaxContinueClickFailures,
			wantCDPErr:      true,
		},
		{
			name:            "regenerate detect error holds below the cap",
			setup:           func(h *recoveryHarness, r *continueRecovery) { h.regenErr = errors.New("evaluate failed") },
			body:            "已停止",
			wantAction:      continueHold,
			wantAttempts:    0,
			wantRegenClicks: 0,
			wantPendingKind: pendingNone,
			wantRegenRuns:   1,
		},
		{
			// Item 3a: a PERSISTENT regen-detect error must not hold a
			// healthy round forever. After the cap the step falls through to
			// the normal flow.
			name: "regen detect error past the cap falls through",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regenErr = errors.New("evaluate failed")
				for i := 0; i < webChatMaxRegenDetectFailures-1; i++ {
					if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
						t.Fatalf("seed error step %d: %v", i, err)
					}
				}
			},
			body:            "已停止",
			wantAction:      continueNone,
			wantAttempts:    0,
			wantRegenClicks: 0,
			wantPendingKind: pendingNone,
			wantRegenRuns:   webChatMaxRegenDetectFailures,
		},
		{
			// Item 3b: while a resume is pending the cap does NOT apply - the
			// click is in flight and only the resume window may fail it.
			name: "regen detect error with pending keeps holding past the cap",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
					t.Fatalf("seed click step: %v", err)
				}
				h.regenErr = errors.New("evaluate failed")
				for i := 0; i < webChatMaxRegenDetectFailures+1; i++ {
					if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
						t.Fatalf("seed error step %d: %v", i, err)
					}
				}
			},
			body:            "已停止",
			wantAction:      continueHold,
			wantAttempts:    1,
			wantRegenClicks: 1,
			wantPendingKind: pendingRegen,
			wantRegenRuns:   webChatMaxRegenDetectFailures + 3,
		},
		{
			name: "stopped state gone and pending cleared proceeds",
			setup: func(h *recoveryHarness, r *continueRecovery) {
				h.regen = regenVisible()
				if _, err := r.step(context.Background(), "已停止 v1", func() string { return "" }); err != nil {
					t.Fatalf("seed step: %v", err)
				}
				h.regen = regenerateDetect{}
				h.clock.adv(webChatContinueCooldown)
			},
			body:            "完整回复",
			wantAction:      continueNone,
			wantAttempts:    1,
			wantRegenClicks: 1,
			wantPendingKind: pendingNone,
			wantRegenRuns:   2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newRecoveryHarness()
			r := h.newRecovery()
			if tc.setup != nil {
				tc.setup(h, r)
			}
			action, err := r.step(context.Background(), tc.body, func() string { return tc.answer })
			// Reuse the shared outcome assertions: wantClicks=0 pins that the
			// continue counter NEVER advances on a regen row.
			assertRecoveryOutcome(t, h, r, action, err, tc.wantAction, tc.wantErr,
				tc.wantAttempts, 0, tc.wantRegenClicks, nil, tc.wantCDPErr)
			if r.pendingKind != tc.wantPendingKind {
				t.Errorf("pendingKind = %v, want %v", r.pendingKind, tc.wantPendingKind)
			}
			if h.regenRuns != tc.wantRegenRuns {
				t.Errorf("regen detector runs = %d, want %d", h.regenRuns, tc.wantRegenRuns)
			}
		})
	}
}

// TestContinueRecoveryRegenDetectFailureReset pins the other half of item 3:
// once the regen detector succeeds again, the consecutive-failure counter
// resets, so a later error gets a fresh cap instead of falling through
// immediately.
func TestContinueRecoveryRegenDetectFailureReset(t *testing.T) {
	h := newRecoveryHarness()
	r := h.newRecovery()

	// Two errors: below the cap, still holding.
	h.regenErr = errors.New("evaluate failed")
	for i := 0; i < webChatMaxRegenDetectFailures-1; i++ {
		action, err := r.step(context.Background(), "已停止", func() string { return "" })
		if err != nil {
			t.Fatalf("error step %d: %v", i, err)
		}
		if action != continueHold {
			t.Fatalf("error step %d action = %v, want hold", i, action)
		}
	}
	if r.regenDetectFailures != webChatMaxRegenDetectFailures-1 {
		t.Fatalf("regenDetectFailures = %d, want %d", r.regenDetectFailures, webChatMaxRegenDetectFailures-1)
	}

	// A successful poll resets the counter and re-arms normal detection.
	h.regenErr = nil
	h.regen = regenerateDetect{}
	action, err := r.step(context.Background(), "已停止", func() string { return "" })
	if err != nil {
		t.Fatalf("successful step: %v", err)
	}
	if action != continueNone {
		t.Errorf("successful step action = %v, want continueNone (no button)", action)
	}
	if r.regenDetectFailures != 0 {
		t.Errorf("regenDetectFailures = %d after a success, want 0", r.regenDetectFailures)
	}

	// A fresh error starts a new run: the first error holds again.
	h.regenErr = errors.New("evaluate failed")
	action, err = r.step(context.Background(), "已停止", func() string { return "" })
	if err != nil {
		t.Fatalf("post-reset error step: %v", err)
	}
	if action != continueHold {
		t.Errorf("post-reset error action = %v, want hold (the cap was reset)", action)
	}
}

// TestContinueRecoveryRegenerateBudgetSeparate pins that the two budgets are
// independent: spending the continue budget must not shrink the regenerate
// budget, and vice versa. Sharing one counter would let a flapping round
// exhaust the wrong allowance.
func TestContinueRecoveryRegenerateBudgetSeparate(t *testing.T) {
	h := newRecoveryHarness()
	r := h.newRecovery()

	// Spend the regenerate budget entirely on the stopped state.
	h.detect = continueDetect{}
	h.regen = regenVisible()
	for i := 0; i < webChatMaxRegenerates; i++ {
		if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
			t.Fatalf("seed regen step %d: %v", i, err)
		}
		h.clock.adv(webChatContinueCooldown)
	}
	if r.regenClicks != webChatMaxRegenerates {
		t.Fatalf("regenClicks = %d, want %d", r.regenClicks, webChatMaxRegenerates)
	}

	// The continue budget is untouched: an INCOMPLETE round can still resume
	// up to webChatMaxContinues times.
	h.detect = visible()
	h.regen = regenerateDetect{}
	h.clock.adv(webChatContinueCooldown)
	action, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" })
	if err != nil {
		t.Fatalf("continue step after regen budget: %v", err)
	}
	if action != continueClicked || r.clicks != 1 {
		t.Errorf("action = %v, clicks = %d; the continue budget must be independent of the regen budget", action, r.clicks)
	}
}

// TestContinueRecoveryPriorityContinueWins pins the recovery priority: when
// BOTH buttons are present, 「继续生成」 (a same-message resume) is clicked and
// the 「重新生成」 detector is never even consulted.
func TestContinueRecoveryPriorityContinueWins(t *testing.T) {
	h := newRecoveryHarness()
	h.detect = visible()
	h.regen = regenVisible()
	r := h.newRecovery()

	action, err := r.step(context.Background(), "半截回复", func() string { return "半截回复" })
	if err != nil {
		t.Fatalf("step: %v", err)
	}
	if action != continueClicked {
		t.Fatalf("action = %v, want continueClicked", action)
	}
	if r.clicks != 1 || r.regenClicks != 0 {
		t.Errorf("clicks = %d, regenClicks = %d; the continue button must win", r.clicks, r.regenClicks)
	}
	if r.pendingKind != pendingContinue {
		t.Errorf("pendingKind = %v, want continue", r.pendingKind)
	}
	if h.regenDetected {
		t.Error("the regenerate detector must not be consulted while the continue button is present")
	}
}

// TestContinueRecoveryRegenerateTimeoutNamesAction pins the timeout text: a
// pending 「重新生成」 resume that outlives the window must say so, not claim a
// 「继续生成」 click.
func TestContinueRecoveryRegenerateTimeoutNamesAction(t *testing.T) {
	h := newRecoveryHarness()
	h.regen = regenVisible()
	r := h.newRecovery()

	if _, err := r.step(context.Background(), "已停止", func() string { return "" }); err != nil {
		t.Fatalf("seed step: %v", err)
	}
	// The new attempt stopped the same way: no body, no active generation.
	h.regen = regenerateDetect{}
	h.clock.adv(webChatContinueResumeWindow + time.Second)
	_, err := r.step(context.Background(), "已停止", func() string { return "" })
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("step error = %v, want ErrTruncated", err)
	}
	if !strings.Contains(err.Error(), "重新生成") {
		t.Errorf("error = %v, want it to name 「重新生成」", err)
	}
}

// TestContinueRecoveryRegenResumeResetsFailureRun pins QA condition 1: a
// CONFIRMED resume (the gate clearing a pending click because the generation
// became active) restarts the regen-detect failure run. Errors accumulated
// during the pending window are hold-only by design; without the reset they
// would leak into the next unpended run and trip the cap on its first hiccup.
func TestContinueRecoveryRegenResumeResetsFailureRun(t *testing.T) {
	h := newRecoveryHarness()
	r := h.newRecovery()

	// 1. Seed a regen click: the stopped state with an idle generation.
	h.regen = regenVisible()
	action, err := r.step(context.Background(), "已停止", func() string { return "" })
	if err != nil {
		t.Fatalf("seed step: %v", err)
	}
	if action != continueClicked || !r.pending || r.pendingKind != pendingRegen {
		t.Fatalf("seed = (action %v, pending %v, kind %v), want a pending regen click", action, r.pending, r.pendingKind)
	}

	// 2. Detector errors while pending: hold-only, and the run grows past the
	// cap (the cap does not apply inside a pending window). step drives the
	// gate internally (production path), so no explicit gate call is needed
	// here.
	h.regenErr = errors.New("evaluate failed")
	for i := 0; i < webChatMaxRegenDetectFailures+2; i++ {
		action, err := r.step(context.Background(), "已停止", func() string { return "" })
		if err != nil {
			t.Fatalf("error step %d: %v", i, err)
		}
		if action != continueHold {
			t.Fatalf("error step %d action = %v, want hold (pending keeps holding)", i, action)
		}
	}
	if r.regenDetectFailures <= webChatMaxRegenDetectFailures {
		t.Fatalf("precondition failed: regenDetectFailures = %d, want > %d", r.regenDetectFailures, webChatMaxRegenDetectFailures)
	}
	// The run consumed its once-per-run warning latch.
	if !r.warnedRegen {
		t.Fatal("warnedRegen = false after the error run, want true (the latch was consumed)")
	}

	// 3. The generation becomes active: the gate confirms the resume, clears
	// pending, and must restart the failure run.
	h.active = true
	if err := r.gate(context.Background(), "已停止", func() string { return "" }); err != nil {
		t.Fatalf("resume gate: %v", err)
	}
	if r.pending {
		t.Fatal("pending must be cleared by a confirmed resume")
	}
	if r.regenDetectFailures != 0 {
		t.Fatalf("regenDetectFailures = %d after a confirmed resume, want 0 (a fresh run)", r.regenDetectFailures)
	}
	// A confirmed resume also re-arms the once-per-run warning latch.
	if r.warnedRegen {
		t.Fatal("warnedRegen = true after a confirmed resume, want false (a fresh run)")
	}

	// 4. Back to idle with the detector still broken: the fresh run must hold
	// for its first two errors and only fall through on the cap-th. The
	// exact counter values hard-pin the cap arithmetic, so this test breaks
	// loudly if webChatMaxRegenDetectFailures changes. The captured stderr
	// pins the observable once-per-run warning contract.
	h.active = false
	want := []continueAction{continueHold, continueHold, continueNone}
	wantFailures := []int{1, 2, webChatMaxRegenDetectFailures}
	drain := captureStderr(t)
	for i, exp := range want {
		action, err := r.step(context.Background(), "已停止", func() string { return "" })
		if err != nil {
			t.Fatalf("post-reset step %d: %v", i, err)
		}
		if action != exp {
			t.Errorf("post-reset step %d action = %v, want %v", i, action, exp)
		}
		if r.regenDetectFailures != wantFailures[i] {
			t.Errorf("post-reset step %d regenDetectFailures = %d, want %d", i, r.regenDetectFailures, wantFailures[i])
		}
		if i == 0 && !r.warnedRegen {
			t.Error("warnedRegen = false after the first post-reset error, want true (the fresh run logged once)")
		}
	}
	stderr := drain()
	if got := strings.Count(stderr, "检测「重新生成」按钮失败"); got != 1 {
		t.Errorf("per-run warning count = %d, want 1 (once per run); stderr:\n%s", got, stderr)
	}
}
