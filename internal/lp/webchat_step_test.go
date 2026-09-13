package lp

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestSendAckStep pins the send-ack stage's contract: the three ack routes,
// the sub-threshold path (webChatAckPending, the caller refreshes lastText),
// the stale-textarea re-dispatch (webChatNextPoll, the caller leaves lastText
// alone), budget exhaustion, and a failed re-dispatch.
func TestSendAckStep(t *testing.T) {
	tests := []struct {
		name         string
		current      string
		baseline     string
		active       bool
		cleared      bool
		resendErr    error
		ackPolls     int
		resendCount  int
		wantAcked    bool
		wantAction   webChatAction
		wantErr      error
		wantResends  int
		wantAckPolls int
	}{
		{
			name:         "ack via new body content",
			current:      "新内容",
			baseline:     "旧内容",
			wantAcked:    true,
			wantAction:   webChatProceed,
			wantAckPolls: 0,
		},
		{
			name:         "ack via active generation",
			current:      "同一内容",
			baseline:     "同一内容",
			active:       true,
			wantAcked:    true,
			wantAction:   webChatProceed,
			wantAckPolls: 0,
		},
		{
			name:         "ack via cleared textarea",
			current:      "同一内容",
			baseline:     "同一内容",
			cleared:      true,
			wantAcked:    true,
			wantAction:   webChatProceed,
			wantAckPolls: 0,
		},
		{
			name:         "sub-threshold is ack-pending",
			current:      "同一内容",
			baseline:     "同一内容",
			ackPolls:     1,
			wantAcked:    false,
			wantAction:   webChatAckPending,
			wantAckPolls: 2,
		},
		{
			name:         "stale textarea re-dispatch continues to the next poll",
			current:      "同一内容",
			baseline:     "同一内容",
			ackPolls:     webChatConfirmPolls - 1,
			wantAcked:    false,
			wantAction:   webChatNextPoll,
			wantResends:  1,
			wantAckPolls: 0,
		},
		{
			name:        "re-dispatch failure rejects the send",
			current:     "同一内容",
			baseline:    "同一内容",
			ackPolls:    webChatConfirmPolls - 1,
			resendErr:   errors.New("no textarea"),
			wantAcked:   false,
			wantAction:  webChatProceed,
			wantErr:     ErrSendRejected,
			wantResends: 0,
			// ackPolls was incremented before the failure and is not reset
			// on the error path (the original inline code did the same).
			wantAckPolls: webChatConfirmPolls,
		},
		{
			name:        "resend budget exhaustion rejects the send",
			current:     "同一内容",
			baseline:    "同一内容",
			ackPolls:    webChatConfirmPolls - 1,
			resendCount: webChatMaxResends,
			wantAcked:   false,
			wantAction:  webChatProceed,
			wantErr:     ErrSendRejected,
			wantResends: webChatMaxResends + 1,
			// Same as above: the increment precedes the budget check, so the
			// error path leaves the incremented value.
			wantAckPolls: webChatConfirmPolls,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ackPolls := tc.ackPolls
			resendCount := tc.resendCount
			lastResendAt := time.Unix(1_700_000_000, 0)
			seams := sendAckSeams{
				active:  func(context.Context) bool { return tc.active },
				cleared: func(context.Context) bool { return tc.cleared },
				resend:  func(context.Context) error { return tc.resendErr },
			}
			acked, action, err := sendAckStep(context.Background(), seams,
				tc.current, tc.baseline, &ackPolls, &resendCount, &lastResendAt)
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if acked != tc.wantAcked {
				t.Errorf("acked = %v, want %v", acked, tc.wantAcked)
			}
			if action != tc.wantAction {
				t.Errorf("action = %v, want %v", action, tc.wantAction)
			}
			if resendCount != tc.wantResends {
				t.Errorf("resendCount = %d, want %d", resendCount, tc.wantResends)
			}
			if ackPolls != tc.wantAckPolls {
				t.Errorf("ackPolls = %d, want %d", ackPolls, tc.wantAckPolls)
			}
		})
	}
}

// resendState is the round state resendStep mutates, gathered for assertion.
type resendState struct {
	ackPolls         int
	stableCount      int
	emptyStableCount int
	lastText         string
	cont             continueRecovery
}

