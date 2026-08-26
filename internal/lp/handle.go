package lp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

// handleWebChatRetryDelays is the backoff sequence between retry attempts for
// transient server overload and truncation (ErrServerBusy / ErrSendRejected /
// ErrTruncated). The official DeepSeek error docs recommend retrying 429/500/
// 503 after a brief wait; each retry starts a fresh conversation, so a
// duplicate send has no side effects. A package variable so tests can shorten
// it.
var handleWebChatRetryDelays = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

// handleWebChatSend is the transport used by HandleWebChat; tests replace it
// with a mock to skip browser automation.
var handleWebChatSend = WebChatWithOptions

// handleWebChatMaxDSMLRounds caps the tool-call rounds within one web chat
// consultation. The loop naturally exits whenever a reply carries no tool
// calls - the expert always finishes with prose - so this is only a failsafe
// against a broken DSML parser (e.g. a reply that keeps matching an unclosed
// <invoke> block). 6 was too small: real sessions can run many rounds (9
// consecutive tool calls were observed in the design case study), so the cap
// is generous. A variable so tests can shrink it.
var handleWebChatMaxDSMLRounds = 1024

// handleWebChatExecDSML is the DSML executor hook; tests replace it with a
// recording mock (the real executor runs shells and needs no browser).
var handleWebChatExecDSML = toolcall.ExecuteDSMLToolCalls

// HandleWebChat sends message to chat.deepseek.com and returns the assistant
// reply, retrying transient failures and - for role-driven consultations -
// executing DSML tool calls the expert embeds in its reply.
//
// It is the high-level entry point shared by the ask_expert tool and the
// webchat CLI command: low-level transport (WebChatWithOptions) performs
// exactly one send, while HandleWebChat adds:
//
//   - Role/system prompt rendering: when opts.Role is non-empty, the
//     role-specific prompt template (see prompt.RenderPromptForRole) is
//     prepended; opts.System (raw text) takes precedence over Role. Neither
//     is injected when both are empty - the message is sent verbatim.
//   - Backoff retry on transient server overload and truncation
//     (ErrServerBusy / ErrSendRejected / ErrTruncated). Permanent errors
//     (login, bad arguments) fail immediately - retrying them is pointless.
//   - DSML tool loop: a role-driven reply (Role non-empty) that embeds DSML
//     tool calls (<invoke name="...">) is parsed, the underlying dscli tools
//     executed locally, and the results fed back into the SAME conversation
//     until the expert produces a final answer. Plain webchat (Role empty)
//     never enters the loop, matching the "Web 版不支持函数调用" caveat.
//   - Shared browser session: every send of one call (initial, backoff
//     retries, tool follow-ups) reuses a single browser, booted lazily on the
//     first send and closed when the call returns - a 9-tool-call
//     consultation launches Chrome once instead of ten times.
//
// A retried send targets the same conversation (Keep is preserved): the busy
// server never acknowledged the send, so re-sending is harmless.
func HandleWebChat(ctx context.Context, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleWebChat")
	defer span.Finish()

	// WebChat has no system prompt concept, so persona text is prepended to
	// the user message. The separator helps the web model distinguish the
	// instructions from the actual task. System wins over Role, matching the
	// ask layer's previous precedence.
	fullMessage := message
	if opts.System != "" {
		fullMessage = opts.System + "\n\n---\n\n## User Request\n\n" + message
	} else if opts.Role != "" {
		fullMessage = prompt.RenderPromptForRole(ctx, opts.Role) + "\n\n---\n\n## User Request\n\n" + message
	}

	// One browser session serves the whole consultation: every send (backoff
	// retries and tool-loop follow-ups) reuses the same tab, booted lazily on
	// the first send, and the browser is closed once when the call returns.
	sess := newWebChatSession()
	defer sess.Close()
	ctx = withWebChatSession(ctx, sess)

	var lastErr error
	for attempt := 0; attempt <= len(handleWebChatRetryDelays); attempt++ {
		if attempt > 0 {
			delay := handleWebChatRetryDelays[attempt-1]
			reason := "服务器繁忙"
			if errors.Is(lastErr, ErrTruncated) {
				reason = "输出被截断"
			}
			fmt.Fprintf(os.Stderr, "🔄 %s，%.0fs 后重试 (attempt %d/%d)...\n",
				reason, delay.Seconds(), attempt+1, len(handleWebChatRetryDelays)+1)

			select {
			case <-ctx.Done():
				return WebChatResult{}, ctx.Err()
			case <-time.After(delay):
			}
		}

		// Role/System are handle-level concerns; the transport rejects them
		// (see WebChatWithOptions), so strip them before the send.
		transport := opts
		transport.Role = ""
		transport.System = ""
		res, callErr := handleWebChatSend(ctx, fullMessage, transport)
		if callErr == nil {
			// Role-driven consultations (code_review's review role) may
			// receive DSML tool calls embedded in the reply: the web expert
			// cannot execute them locally, so we parse, run the underlying
			// dscli tools, and feed the results back into the SAME
			// conversation until the expert produces a final answer.
			// Plain webchat (Role empty) is unaffected.
			//
			// Only a PURE tool-call reply enters the loop (DeepSeek web
			// emits tool calls as pure DSML); a long answer that merely
			// quotes an <invoke> example must not be executed - the DSML
			// is stripped so callers see clean prose.
			if opts.Role != "" && toolcall.IsPureDSMLToolCalls(res.Content) {
				return handleWebChatToolLoop(ctx, res, opts)
			}
			if opts.Role != "" && toolcall.HasDSMLToolCalls(res.Content) {
				res.Content = toolcall.StripDSMLToolCalls(res.Content)
			}
			return res, nil
		}
		lastErr = callErr
		if !errors.Is(callErr, ErrServerBusy) && !errors.Is(callErr, ErrSendRejected) && !errors.Is(callErr, ErrTruncated) {
			return WebChatResult{}, callErr
		}
	}
	return WebChatResult{}, fmt.Errorf("webchat: %w (after %d attempts)", lastErr, len(handleWebChatRetryDelays)+1)
}

