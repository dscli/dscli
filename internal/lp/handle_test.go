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
const dsmlReply = `I need to inspect the amap implementation.
<tool_calls>
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

func TestHandleWebChatToolLoopNoRole(t *testing.T) {
	// Plain webchat (role empty) must never enter the tool loop: the DSML
	// text is returned verbatim, treating the expert's own words as content
	// rather than executing commands.
	origFunc := handleWebChatSend
	t.Cleanup(func() { handleWebChatSend = origFunc })
	handleWebChatSend = func(_ context.Context, _ string, _ WebChatOptions) (WebChatResult, error) {
		return WebChatResult{Content: dsmlReply, URL: "https://chat.deepseek.com/a/chat/s/convX"}, nil
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
		t.Error("tool executor called for role-less consultation; loop must not run")
	}
	if res.Content != dsmlReply {
		t.Errorf("content = %q, want verbatim DSML text", res.Content)
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
	if !strings.Contains(res.Content, "I need to inspect") {
		t.Errorf("content lost the prose part:\n%s", res.Content)
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