// assertResendState checks the two mutually exclusive outcomes: a no-op leaves
// every counter untouched; a resend clears them all (including the continue
// recovery, which belongs to the abandoned send).
func assertResendState(t *testing.T, handled bool, s resendState) {
	t.Helper()
	if !handled {
		if s.ackPolls != 7 || s.stableCount != 7 || s.emptyStableCount != 7 || s.lastText != "stale" {
			t.Errorf("no-op mutated state: ackPolls=%d stable=%d empty=%d lastText=%q",
				s.ackPolls, s.stableCount, s.emptyStableCount, s.lastText)
		}
		if !s.cont.pending || s.cont.clicks != 2 {
			t.Errorf("no-op mutated continue recovery: %+v", s.cont)
		}
		return
	}
	if s.ackPolls != 0 || s.stableCount != 0 || s.emptyStableCount != 0 || s.lastText != "" {
		t.Errorf("resend did not reset round state: ackPolls=%d stable=%d empty=%d lastText=%q",
			s.ackPolls, s.stableCount, s.emptyStableCount, s.lastText)
	}
	if s.cont.pending || s.cont.clicks != 0 || s.cont.base != "" || !s.cont.deadline.IsZero() {
		t.Errorf("resend did not reset continue recovery: %+v", s.cont)
	}
}

// TestResendStep pins the failed-send retry stage: no button is a no-op, a
// hit resets the whole round state (including the continue recovery, whose
// pending/deadline belongs to the abandoned send), and an exhausted budget
// wraps ErrServerBusy.
func TestResendStep(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	tests := []struct {
		name        string
		detect      func(context.Context) (string, bool)
		lastResend  time.Time
		resendCount int
		wantHandled bool
		wantErr     error
		wantResends int
	}{
		{
			name:        "no retry button is a no-op",
			detect:      func(context.Context) (string, bool) { return "", false },
			lastResend:  now.Add(-time.Hour),
			wantHandled: false,
			wantResends: 0,
		},
		{
			name:        "cooldown not elapsed skips detection",
			detect:      func(context.Context) (string, bool) { return "重发", true },
			lastResend:  now.Add(-time.Second),
			wantHandled: false,
			wantResends: 0,
		},
		{
			name:        "hit resets the round state",
			detect:      func(context.Context) (string, bool) { return "重发", true },
			lastResend:  now.Add(-time.Hour),
			wantHandled: true,
			wantResends: 1,
		},
		{
			name:        "budget exhausted wraps ErrServerBusy",
			detect:      func(context.Context) (string, bool) { return "重发", true },
			lastResend:  now.Add(-time.Hour),
			resendCount: webChatMaxResends,
			wantHandled: false,
			wantErr:     ErrServerBusy,
			wantResends: webChatMaxResends + 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resendCount := tc.resendCount
			lastResendAt := tc.lastResend
			ackPolls := 7
			stableCount := 7
			emptyStableCount := 7
			lastText := "stale"
			cont := continueRecovery{pending: true, clicks: 2, base: "old"}

			seams := resendSeams{
				detect: tc.detect,
				now:    func() time.Time { return now },
			}
			handled, err := resendStep(context.Background(), seams, resendInput{
				resendCount:      &resendCount,
				lastResendAt:     &lastResendAt,
				ackPolls:         &ackPolls,
				stableCount:      &stableCount,
				emptyStableCount: &emptyStableCount,
				lastText:         &lastText,
				cont:             &cont,
			})
			if tc.wantErr == nil {
				if err != nil {
					t.Fatalf("err = %v, want nil", err)
				}
			} else if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if handled != tc.wantHandled {
				t.Errorf("handled = %v, want %v", handled, tc.wantHandled)
			}
			if resendCount != tc.wantResends {
				t.Errorf("resendCount = %d, want %d", resendCount, tc.wantResends)
			}
			assertResendState(t, tc.wantHandled, resendState{
				ackPolls:         ackPolls,
				stableCount:      stableCount,
				emptyStableCount: emptyStableCount,
				lastText:         lastText,
				cont:             cont,
			})
		})
	}
}
