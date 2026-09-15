package lp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/shellblock"
)

// resumeReadReply returns a mock read for handleWebChatResumeRead and a
// cleanup that restores it. The content is the conversation tail the resumed
// round was interrupted on; tailRole defaults to "ASSISTANT" (the tail is
// the assistant's reply) and the refusal reason is empty - mockResumeReadFull
// covers the other shapes.
func mockResumeRead(t *testing.T, content, status string, ok bool) {
	t.Helper()
	mockResumeReadFull(t, content, status, "ASSISTANT", "", ok)
}

// mockResumeReadFull is mockResumeRead with explicit tailRole and refusal
// reason values (the tail-guard and diagnostics cases).
func mockResumeReadFull(t *testing.T, content, status, tailRole, reason string, ok bool) {
	t.Helper()
	orig := handleWebChatResumeRead
	handleWebChatResumeRead = func(_ context.Context, _ string) (string, string, string, string, bool) {
		return content, status, tailRole, reason, ok
	}
	t.Cleanup(func() { handleWebChatResumeRead = orig })
}

// mockResumeResolve makes Keep resolve to a fixed URL and returns a cleanup.
func mockResumeResolve(t *testing.T, url string) {
	t.Helper()
	orig := handleWebChatResolveConversation
	handleWebChatResolveConversation = func(keep string) (string, error) {
		if keep == "" {
			return "", nil
		}
		return url, nil
	}
	t.Cleanup(func() { handleWebChatResolveConversation = orig })
}

func TestHandleWebChatResumeEmptyKeep(t *testing.T) {
	_, err := HandleWebChatResume(context.Background(), WebChatOptions{})
	if err == nil || !strings.Contains(err.Error(), "Keep") {
		t.Fatalf("err = %v, want Keep-required error", err)
	}
}

func TestHandleWebChatResumeReadFailure(t *testing.T) {
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/conv123")
	mockResumeRead(t, "", "no-idb", false)
	_, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "conv123"})
	if err == nil || !strings.Contains(err.Error(), "cannot read the conversation tail") {
		t.Fatalf("err = %v, want read-failure error", err)
	}
}

func TestHandleWebChatResumeTrailingUserMessageRejected(t *testing.T) {
	// The newest message is a USER message (e.g. typed in the user's own
	// browser, reply not landed yet): the last assistant message is NOT the
	// conversation tail, so resuming must refuse instead of executing an
	// older, possibly stale block.
	origExec := handleWebChatExecShell
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		t.Fatal("no block may execute when the newest message is a user message")
		return nil, nil
	}
	t.Cleanup(func() { handleWebChatExecShell = origExec })

	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convTAIL")
	mockResumeReadFull(t, shellBlockTest, "FINISHED", "USER", "", true)
	_, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convTAIL", Role: "dev"})
	if err == nil || !strings.Contains(err.Error(), "newest message") {
		t.Fatalf("err = %v, want newest-message refusal", err)
	}
}

func TestHandleWebChatResumeConfirmReadGate(t *testing.T) {
	// The conversation can be continued elsewhere between the initial read
	// and the pre-execution confirmation read. When that happens NOTHING
	// may execute and the caller retries against the new state.
	origRead := handleWebChatResumeRead
	origExec := handleWebChatExecShell
	t.Cleanup(func() {
		handleWebChatResumeRead = origRead
		handleWebChatExecShell = origExec
	})
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convRACE")

	t.Run("content changed", func(t *testing.T) {
		calls := 0
		handleWebChatResumeRead = func(_ context.Context, _ string) (string, string, string, string, bool) {
			calls++
			if calls == 1 {
				return shellBlockTest, "FINISHED", "ASSISTANT", "", true
			}
			return "the conversation moved on\n\nnew content", "FINISHED", "ASSISTANT", "", true
		}
		handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
			t.Fatal("a changed conversation must not execute the stale block")
			return nil, nil
		}
		_, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convRACE", Role: "dev"})
		if err == nil || !strings.Contains(err.Error(), "changed between the two reads") {
			t.Fatalf("err = %v, want changed-between-reads refusal", err)
		}
		if calls != 2 {
			t.Errorf("reads = %d, want 2 (initial + confirmation)", calls)
		}
	})

	t.Run("confirmation read failed", func(t *testing.T) {
		calls := 0
		handleWebChatResumeRead = func(_ context.Context, _ string) (string, string, string, string, bool) {
			calls++
			if calls == 1 {
				return shellBlockTest, "FINISHED", "ASSISTANT", "", true
			}
			return "", "", "", "no idb record", false
		}
		handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
			t.Fatal("an unconfirmable conversation must not execute the block")
			return nil, nil
		}
		_, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convRACE", Role: "dev"})
		if err == nil || !strings.Contains(err.Error(), "confirmation read failed") {
			t.Fatalf("err = %v, want confirmation-failure refusal", err)
		}
		if calls != 2 {
			t.Errorf("reads = %d, want 2 (initial + confirmation)", calls)
		}
	})

	t.Run("confirmation read shows a user tail", func(t *testing.T) {
		calls := 0
		handleWebChatResumeRead = func(_ context.Context, _ string) (string, string, string, string, bool) {
			calls++
			if calls == 1 {
				return shellBlockTest, "FINISHED", "ASSISTANT", "", true
			}
			return "waiting for a reply", "FINISHED", "USER", "", true
		}
		handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
			t.Fatal("a conversation whose tail turned into a user message must not execute")
			return nil, nil
		}
		_, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convRACE", Role: "dev"})
		if err == nil || !strings.Contains(err.Error(), "now a") {
			t.Fatalf("err = %v, want user-tail refusal", err)
		}
		if calls != 2 {
			t.Errorf("reads = %d, want 2 (initial + confirmation)", calls)
		}
	})
}

