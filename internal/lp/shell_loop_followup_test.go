package lp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/shellblock"
)

// mockShellExecOK replaces handleWebChatExecShell with a successful-run mock
// (script1.sh / script1.txt, exit 0) and restores it on cleanup.
func mockShellExecOK(t *testing.T) {
	t.Helper()
	orig := handleWebChatExecShell
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		return &shellblock.RunResult{Number: 1, ScriptPath: "/tmp/script1.sh", OutputPath: "/tmp/script1.txt", ExitCode: 0}, nil
	}
	t.Cleanup(func() { handleWebChatExecShell = orig })
}

// TestHandleWebChatShellFollowUpTransientResends: a busy/rejected feedback
// send produced no reply, so the retry must re-send the SAME feedback message
// verbatim (a continuation nudge would answer a reply that never happened).
// Both transient no-reply errors share the branch; pin them together so a
// future refactor cannot drop ErrSendRejected.
func TestHandleWebChatShellFollowUpTransientResends(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{name: "ErrServerBusy", err: ErrServerBusy},
		{name: "ErrSendRejected", err: ErrSendRejected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			origSend, origDelays := handleWebChatSend, handleWebChatRetryDelays
			t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origSend, origDelays })
			handleWebChatRetryDelays = []time.Duration{0}

			const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
			var messages []string
			calls := 0
			handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
				calls++
				messages = append(messages, msg)
				switch calls {
				case 1:
					return WebChatResult{Content: shellBlockTest, URL: convURL}, nil
				case 2:
					return WebChatResult{}, tc.err
				default:
					return WebChatResult{Content: shellFinalTest, URL: convURL}, nil
				}
			}
			mockShellExecOK(t)

			res, err := HandleWebChat(context.Background(), "do it", WebChatOptions{})
			if err != nil {
				t.Fatalf("HandleWebChat: %v", err)
			}
			if res.Content != shellFinalTest {
				t.Errorf("content = %q, want the final report", res.Content)
			}
			if calls != 3 {
				t.Fatalf("sends = %d, want 3 (initial + feedback + resend)", calls)
			}
			if messages[2] != messages[1] {
				t.Errorf("retry message = %q, want the original feedback %q", messages[2], messages[1])
			}
		})
	}
}

// TestHandleWebChatShellFollowUpSendFatal: a permanent follow-up failure
// (browser/network) is fatal - retrying mid-conversation is not safe.
func TestHandleWebChatShellFollowUpSendFatal(t *testing.T) {
	origSend := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origSend })

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	hardErr := errors.New("browser crashed")
	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		if calls == 1 {
			return WebChatResult{Content: shellBlockTest, URL: convURL}, nil
		}
		return WebChatResult{}, hardErr
	}
	mockShellExecOK(t)

	_, err := HandleWebChat(context.Background(), "do it", WebChatOptions{})
	if err == nil || !errors.Is(err, hardErr) {
		t.Fatalf("err = %v, want the permanent error in the chain", err)
	}
	if !strings.Contains(err.Error(), "feedback send") {
		t.Errorf("err = %v, want feedback-send context", err)
	}
	if calls != 2 {
		t.Errorf("sends = %d, want 2 (follow-up attempted once)", calls)
	}
}

// TestHandleWebChatShellFollowUpPersistentTruncation: a feedback send that
// stays truncated exhausts the shared backoff budget (1 initial send +
// len(delays)+1 follow-up attempts) and reports the truncation in the error
// chain instead of hanging.
func TestHandleWebChatShellFollowUpPersistentTruncation(t *testing.T) {
	origSend, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origSend, origDelays })
	handleWebChatRetryDelays = []time.Duration{0, 0}

	const convURL = "https://chat.deepseek.com/a/chat/s/convSHELL"
	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		if calls == 1 {
			return WebChatResult{Content: shellBlockTest, URL: convURL}, nil
		}
		return WebChatResult{}, ErrTruncated
	}
	mockShellExecOK(t)

	_, err := HandleWebChat(context.Background(), "do it", WebChatOptions{})
	if err == nil || !errors.Is(err, ErrTruncated) {
		t.Fatalf("err = %v, want ErrTruncated chain", err)
	}
	if want := 1 + len(handleWebChatRetryDelays) + 1; calls != want {
		t.Errorf("sends = %d, want %d (initial + retry budget)", calls, want)
	}
}