// handleWebChatToolLoop continues a role-driven WebChat conversation while the
// expert emits DSML tool calls: parse the calls, execute them locally, and
// post the results back into the SAME conversation (Keep=first URL). The
// role prompt is injected only on the first round - HandleWebChat already
// rendered it - so follow-up messages carry tool results verbatim.
//
// Exits with the last assistant reply when:
//   - the reply contains no tool calls (final answer), or
//   - handleWebChatMaxDSMLRounds is exhausted or a tool-call block is
//     truncated; the DSML is stripped so callers see only prose, plus a
//     warning on stderr.
//
// Browser/network errors are fatal: retrying mid-conversation is not safe.
func handleWebChatToolLoop(ctx context.Context, first WebChatResult, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleWebChatToolLoop")
	defer span.Finish()

	message := first.Content
	convURL := first.URL
	for round := 1; round <= handleWebChatMaxDSMLRounds; round++ {
		// Only pure tool-call replies continue the loop (see
		// IsPureDSMLToolCalls): a reply that mixes prose with DSML - the
		// expert finished its reasoning, or is quoting an example - ends
		// the loop with the DSML stripped.
		calls, parseErr := toolcall.ParseDSMLToolCalls(message)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️ 工具调用不完整，已停止循环: %v\n", parseErr)
			return WebChatResult{Content: toolcall.StripDSMLToolCalls(message), URL: convURL}, nil
		}
		if len(calls) == 0 {
			return WebChatResult{Content: message, URL: convURL}, nil
		}
		if !toolcall.IsPureDSMLToolCalls(message) {
			return WebChatResult{Content: toolcall.StripDSMLToolCalls(message), URL: convURL}, nil
		}

		fmt.Fprintf(os.Stderr, "🤖 专家请求执行 %d 个工具调用（第 %d/%d 轮）…\n",
			len(calls), round, handleWebChatMaxDSMLRounds)
		outputs := handleWebChatExecDSML(ctx, calls)
		if len(outputs) == 0 {
			// Every parsed call was outside the whitelist (a quoted
			// DSML example, not an instruction): do NOT send an empty
			// feedback message - the expert would be confused by a
			// blank turn. Strip the quoted markup so the caller sees
			// clean content.
			return WebChatResult{Content: toolcall.StripDSMLToolCalls(message), URL: convURL}, nil
		}
		// Each output is a self-delimiting <tool_result> block, in
		// tool_calls order — newline separation is enough.
		feedback := strings.Join(outputs, "\n")

		// Continue the SAME conversation: same mode, Keep set to the URL
		// returned by the previous send. Explicit construction (not
		// copy-and-clear) so future WebChatOptions fields never leak into
		// follow-ups: no role injection and no re-upload of attachments
		// here - the expert only gets the tool results.
		followUp := WebChatOptions{Mode: opts.Mode, Keep: convURL}
		res, callErr := handleWebChatSend(ctx, feedback, followUp)
		if callErr != nil {
			return WebChatResult{}, fmt.Errorf("webchat tool loop: continue conversation during round %d: %w", round, callErr)
		}
		message = res.Content
		if res.URL != "" {
			convURL = res.URL
		}
	}
	fmt.Fprintf(os.Stderr, "⚠️ 专家连续工具调用超过 %d 轮上限，已返回中间结果\n", handleWebChatMaxDSMLRounds)
	return WebChatResult{Content: toolcall.StripDSMLToolCalls(message), URL: convURL}, nil
}