func TestHandleWebChatResumeMultiTurnReply(t *testing.T) {
	// The last message is a normal reply (multi-turn conversation, nothing
	// pending): the content is returned verbatim, nothing executes, nothing
	// is sent.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	reply := "This conversation already reached its conclusion. " + strings.Repeat("Nothing is pending anymore. ", 6)
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		t.Fatal("handleWebChatSend must not be called for a multi-turn resume")
		return WebChatResult{}, nil
	}

	origExec := handleWebChatExecShell
	executed := false
	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		executed = true
		return &shellblock.RunResult{}, nil
	}
	t.Cleanup(func() { handleWebChatExecShell = origExec })

	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/conv123")
	mockResumeRead(t, reply, "FINISHED", true)
	res, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "conv123", Role: "test"})
	if err != nil {
		t.Fatalf("HandleWebChatResume: %v", err)
	}
	if executed {
		t.Error("tool executor must not run for a multi-turn reply")
	}
	if res.Content != reply {
		t.Errorf("content = %q, want verbatim last reply", res.Content)
	}
	if res.Printed {
		t.Error("multi-turn reply must not be marked Printed (caller prints it)")
	}
}

// TestWebChatTransportOptionsClearsHandleFields guards the strip before
// every transport send: a new handle-level field that leaks into the
// transport makes WebChatWithOptions reject the whole send (the live smoke
// test caught exactly that for SkipPromptInjection).
func TestWebChatTransportOptionsClearsHandleFields(t *testing.T) {
	att := filepath.Join(t.TempDir(), "x.md")
	if err := os.WriteFile(att, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := WebChatOptions{
		Role:                "review",
		System:              "persona",
		SkipPromptInjection: true,
		Keep:                "abc",
		Attachments:         []string{att},
	}
	transport := webChatTransportOptions(opts)
	if transport.Role != "" || transport.System != "" || transport.SkipPromptInjection {
		t.Errorf("handle-only fields must be cleared: %+v", transport)
	}
	if transport.Keep != "abc" || len(transport.Attachments) != 1 {
		t.Errorf("transport fields must survive: %+v", transport)
	}
	if err := validateWebChatOptions(transport); err != nil {
		t.Errorf("stripped transport options must validate: %v", err)
	}
}

func TestWebChatWithOptionsRejectsHandleFields(t *testing.T) {
	// Role/System/SkipPromptInjection are HandleWebChat-only: the transport
	// must fail loudly instead of silently ignoring them (a caller using the
	// wrong entry point would get no role prompt and no tool loop).
	for _, tc := range []struct {
		name string
		opts WebChatOptions
	}{
		{"role", WebChatOptions{Role: "expert"}},
		{"system", WebChatOptions{System: "persona"}},
		{"both", WebChatOptions{Role: "expert", System: "persona"}},
		{"skip injection", WebChatOptions{SkipPromptInjection: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := WebChatWithOptions(context.Background(), "msg", tc.opts)
			if err == nil || !strings.Contains(err.Error(), "HandleWebChat") {
				t.Fatalf("err = %v, want HandleWebChat-only rejection", err)
			}
		})
	}
}

// TestHandleWebChatKeepSkipsPromptInjection verifies that a resumed web
// conversation (Keep != "") does not re-inject the persona: the sent message
// must equal the input verbatim. First-round cases (Keep == "") assert the
// contrast - the persona IS injected, detectable via the "## User Request"
// separator, with the user message preserved after it. Role "dev" relies on
// roles.DefaultFor("dev") falling back to the dev template, so the separator's
// presence/absence is the sufficient, stable signal of injection.
// SkipPromptInjection is the second suppression path: with it set, the first
// round sends the input verbatim even though Role is non-empty.
func TestHandleWebChatKeepSkipsPromptInjection(t *testing.T) {
	const message = "continue the discussion"

	tests := []struct {
		name         string
		keep         string
		role         string
		system       string
		skip         bool
		wantInjected bool // true = persona injected (first round); false = verbatim (resumed / SkipPromptInjection)
	}{
		{"first round role injected", "", "dev", "", false, true},
		{"first round system wins over role", "", "dev", "persona-x", false, true},
		{"first round skip suppresses role injection", "", "dev", "", true, false},
		{"first round skip suppresses system injection", "", "", "persona", true, false},
		{"resume role skipped", "conv123", "dev", "", false, false},
		{"resume system skipped", "conv123", "", "persona", false, false},
		{"resume last role skipped", "last", "dev", "", false, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := handleWebChatSend
			t.Cleanup(func() { handleWebChatSend = orig })

			var sent string
			handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
				sent = msg
				return WebChatResult{Content: strings.Repeat("ok. ", 40), URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
			}

			_, err := HandleWebChat(context.Background(), message, WebChatOptions{
				Keep:                tc.keep,
				Role:                tc.role,
				System:              tc.system,
				SkipPromptInjection: tc.skip,
			})
			if err != nil {
				t.Fatalf("HandleWebChat: %v", err)
			}

			if tc.wantInjected {
				if !strings.Contains(sent, "## User Request") {
					t.Errorf("first-round message lacks ## User Request separator (persona not injected), got %q", sent)
				}
				if !strings.HasSuffix(sent, message) {
					t.Errorf("first-round message must preserve the user message verbatim, got %q", sent)
				}
				if tc.system != "" && !strings.Contains(sent, tc.system) {
					t.Errorf("first-round message must contain the system persona %q (System wins over Role), got %q", tc.system, sent)
				}
			} else if sent != message {
				t.Errorf("resumed message must equal input verbatim, got %q", sent)
			}
		})
	}
}

