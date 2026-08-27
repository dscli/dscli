package lp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/toolcall"
)

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
// Observed shape: tool-call replies are pure DSML (no leading prose), which
// IsDSMLToolCallReply accepts (as does a short prose preamble, tested in
// TestHandleWebChatToolLoopProsePreamble).
const dsmlReply = `<tool_calls>
<invoke name="exec_command">
<parameter name="cmd" string="true">git show --stat</parameter>
<parameter name="justification" string="true">See changed files</parameter>
<parameter name="timeout" string="false">10000</parameter>
</invoke>
</tool_calls>`

// captureExecDSML replaces handleWebChatExecDSML with a recorder and returns
// the feedback text to emit. The recorded calls are verified per test.
func captureExecDSML(t *testing.T, feedback string) *[]toolcall.DSMLCall {
	t.Helper()
	orig := handleWebChatExecDSML
	var seen []toolcall.DSMLCall
	handleWebChatExecDSML = func(_ context.Context, calls []toolcall.DSMLCall) []string {
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

	seen := captureExecDSML(t, "exec_command 工具调用的结果：\n```\nchanged files\n```")

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
	if len(*seen) != 1 || (*seen)[0].Name != "exec_command" {
		t.Errorf("executed calls = %+v, want one exec_command", *seen)
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

func TestHandleWebChatToolLoopPlainChatExecutesDSML(t *testing.T) {
	// Plain webchat (role empty) also enters the tool loop when the reply
	// IS a tool call: chat.deepseek.com's model emits DSML natively, so
	// the judge (IsDSMLToolCallReply) - not the role flag - decides
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
	longProse := "Solid work. `<invoke name=\"exec_command\"><parameter name=\"cmd\" string=\"true\">ls</parameter></invoke>` pins it. The implementation matches the design and the new tests cover every edge case from the review."
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: longProse, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []toolcall.DSMLCall) []string {
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
<invoke name="exec_command">
<parameter name="cmd" string="true">git show`
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: cut, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
	}
	origExec := handleWebChatExecDSML
	executed := false
	handleWebChatExecDSML = func(_ context.Context, _ []toolcall.DSMLCall) []string {
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
	handleWebChatExecDSML = func(_ context.Context, _ []toolcall.DSMLCall) []string {
		executed = true
		return []string{`<tool_result>{"error":"unsupported tool ..."}</tool_result>`}
	}
	t.Cleanup(func() { handleWebChatExecDSML = origExec })

	quoted := "## Overall\nSolid work.\n\nExample: `<invoke name=\"a\"><parameter name=\"cmd\" string=\"true\">x</parameter></invoke>` pins the behavior.\n```\n<invoke name=\"exec_command\"><parameter name=\"cmd\" string=\"true\">ls</parameter></invoke>\n```\nEnd."
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
