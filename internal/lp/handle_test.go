package lp

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/dsml"
	"github.com/dscli/dscli/internal/outfmt"
)

// interruptedRound is the exact last-message content of a real QA round that
// died mid tool-call: every <invoke> parses but the close tag is the typo'd
// "</_calls>" (the model dropped "tool"). This is the resume target.
const interruptedRound = `<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">grep -rn "func parseSince" internal/toolcall/ask/ && echo '===' && grep -rn "func truncateReviewRequest" internal/toolcall/ask/ && echo '===' && grep -rn "func AskExpertWithRole" internal/toolcall/ask/</parameter>
<parameter name="justification" string="true">Locate parseSince, truncateReviewRequest, AskExpertWithRole to inspect validation and reuse.</parameter>
</invoke>
<invoke name="shell">
<parameter name="script" string="true">ls internal/toolcall/ask/</parameter>
<parameter name="justification" string="true">List ask package files to find code_review and helpers.</parameter>
</invoke>
</tool_calls>`

// resumeReadReply returns a mock read for handleWebChatResumeRead and a
// cleanup that restores it. The content is the "last assistant message" the
// resumed conversation was interrupted on.
func mockResumeRead(t *testing.T, content, status string, ok bool) {
	t.Helper()
	orig := handleWebChatResumeRead
	handleWebChatResumeRead = func(_ context.Context, _ string) (string, string, bool) {
		return content, status, ok
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
	if err == nil || !strings.Contains(err.Error(), "cannot read last assistant message") {
		t.Fatalf("err = %v, want read-failure error", err)
	}
}

func TestHandleWebChatResumePendingToolCalls(t *testing.T) {
	// The last message is an interrupted tool-call round (broken close tag):
	// HandleWebChatResume must execute the pending calls locally and feed the
	// results back into the SAME conversation until the final answer.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	const (
		convURL     = "https://chat.deepseek.com/a/chat/s/convRESUME"
		finalAnswer = "## Quality Report\nAll checks pass..."
	)
	var messages []string
	var callsOpts []WebChatOptions
	handleWebChatSend = func(_ context.Context, msg string, opts WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		callsOpts = append(callsOpts, opts)
		return WebChatResult{Content: finalAnswer, URL: convURL}, nil
	}

	mockResumeResolve(t, convURL)
	mockResumeRead(t, interruptedRound, "FINISHED", true)
	seen := captureExecDSML(t, "<tool_result>grep output</tool_result>")

	res, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convRESUME", Role: "test"})
	if err != nil {
		t.Fatalf("HandleWebChatResume: %v", err)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if !res.Printed {
		t.Error("loop result must be marked Printed (final answer shown per round)")
	}
	// Both calls of the interrupted round executed.
	if len(*seen) != 2 {
		t.Fatalf("executed calls = %d, want 2 (both pending invokes)", len(*seen))
	}
	if (*seen)[0].Name != "shell" || (*seen)[1].Name != "shell" {
		t.Errorf("executed calls = %+v, want two shell", *seen)
	}
	// The follow-up must continue the SAME conversation with tool feedback.
	if len(messages) != 1 {
		t.Fatalf("sends = %d, want 1 (tool feedback only; nothing new asked)", len(messages))
	}
	if !strings.Contains(messages[0], "<tool_result>") {
		t.Errorf("feedback message = %q, want tool_result block", messages[0])
	}
	if callsOpts[0].Keep != convURL {
		t.Errorf("follow-up Keep = %q, want %s", callsOpts[0].Keep, convURL)
	}
	if callsOpts[0].Role != "" {
		t.Errorf("follow-up Role = %q, want empty (no role re-injection)", callsOpts[0].Role)
	}
}

func TestHandleWebChatResumeMultiTurnReply(t *testing.T) {
	// The last message is a normal reply (multi-turn conversation, nothing
	// pending): the content is returned verbatim, nothing executes, nothing
	// is sent.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	reply := "This conversation already reached its conclusion."
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		t.Fatal("handleWebChatSend must not be called for a multi-turn resume")
		return WebChatResult{}, nil
	}

	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return nil
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

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

func TestWebChatWithOptionsRejectsHandleFields(t *testing.T) {
	// Role/System are HandleWebChat-only: the transport must fail loudly
	// instead of silently ignoring them (a caller using the wrong entry
	// point would get no role prompt and no DSML loop).
	for _, tc := range []struct {
		name string
		opts WebChatOptions
	}{
		{"role", WebChatOptions{Role: "expert"}},
		{"system", WebChatOptions{System: "persona"}},
		{"both", WebChatOptions{Role: "expert", System: "persona"}},
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
func TestHandleWebChatKeepSkipsPromptInjection(t *testing.T) {
	const message = "continue the discussion"

	tests := []struct {
		name         string
		keep         string
		role         string
		system       string
		wantInjected bool // true = persona injected (first round), false = verbatim (resumed, Keep != "")
	}{
		{"first round role injected", "", "dev", "", true},
		{"first round system wins over role", "", "dev", "persona-x", true},
		{"resume role skipped", "conv123", "dev", "", false},
		{"resume system skipped", "conv123", "", "persona", false},
		{"resume last role skipped", "last", "dev", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			orig := handleWebChatSend
			t.Cleanup(func() { handleWebChatSend = orig })

			var sent string
			handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
				sent = msg
				return WebChatResult{Content: "ok", URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
			}

			_, err := HandleWebChat(context.Background(), message, WebChatOptions{
				Keep:   tc.keep,
				Role:   tc.role,
				System: tc.system,
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
		return WebChatResult{Content: "expert answer"}, nil
	}
	handleWebChatRetryDelays = []time.Duration{0, 0, 0}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != "expert answer" {
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
		return WebChatResult{Content: "expert answer"}, nil
	}
	handleWebChatRetryDelays = []time.Duration{0, 0, 0}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != "expert answer" {
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

// dsmlReply is a realistic DSML tool-call reply from a review expert.
// Observed shape: tool-call replies are wrapped (or end) with
// </tool_calls>, which IsDSMLToolCallEnd accepts (as does a prose
// preamble before the wrapper, tested in
// TestHandleWebChatToolLoopProsePreamble and
// TestHandleWebChatToolLoopLongPreambleExecutes).
const dsmlReply = `<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">git show --stat</parameter>
<parameter name="justification" string="true">See changed files</parameter>
<parameter name="timeout" string="false">10000</parameter>
</invoke>
</tool_calls>`

// captureExecDSML replaces handleWebChatExecDSML with a recorder and returns
// the feedback text to emit. The recorded calls are verified per test.
func captureExecDSML(t *testing.T, feedback string) *[]dsml.DSMLCall {
	t.Helper()
	orig := handleWebChatExecDSML
	var seen []dsml.DSMLCall
	handleWebChatExecDSML = func(_ context.Context, calls []dsml.DSMLCall) []string {
		seen = append(seen, calls...)
		return []string{feedback}
	}
	t.Cleanup(func() { handleWebChatExecDSML = orig })
	return &seen
}

func TestHandleWebChatToolLoop(t *testing.T) {
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	const (
		url1        = "https://chat.deepseek.com/a/chat/s/convAAA"
		url2        = "https://chat.deepseek.com/a/chat/s/convBBB"
		finalAnswer = "## Overall Assessment\nSolid implementation..."
	)
	var calls []WebChatOptions
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, opts WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		calls = append(calls, opts)
		switch len(messages) {
		case 1:
			return WebChatResult{Content: dsmlReply, URL: url1}, nil
		case 2:
			return WebChatResult{Content: finalAnswer, URL: url2}, nil
		}
		return WebChatResult{Content: "unexpected extra round", URL: url2}, nil
	}

	seen := captureExecDSML(t, "shell 工具调用的结果：\n```\nchanged files\n```")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if res.URL != url2 {
		t.Errorf("URL = %q, want %s", res.URL, url2)
	}
	if len(*seen) != 1 || (*seen)[0].Name != "shell" {
		t.Errorf("executed calls = %+v, want one shell", *seen)
	}
	// Round 2 must continue the SAME conversation (Keep = first URL) with
	// the tool feedback as message - and must NOT re-inject the role prompt
	// or re-upload attachments.
	if len(messages) != 2 {
		t.Fatalf("handleWebChatSend messages = %d, want 2 (initial + tool follow-up)", len(messages))
	}
	if !strings.Contains(messages[1], "工具调用的结果") || strings.Contains(messages[1], "Core Identity") {
		t.Errorf("round-2 message = %q, want tool feedback without role prompt", messages[1])
	}
	if calls[1].Keep != url1 {
		t.Errorf("round-2 Keep = %q, want %s", calls[1].Keep, url1)
	}
	if calls[1].Mode != "" {
		t.Errorf("round-2 Mode = %q, want empty (preserve conversation mode)", calls[1].Mode)
	}
	if calls[1].Role != "" {
		t.Errorf("round-2 Role = %q, want empty (no re-injection)", calls[1].Role)
	}
	if calls[1].Attachments != nil {
		t.Errorf("round-2 Attachments = %v, want nil (no re-upload)", calls[1].Attachments)
	}
}

func TestHandleWebChatToolLoopBareInvoke(t *testing.T) {
	// The developer model (code_dev round, 2026-08-29) replied with a BARE
	// <invoke> block - no <tool_calls> wrapper, no wrapper close - which the
	// End/Cut signals alone would not route into the loop. The reply is a
	// complete executable emission (IsDSMLToolCallReply via
	// IsPureDSMLToolCalls), so the loop must execute it and continue until a
	// final answer, exactly as for a wrapped call.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	bare := `<invoke name="read_file">
<parameter name="path" string="true">AGENTS.md</parameter>
</invoke>`
	finalAnswer := "## Overall Assessment\nEverything looks good."
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			return WebChatResult{Content: bare, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
		}
		return WebChatResult{Content: finalAnswer, URL: "https://chat.deepseek.com/a/chat/s/convY"}, nil
	}
	seen := captureExecDSML(t, "file read result")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "dev"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if len(*seen) != 1 || (*seen)[0].Name != "read_file" {
		t.Errorf("executed calls = %+v, want one read_file", *seen)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(messages) != 2 {
		t.Fatalf("sends = %d, want 2 (initial + tool feedback)", len(messages))
	}
	if messages[1] != "file read result" {
		t.Errorf("round-2 message = %q, want tool feedback", messages[1])
	}
}

func TestHandleWebChatToolLoopPlainChatExecutesDSML(t *testing.T) {
	// Plain webchat (role empty) also enters the tool loop when the reply
	// IS a tool call: chat.deepseek.com's model emits DSML natively, so
	// the judge (IsDSMLToolCallEnd) - not the role flag - decides
	// execution. A prose reply that merely cites an <invoke> example stays
	// verbatim (TestHandleWebChatToolLoopPlainChatProseReference).
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			return WebChatResult{Content: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
		}
		return WebChatResult{Content: "final answer", URL: "https://chat.deepseek.com/a/chat/s/convY"}, nil
	}
	seen := captureExecDSML(t, "tool feedback")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if len(*seen) == 0 {
		t.Error("tool executor not called for a tool-call reply in plain chat")
	}
	if res.Content != "final answer" {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(messages) != 2 {
		t.Fatalf("sends = %d, want 2 (initial + tool feedback)", len(messages))
	}
	if messages[1] != "tool feedback" {
		t.Errorf("round-2 message = %q, want tool feedback", messages[1])
	}
}

func TestHandleWebChatToolLoopProsePreamble(t *testing.T) {
	// A reply pairing a short prose preamble with a complete call block is
	// still a tool call (the model explaining it is re-sending its call):
	// the calls execute and the preamble is discarded with the round.
	preamble := "你说得对，我上一条消息的工具调用少了 `<tool_calls>` 包裹标签，格式不完整。不用你补齐，我重新发一遍正确的：\n\n" + dsmlReply

	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			return WebChatResult{Content: preamble, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
		}
		return WebChatResult{Content: "final answer", URL: "https://chat.deepseek.com/a/chat/s/convY"}, nil
	}
	seen := captureExecDSML(t, "tool feedback")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if len(*seen) == 0 {
		t.Error("tool executor not called for a tool-call reply with a prose preamble")
	}
	if res.Content != "final answer" {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(messages) != 2 || messages[1] != "tool feedback" {
		t.Errorf("round-2 message = %q, want tool feedback (preamble not forwarded)", messages[1])
	}
}

func TestHandleWebChatToolLoopPlainChatProseReference(t *testing.T) {
	// A plain-chat reply that merely quotes an <invoke> example inside long
	// prose is content, not a command: returned verbatim, never executed.
	longProse := "Solid work. `<invoke name=\"shell\"><parameter name=\"cmd\" string=\"true\">ls</parameter></invoke>` pins it. The implementation matches the design and the new tests cover every edge case from the review."
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: longProse, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return nil
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if executed {
		t.Error("tool executor called for a prose reply citing a quoted example; loop must not run")
	}
	if res.Content != longProse {
		t.Errorf("content = %q, want verbatim prose (no stripping in plain chat)", res.Content)
	}
}

func TestHandleWebChatToolLoopPlainChatNonRegistered(t *testing.T) {
	// Plain chat + short preamble + a complete non-registered call inside a
	// <tool_calls> wrapper passes the gate (IsDSMLToolCallEnd checks the
	// wrapper close, not the tool set) but produces no executable output:
	// the reply is returned verbatim - it is content, not a command - and
	// nothing is stripped.
	text := "我给你一个示例：\n" + `<tool_calls>
<invoke name="write_file">
<parameter name="path" string="true">a.txt</parameter>
</invoke>
</tool_calls>`
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: text, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return nil // non-registered call: native executor also returns nothing
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if !executed {
		t.Error("tool executor should be called (the gate passed), so the unregistered skip path is exercised")
	}
	if res.Content != text {
		t.Errorf("content = %q, want verbatim text (no stripping in plain chat)", res.Content)
	}
}

func TestHandleWebChatToolLoopPlainChatRound2Reference(t *testing.T) {
	// A round-2 plain-chat reply that merely quotes an <invoke> example
	// inside long prose exits the loop verbatim - plain chat never strips
	// the expert's words, unlike role consultations.
	round2 := "Solid work. `<invoke name=\"shell\"><parameter name=\"cmd\" string=\"true\">ls</parameter></invoke>` pins it. The implementation matches the design and the tests cover every edge case."
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	var sends int
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		sends++
		if sends == 1 {
			return WebChatResult{Content: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
		}
		return WebChatResult{Content: round2, URL: "https://chat.deepseek.com/a/chat/s/convY"}, nil
	}
	captureExecDSML(t, "tool feedback")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != round2 {
		t.Errorf("content = %q, want verbatim round-2 prose (no stripping in plain chat)", res.Content)
	}
}

func TestHandleWebChatToolLoopLongPreambleExecutes(t *testing.T) {
	// 需求（2026-08）：放弃"Content 中只有 <tool_calls>"的约定——只要
	// Content 以 </tool_calls> 结束就解析并执行其中的 tool_calls，即使
	// 前面有很长一段散文（模型解释它要做什么）；前言随该轮丢弃，不回填。
	longPreamble := strings.Repeat("我来说明一下为什么要先读这两个文件：", 5) + "\n\n" + dsmlReply

	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			return WebChatResult{Content: longPreamble, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
		}
		return WebChatResult{Content: "final answer", URL: "https://chat.deepseek.com/a/chat/s/convY"}, nil
	}
	seen := captureExecDSML(t, "tool feedback")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if len(*seen) == 0 {
		t.Error("tool executor not called for a long-preamble reply ending in </tool_calls>")
	}
	if res.Content != "final answer" {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(messages) != 2 || messages[1] != "tool feedback" {
		t.Errorf("round-2 message = %q, want tool feedback (preamble not forwarded)", messages[1])
	}
	if !res.Printed {
		t.Error("loop result must be marked Printed (final answer shown per round)")
	}
}

func TestHandleWebChatToolLoopPrintsRounds(t *testing.T) {
	// 工具循环每一轮（reasoning + content）都通过 outfmt.PrintContent
	// 打印（头部按角色显示，不打印 token 计数）；结果标记 Printed 让
	// 调用方不要再打印一次。
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	var buf bytes.Buffer
	outfmt.SetOutputWriter(&buf)
	t.Cleanup(func() { outfmt.SetOutputWriter(os.Stdout) })

	var sends int
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		sends++
		switch sends {
		case 1:
			return WebChatResult{Content: dsmlReply, Reasoning: "先看看改动", URL: "https://chat.deepseek.com/a/chat/s/convX", OutputTokens: 321}, nil
		default:
			return WebChatResult{Content: "final answer", Reasoning: "总结完成", URL: "https://chat.deepseek.com/a/chat/s/convY", OutputTokens: 456}, nil
		}
	}
	captureExecDSML(t, "tool feedback")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if !res.Printed {
		t.Error("loop result must be marked Printed")
	}
	out := buf.String()
	// 每一轮的 reasoning 与 content 都打印（首轮 + 末轮）；头部按角色
	// 显示（review·代码审查），不再打印 T: 计数。
	for _, want := range []string{"先看看改动", "总结完成", "final answer", "review·代码审查"} {
		if !strings.Contains(out, want) {
			t.Errorf("printed output missing %q:\n%s", want, out)
		}
	}
	for _, notWant := range []string{"T:321", "T:456"} {
		if strings.Contains(out, notWant) {
			t.Errorf("printed output should not contain %q:\n%s", notWant, out)
		}
	}
}

func TestHandleWebChatOneShotNotPrinted(t *testing.T) {
	// 非工具循环的一次性散文回复不标记 Printed：调用方（CLI/ask_expert）
	// 负责打印 content，行为与之前一致。
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: "plain prose answer", URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Printed {
		t.Error("one-shot reply must not be marked Printed")
	}
	if res.Content != "plain prose answer" {
		t.Errorf("content = %q, want plain prose answer", res.Content)
	}
}

func TestHandleWebChatToolLoopEmptyWrapperExecutesNothing(t *testing.T) {
	// gate 通过（以 </tool_calls> 结尾）但包裹内没有任何可解析的 <invoke>
	//（例如模型只是引用了语法、或只有一个裸闭合标签）→ 解析器返回 0 个
	// 调用 → 循环退出且不执行任何工具；结果标记 Printed（已在循环内打印）。
	text := "例如 <tool_calls> 就是工具调用的包裹标签，注意别漏掉结尾：\n<tool_calls>\n</tool_calls>"
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: text, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return nil
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if executed {
		t.Error("empty tool_calls wrapper must not execute anything")
	}
	if !res.Printed {
		t.Error("loop exit must mark the result Printed (it was printed inside the loop)")
	}
	if res.Content != text {
		t.Errorf("content = %q, want verbatim text (plain chat keeps the expert's words)", res.Content)
	}
}

func TestHandleWebChatToolLoopSlashlessProseOnly(t *testing.T) {
	// 回复以开口拼写的 <tool_calls>（无斜杠）结尾但没有可解析 <invoke>：
	// 门（IsDSMLToolCallReply）因 2026-08 "重复的开头" 退化而放行进循环，
	// 解析得 0 调用 → 循环退出且不执行任何工具。角色会话也必须把内容
	// 原样返回——散文里字面的 "<tool_calls>" 提及是模型的话，不是标记，
	// StripDSMLToolCalls 的全局 ReplaceAll 不得误删它。
	text := "记得用 <tool_calls> 包裹你的工具调用哦\n<tool_calls>"
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: text, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return nil
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if executed {
		t.Error("prose-only reply must not execute any tool")
	}
	if !res.Printed {
		t.Error("loop exit must mark the result Printed (it was printed inside the loop)")
	}
	if res.Content != text {
		t.Errorf("content = %q, want verbatim text (prose mention of <tool_calls> must survive)", res.Content)
	}
}

func TestHandleWebChatToolLoopEmptyWrapperRoleSession(t *testing.T) {
	// 0 调用轮次的角色会话：空 wrapper（以 </tool_calls> 结尾但内部没有任何
	// 可解析 <invoke>）也原样返回（verbatim），与"散文提及 <tool_calls>"
	// 无法可靠区分——两者都只是字面标签，没有可解析的调用；误删散文提及
	// 比保留一个空 wrapper 更糟，所以 0 调用轮次一律 verbatim（见 cleanExit
	// 注释）。plain-chat 变体见 TestHandleWebChatToolLoopEmptyWrapperExecutesNothing；
	// 本测试锁定 role 会话的同一行为。
	text := "例如 <tool_calls> 就是工具调用的包裹标签，注意别漏掉结尾：\n<tool_calls>\n</tool_calls>"
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: text, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return nil
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if executed {
		t.Error("empty tool_calls wrapper must not execute anything")
	}
	if !res.Printed {
		t.Error("loop exit must mark the result Printed (it was printed inside the loop)")
	}
	if res.Content != text {
		t.Errorf("content = %q, want verbatim text (zero-call round never strips)", res.Content)
	}
}

func TestHandleWebChatToolLoopRoundCap(t *testing.T) {
	origFunc, origRounds := handleWebChatSend, handleWebChatMaxDSMLRounds
	t.Cleanup(func() { handleWebChatSend, handleWebChatMaxDSMLRounds = origFunc, origRounds })
	handleWebChatMaxDSMLRounds = 2

	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		// The expert keeps requesting tools: every round returns DSML.
		return WebChatResult{Content: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	captureExecDSML(t, "tool output")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if strings.Contains(res.Content, "<invoke") || strings.Contains(res.Content, "<tool_calls") {
		t.Errorf("content still contains DSML markers after cap:\n%s", res.Content)
	}
	if res.Content != "" {
		t.Errorf("content = %q, want empty (pure DSML stripped)", res.Content)
	}
}

func TestHandleWebChatToolLoopTruncatedDSML(t *testing.T) {
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	// The first reply begins a tool call but is cut off before </invoke>.
	cut := `<tool_calls>
<invoke name="shell">
<parameter name="script" string="true">git show`
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: cut, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return nil
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if executed {
		t.Error("truncated tool call must not be executed")
	}
	if strings.Contains(res.Content, "<invoke") {
		t.Errorf("content still contains truncated DSML:\n%s", res.Content)
	}
}

func TestHandleWebChatToolLoopContinueFails(t *testing.T) {
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	calls := 0
	var lastErr error
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		calls++
		if calls == 1 {
			return WebChatResult{Content: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
		}
		lastErr = errors.New("browser crashed")
		return WebChatResult{}, lastErr
	}
	captureExecDSML(t, "tool output")

	_, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err == nil || !strings.Contains(err.Error(), "continue conversation") {
		t.Fatalf("err = %v, want continue-conversation error", err)
	}
	if calls != 2 {
		t.Errorf("handleWebChatSend calls = %d, want 2 (follow-up attempted once)", calls)
	}
}

// TestHandleWebChatQuotedDSMLNotExecuted: a long answer that merely QUOTES an
// <invoke> example (a review citing the DSML test corpus, either inline or
// inside a fenced code block) must not enter the tool loop, must not produce
// "unsupported tool" feedback, and must keep its prose in the returned
// content.
func TestHandleWebChatQuotedDSMLNotExecuted(t *testing.T) {
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	executed := false
	origExec := handleWebChatExecDSML
	handleWebChatExecDSML = func(_ context.Context, _ []dsml.DSMLCall) []string {
		executed = true
		return []string{`<tool_result>{"error":"unsupported tool ..."}</tool_result>`}
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	quoted := "## Overall\nSolid work.\n\nExample: `<invoke name=\"a\"><parameter name=\"cmd\" string=\"true\">x</parameter></invoke>` pins the behavior.\n```\n<invoke name=\"shell\"><parameter name=\"cmd\" string=\"true\">ls</parameter></invoke>\n```\nEnd."
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: quoted, URL: "https://chat.deepseek.com/a/chat/s/convQ"}, nil
	}

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if executed {
		t.Error("quoted DSML must not be executed")
	}
	if strings.Contains(res.Content, "<tool_result") {
		t.Errorf("content contains tool feedback noise:\n%s", res.Content)
	}
	if !strings.Contains(res.Content, "Solid work") || !strings.Contains(res.Content, "End.") {
		t.Errorf("content lost prose: %q", res.Content)
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
	// initial send, backoff retries and DSML tool-loop follow-ups - not
	// launch a fresh Chrome per round (the dominant cost of a multi-tool
	// consultation).
	origFunc, origDelays := handleWebChatSend, handleWebChatRetryDelays
	t.Cleanup(func() { handleWebChatSend, handleWebChatRetryDelays = origFunc, origDelays })
	handleWebChatRetryDelays = []time.Duration{0}

	const (
		convURL     = "https://chat.deepseek.com/a/chat/s/convSHARE"
		finalAnswer = "## Overall Assessment\nSolid implementation..."
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
			return WebChatResult{Content: dsmlReply, URL: convURL}, nil // DSML -> tool loop
		default:
			return WebChatResult{Content: finalAnswer, URL: convURL}, nil
		}
	}
	captureExecDSML(t, "tool output")

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

// cutRound is the shape of a real review round whose content ended with a
// CUT-OFF wrapper close tag (">" never came): every <invoke> is complete,
// the tail is "</" — the IsDSMLToolCallCut gate must route it into the loop.
const cutRound = `<tool_calls>
<invoke name="read_file">
<parameter name="path" string="true">AGENTS.md</parameter>
<parameter name="justification" string="true">Read project conventions before reviewing</parameter>
</invoke>
<invoke name="shell">
<parameter name="script" string="true">git show --stat HEAD</parameter>
<parameter name="justification" string="true">See full commit stats and files changed</parameter>
</invoke>
</tool_calls>`

func TestHandleWebChatToolLoopCutCloseTagExecutes(t *testing.T) {
	// A live reply whose wrapper close tag was cut off before ">": the gate
	// IsDSMLToolCallCut opens, the calls parse (all invokes complete), and
	// the execute-feedback loop runs — never returned verbatim.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	const finalAnswer = "## Overall Assessment\nSolid implementation..."
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			return WebChatResult{Content: cutRound, URL: "https://chat.deepseek.com/a/chat/s/convCUT"}, nil
		}
		return WebChatResult{Content: finalAnswer, URL: "https://chat.deepseek.com/a/chat/s/convF"}, nil
	}
	seen := captureExecDSML(t, "<tool_result>contents</tool_result>")

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if len(*seen) != 2 {
		t.Fatalf("executed calls = %d, want 2 (both invokes complete)", len(*seen))
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(messages) != 2 {
		t.Fatalf("sends = %d, want 2 (initial cut round + feedback)", len(messages))
	}
}

func TestHandleWebChatResumeCutCloseTag(t *testing.T) {
	// Resume with a cut close tag: the last assistant message is a pending
	// tool-call round with a truncated wrapper — must execute, not return
	// the markup verbatim.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	const finalAnswer = "## Quality Report\nAll checks pass..."
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		return WebChatResult{Content: finalAnswer, URL: "https://chat.deepseek.com/a/chat/s/convRCUT"}, nil
	}
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convRCUT")
	mockResumeRead(t, cutRound, "FINISHED", true)
	seen := captureExecDSML(t, "<tool_result>resumed</tool_result>")

	res, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convRCUT", Role: "test"})
	if err != nil {
		t.Fatalf("HandleWebChatResume: %v", err)
	}
	if len(*seen) != 2 {
		t.Fatalf("executed calls = %d, want 2", len(*seen))
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(messages) != 1 {
		t.Fatalf("sends = %d, want 1 (feedback only)", len(messages))
	}
}

func TestHandleWebChatResumeStreamingReplyRejected(t *testing.T) {
	// The last message is still streaming (status != FINISHED): surfacing it
	// as the resume result would hand the caller a half-written conclusion.
	// Refuse with a clear error instead of pretending it is final.
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convS")
	mockResumeRead(t, "Partial report...", "STREAMING", true)
	_, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convS", Role: "test"})
	if err == nil || !strings.Contains(err.Error(), "not finished") {
		t.Fatalf("err = %v, want not-finished error", err)
	}
}

func TestHandleWebChatResumeUnknownStatusAccepted(t *testing.T) {
	// status == "" (IDB does not always record a status): a plain reply is
	// still returned verbatim — blank status is not treated as streaming.
	mockResumeResolve(t, "https://chat.deepseek.com/a/chat/s/convU")
	mockResumeRead(t, "Final conclusion", "", true)
	res, err := HandleWebChatResume(context.Background(), WebChatOptions{Keep: "convU", Role: "test"})
	if err != nil {
		t.Fatalf("HandleWebChatResume: %v", err)
	}
	if res.Content != "Final conclusion" {
		t.Errorf("content = %q, want verbatim last reply", res.Content)
	}
}

func TestHandleWebChatToolLoopViolationExecutesWithWarning(t *testing.T) {
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	const finalAnswer = "Done."
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			// First send receives the tool-call reply: dsmlReply carries the
			// decorative justification parameter - parseable, but a format
			// violation (message.OK=false).
			return WebChatResult{Content: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
		}
		return WebChatResult{Content: finalAnswer, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}

	res := WebChatResult{Content: "unset"}
	seen := captureExecDSML(t, toolResultOK)
	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(*seen) != 1 || (*seen)[0].Name != "shell" {
		t.Fatalf("executed = %+v, want one shell (violation must NOT block execution)", *seen)
	}
	if len(messages) != 2 {
		t.Fatalf("sends = %d, want 2 (initial + tool feedback)", len(messages))
	}
	if !strings.Contains(messages[1], "ok") || !strings.Contains(messages[1], dsml.StrictWarning) {
		t.Errorf("tool feedback must carry the result AND StrictWarning:\n%s", messages[1])
	}
}

func TestHandleWebChatToolLoopReissueThenSuccess(t *testing.T) {
	// A reply that tries to emit a tool call but cannot be parsed (cut off)
	// re-issues the ReissueWarning and keeps the SAME conversation alive;
	// the model's corrected re-send then executes normally.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	cut := "<" + "tool_calls>\n<" + "invoke name=\"shell\">\n<" + "parameter name=\"script\" string=\"true\">git show"
	const finalAnswer = "All good."
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		switch len(messages) {
		case 1:
			return WebChatResult{Content: cut, URL: "https://chat.deepseek.com/a/chat/s/convR1"}, nil
		case 2:
			return WebChatResult{Content: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convR2"}, nil
		default:
			return WebChatResult{Content: finalAnswer, URL: "https://chat.deepseek.com/a/chat/s/convR3"}, nil
		}
	}
	seen := captureExecDSML(t, toolResultOK)

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(messages) != 3 {
		t.Fatalf("sends = %d, want 3 (re-issue warning, tool feedback, final)", len(messages))
	}
	if messages[1] != dsml.ReissueWarning {
		t.Errorf("second send = %q, want ReissueWarning", messages[1])
	}
	if len(*seen) != 1 || (*seen)[0].Name != "shell" {
		t.Errorf("executed = %+v, want one shell after re-issue", *seen)
	}
}

// toolResultOK is a well-formed tool_result block, built at runtime so this
// file stays transportable through DSML tool calls (literal angle brackets
// in a write_file content would be misread as markup and truncate the
// payload).
const toolResultOK = "<" + "tool_result>" + `{"result":"ok"}` + "<" + "/tool_result>"

func TestHandleWebChatToolLoopAllRejectedStillFeedsBack(t *testing.T) {
	// A round whose calls all fail conversion (e.g. the destructive-command
	// interception) must STILL route into the execution path so the model
	// receives the rejection feedback - msg.ToolCalls is empty for such a
	// round (conversion failures never enter ToolCalls), so the loop must
	// not judge it by ToolCalls alone.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	const finalAnswer = "Adapted."
	// A DESTRUCTIVE command call: it parses (raw calls non-empty) but its
	// conversion is rejected by normalizeDSMLInvoke (dsmlBlockedCmdRe), so
	// msg.ToolCalls stays EMPTY while the raw parse still has the call. Only
	// this shape exercises the regression: the old len(msg.ToolCalls)==0
	// routing would exit without feedback.
	destructive := "<" + "tool_calls><" + "invoke name=\"shell\">" +
		"<" + "parameter name=\"script\" string=\"true\">rm -rf /<" + "/parameter>" +
		"<" + "/invoke><" + "/tool_calls>"
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			return WebChatResult{Content: destructive, URL: "https://chat.deepseek.com/a/chat/s/convA"}, nil
		}
		return WebChatResult{Content: finalAnswer, URL: "https://chat.deepseek.com/a/chat/s/convA"}, nil
	}

	seen := captureExecDSML(t, rejectedBlock)
	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(*seen) != 1 {
		t.Fatalf("exec = %+v, want the parsed calls handed to the executor", *seen)
	}
	if len(messages) != 2 {
		t.Fatalf("sends = %d, want 2 (rejection feedback + final)", len(messages))
	}
	if !strings.Contains(messages[1], "rejected") {
		t.Errorf("rejection feedback must reach the model:\n%s", messages[1])
	}
}

func TestHandleWebChatToolLoopReasoningDraftExecutes(t *testing.T) {
	// Calls drafted in the REASONING (content carries no call) are the
	// fallback execution source: the loop must use the reasoning text as
	// the admission source, not the content.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })

	const finalAnswer = "Checked."
	var messages []string
	handleWebChatSend = func(_ context.Context, msg string, _ WebChatOptions) (WebChatResult, error) {
		messages = append(messages, msg)
		if len(messages) == 1 {
			return WebChatResult{Content: "Let me check the repo.", Reasoning: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convR"}, nil
		}
		return WebChatResult{Content: finalAnswer, URL: "https://chat.deepseek.com/a/chat/s/convR"}, nil
	}
	seen := captureExecDSML(t, toolResultOK)

	res, err := HandleWebChat(context.Background(), "input", WebChatOptions{Role: "review"})
	if err != nil {
		t.Fatalf("HandleWebChat: %v", err)
	}
	if res.Content != finalAnswer {
		t.Errorf("content = %q, want final answer", res.Content)
	}
	if len(*seen) != 1 || (*seen)[0].Name != "shell" {
		t.Fatalf("executed = %+v, want the reasoning-drafted shell call", *seen)
	}
	if len(messages) != 2 {
		t.Fatalf("sends = %d, want 2 (tool feedback + final)", len(messages))
	}
}

// rejectedBlock is a tool_result carrying a rejection error, built at runtime
// so this file stays transportable through DSML tool calls.
const rejectedBlock = "<" + "tool_result>" + `{"error":"destructive command rejected"}` + "<" + "/tool_result>"