func TestHandleWebChatRetriesOnBusy(t *testing.T) {
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })

	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		return WebChatResult{}, ErrServerBusy
	}
	handleWebChatRetryDelays = []time.Duration{0, 0, 0}

	_, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err == nil {
		t.Fatal("expected error for persistent server busy")
	}
	if !errors.Is(err, ErrServerBusy) {
		t.Errorf("err = %v, want ErrServerBusy chain", err)
	}
	// 1 initial attempt + 3 backoff retries.
	if calls != 4 {
		t.Errorf("handleWebChatSend calls = %d, want 4", calls)
	}
}

func TestHandleWebChatRetriesThenSuccess(t *testing.T) {
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })

	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		if calls <= 2 {
			return WebChatResult{}, ErrServerBusy
		}
		return WebChatResult{Content: strings.Repeat("expert answer. ", 10)}, nil
	}
	handleWebChatRetryDelays = []time.Duration{0, 0, 0}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != strings.Repeat("expert answer. ", 10) {
		t.Errorf("content = %q, want expert answer", res.Content)
	}
	if calls != 3 {
		t.Errorf("handleWebChatSend calls = %d, want 3", calls)
	}
}

func TestHandleWebChatNoRetryOnHardError(t *testing.T) {
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })

	hardErr := errors.New("login required")
	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		return WebChatResult{}, hardErr
	}

	_, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if !errors.Is(err, hardErr) {
		t.Errorf("err = %v, want hard error passthrough", err)
	}
	if calls != 1 {
		t.Errorf("handleWebChatSend calls = %d, want 1 (no retry on permanent error)", calls)
	}
}

func TestHandleWebChatRetriesOnTruncated(t *testing.T) {
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })

	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		return WebChatResult{}, ErrTruncated
	}
	handleWebChatRetryDelays = []time.Duration{0, 0, 0}

	_, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err == nil {
		t.Fatal("expected error for persistent truncation")
	}
	if !errors.Is(err, ErrTruncated) {
		t.Errorf("err = %v, want ErrTruncated chain", err)
	}
	// 1 initial attempt + 3 backoff retries.
	if calls != 4 {
		t.Errorf("handleWebChatSend calls = %d, want 4", calls)
	}
}

func TestHandleWebChatTruncatedThenSuccess(t *testing.T) {
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })

	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		if calls <= 2 {
			return WebChatResult{}, ErrTruncated
		}
		return WebChatResult{Content: strings.Repeat("expert answer. ", 10)}, nil
	}
	handleWebChatRetryDelays = []time.Duration{0, 0, 0}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != strings.Repeat("expert answer. ", 10) {
		t.Errorf("content = %q, want expert answer", res.Content)
	}
	if calls != 3 {
		t.Errorf("handleWebChatSend calls = %d, want 3", calls)
	}
}

func TestHandleWebChatRetryAbortsOnCancel(t *testing.T) {
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })

	calls := 0
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		return WebChatResult{}, ErrServerBusy
	}
	// Non-zero delay guarantees ctx.Done wins the select.
	handleWebChatRetryDelays = []time.Duration{time.Hour}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := HandleWebChat(ctx, "input", WebChatOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
	if calls != 1 {
		t.Errorf("handleWebChatSend calls = %d, want 1 (backoff aborted by cancel)", calls)
	}
}

func TestHandleWebChatOneShotNotPrinted(t *testing.T) {
	// 非工具循环的一次性散文回复不标记 Printed：调用方（CLI/ask_expert）
	// 负责打印 content，行为与之前一致。
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: strings.Repeat("plain prose answer. ", 10), URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Printed {
		t.Error("one-shot reply must not be marked Printed")
	}
	if res.Content != strings.Repeat("plain prose answer. ", 10) {
		t.Errorf("content = %q, want plain prose answer", res.Content)
	}
}

// TestHandleWebChatRetryErrorSinglePrefix pins the report's cosmetic defect:
// the exhausted-retry error must not stack two "webchat:" prefixes (the
// transport already wraps the underlying failure).
func TestHandleWebChatRetryErrorSinglePrefix(t *testing.T) {
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })
	handleWebChatRetryDelays = []time.Duration{0}

	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{}, fmt.Errorf("webchat: %w", ErrTruncated)
	}

	_, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err == nil {
		t.Fatal("expected error after retry exhaustion")
	}
	if got := strings.Count(err.Error(), "webchat:"); got != 1 {
		t.Errorf("error %q has %d webchat: prefixes, want 1", err, got)
	}
}

// TestHandleWebChatQuotedShellNotExecuted: a long answer that merely QUOTES a
// `<shell>` example (inside a fenced code block) must not enter the tool
// loop, must not execute anything, and must keep its prose in the returned
// content.
func TestHandleWebChatQuotedShellNotExecuted(t *testing.T) {
	origSend, origExec := handleWebChatSend, handleWebChatExecShell
	t.Cleanup(func() { handleWebChatSend, handleWebChatExecShell = origSend, origExec })

	handleWebChatExecShell = func(_ context.Context, _ string, _ string, _ time.Duration) (*shellblock.RunResult, error) {
		t.Fatal("a quoted example must not execute")
		return nil, nil
	}
	quoted := "The protocol shape is:\n```\n" + shellBlockTest + "\n```\nThat is what I would send if a script were needed, but this reply is just prose about it."
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: quoted, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Printed {
		t.Error("a quoted example never enters the loop, so Printed must be false")
	}
	if res.Content != quoted {
		t.Errorf("content = %q, want the reply verbatim", res.Content)
	}
}

// TestHandleWebChatSkipPromptInjectionKeepsContentVerbatim verifies that
// SkipPromptInjection only suppresses the persona injection: the message
// goes out verbatim and the reply comes back as-is (no post-processing of
// any markup the expert wrote).
func TestHandleWebChatSkipPromptInjectionKeepsContentVerbatim(t *testing.T) {
	orig := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = orig })

	var sent string
	reply := "The review is complete. " + strings.Repeat("No blocking issues were found. ", 5) +
		"It merely quotes <tool_calls> and <invoke> as text, not as commands."
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		sent = msg
		return WebChatResult{Content: reply, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review", SkipPromptInjection: true})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if sent != "input" {
		t.Errorf("message = %q, want the input verbatim (no prompt injection)", sent)
	}
	if res.Content != reply {
		t.Errorf("content = %q, want the reply verbatim (no post-processing)", res.Content)
	}
}

func TestWebChatSessionLifecycle(t *testing.T) {
	// Close before any send (nothing was booted) must be safe, and repeated
	// Close calls must be idempotent: HandleWebChat defers Close even when the
	// first send fails validation and the browser was never launched.
	s := newWebChatSession()
	s.Close()
	s.Close()

	if webChatSessionFrom(context.Background()) != nil {
		t.Error("session present in a plain context")
	}
}

func TestWebChatSessionCloseAfterBoot(t *testing.T) {
	// White-box: once booted (tabCtx/stop set), Close must invoke the stop
	// function exactly once, clear the boot state, and stay idempotent. The
	// real boot path needs Chrome, so the booted state is simulated.
	s := newWebChatSession()
	s.tabCtx = context.Background() // pretend the browser was booted
	stopped := 0
	s.stop = func() { stopped++ }

	s.Close()
	if stopped != 1 {
		t.Errorf("stop calls = %d, want 1", stopped)
	}
	if s.tabCtx != nil || s.stop != nil {
		t.Error("boot state not cleared after Close")
	}
	s.Close() // second Close must be a no-op
	if stopped != 1 {
		t.Errorf("stop calls after second Close = %d, want still 1", stopped)
	}
}

func TestHandleWebChatSharesBrowserSession(t *testing.T) {
	// One HandleWebChat call must reuse a SINGLE browser session across the
	// initial send, backoff retries and shell tool-loop follow-ups - not
	// launch a fresh Chrome per round (the dominant cost of a multi-tool
	// consultation).
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })
	handleWebChatRetryDelays = []time.Duration{0}

	const (
		convURL     = "https://chat.deepseek.com/a/chat/s/convSHARE"
		finalAnswer = "## Overall Assessment - solid implementation; no regressions found; tests pass; coverage adequate; release verdict: acceptable; risk items: none; recommendation: ship it."
	)
	var sessions []*webChatSession
	calls := 0
	handleWebChatSend = func(ctx context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		sessions = append(sessions, webChatSessionFrom(ctx))
		switch calls {
		case 1:
			return WebChatResult{}, ErrServerBusy // transient failure -> backoff retry
		case 2:
			return WebChatResult{Content: shellBlockTest, URL: convURL}, nil // block -> shell tool loop
		default:
			return WebChatResult{Content: finalAnswer, URL: convURL}, nil
		}
	}
	mockShellExecOK(t)

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(sessions) != 3 {
		t.Fatalf("send calls = %d, want 3 (initial + retry + tool follow-up)", calls)
	}
	for i, sess := range sessions {
		if sess == nil {
			t.Fatalf("send %d received no shared session", i+1)
		}
		if sess != sessions[0] {
			t.Errorf("send %d uses a different session than send 1", i+1)
		}
	}
}

func TestHandleWebChatResumeStreamingReplyRejected(t *testing.T) {
	// The last message is still streaming (status != FINISHED): surfacing it
	// as the resume result would hand the caller a half-written conclusion.
	// Refuse with a clear error instead of pretending it is final.
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convS")
	mockResumeRead(t, "Partial report, still writing. "+strings.Repeat("More will come. ", 8), "STREAMING", true)
	_, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convS", Role: "test"})
	if err == nil || !strings.Contains(err.Error(), "not finished") {
		t.Fatalf("err = %v, want not-finished error", err)
	}
}

func TestHandleWebChatResumeUnknownStatusAccepted(t *testing.T) {
	// status == "" (IDB does not always record a status): a plain reply is
	// still returned verbatim — blank status is not treated as streaming.
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convU")
	reply := "Final conclusion. " + strings.Repeat("Everything verified. ", 8)
	mockResumeRead(t, reply, "", true)
	res, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convU", Role: "test"})
	if err != nil {
		t.Fatalf("HandleWebChatResume: %v", err)
	}
	if res.Content != reply {
		t.Errorf("content = %q, want verbatim last reply", res.Content)
	}
}
