package lp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/outfmt"
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
// reply, retrying transient failures and - when the reply is judged to be a
// DSML tool call - executing the tools the expert embeds in its reply.
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
//   - DSML tool loop: a reply (role-driven or plain chat alike) that ENDS
//     with a </tool_calls> close tag - the web expert's own signal that the
//     emission is complete, whatever prose precedes it - has its underlying
//     dscli tools executed locally, and the results fed back into the SAME
//     conversation until the expert produces a final answer. The judge
//     (toolcall.IsDSMLToolCallEnd) is deliberately structural, and the
//     whitelist plus destructive-command interception (see dsmlToolNames /
//     dsmlBlockedCmdRe) are the safety boundary: a long answer that merely
//     cites an <invoke> example does not end with the wrapper close tag and
//     is never executed; role consultations still strip such quotes so
//     callers see clean prose, while plain chat keeps them verbatim.
//   - Round visibility: every reply the loop receives (reasoning + content,
//     with token counts from the site's IndexedDB when available) is
//     printed via outfmt.PrintContent, so a multi-tool consultation shows
//     what the expert said in each round instead of only the tool results.
//     The returned result is marked Printed so callers skip re-printing
//     the final answer.
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
			// The web expert (chat.deepseek.com) emits tool calls natively
			// in DSML: role consultations (code_review's review role) and
			// plain chat alike may receive them. WebChat cannot execute
			// them locally, so we parse, run the underlying dscli tools,
			// and feed the results back into the SAME conversation until
			// the expert produces a final answer.
			//
			// IsDSMLToolCallEnd is the gate: the reply must END with a
			// complete </tool_calls> close tag - the web expert's signal
			// that the emission is complete. A long answer that merely
			// quotes an <invoke> example does not end with the wrapper
			// close tag and must never be executed. Non-executable DSML
			// in role consultations is stripped so callers see clean
			// prose; plain chat keeps it verbatim (the expert's words are
			// content there, not a command).
			if toolcall.IsDSMLToolCallEnd(res.Content) {
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

// handleWebChatToolLoop continues a WebChat conversation (role-driven or
// plain chat; the gate toolcall.IsDSMLToolCallEnd decided entry) while the
// expert emits DSML tool calls: parse the calls, execute them locally, and
// post the results back into the SAME conversation (Keep=first URL). The
// role prompt is injected only on the first round - HandleWebChat already
// rendered it - so follow-up messages carry tool results verbatim.
//
// Every reply received (the first one and each follow-up) is printed via
// outfmt.PrintContent - reasoning and content, with the token counts the
// transport extracted from the site's IndexedDB (0 when unavailable) - so
// the user sees what the expert said in each round, not just the tool
// results. The returned result is marked Printed: the final answer was
// already printed inside the loop, so callers must not re-print it.
//
// The local-execution warning is printed here, not at the caller: this is
// the exact moment a remote model's markup becomes a local command, and it
// applies to plain chat just as much as to role consultations.
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

	// The remote model's DSML markup is about to run locally with the user's
	// OS permissions; say so before the first execution (stderr, so piped
	// stdout stays clean) - silent local execution is the surprise.
	fmt.Fprintf(os.Stderr, "⚠️ 远程模型回复中的 DSML 工具调用（read_file/exec_command/apply_patch）将在本地执行。\n")

	message := first.Content
	convURL := first.URL
	// cleanExit 收尾：角色会话剥离 DSML 标记，露出干净散文；纯聊天原样返回
	// （专家的话是内容，不是命令）——与第一轮的非执行契约保持一致。
	// Printed 表示最后一轮内容已经在循环内打印过，调用方不要重复打印。
	cleanExit := func() (WebChatResult, error) {
		content := message
		if opts.Role != "" {
			content = toolcall.StripDSMLToolCalls(message)
		}
		return WebChatResult{Content: content, URL: convURL, Printed: true}, nil
	}
	printRound := func(res WebChatResult) {
		outfmt.PrintContent(ctx, res.Reasoning, res.Content, res.ThinkingTokens, res.ContentTokens)
	}
	// 第一轮回复（进入循环的那份）同样可见：专家为什么发工具调用、思考了什么。
	printRound(first)
	for round := 1; round <= handleWebChatMaxDSMLRounds; round++ {
		// Only tool-call replies continue the loop (see IsDSMLToolCallEnd):
		// a reply that does not end with a wrapper close tag - the expert
		// finished its reasoning, or is quoting an example - ends the loop
		// (stripped for role consultations). Prose BEFORE the wrapper is
		// tolerated: when the closing tag is present the emission is
		// complete, the calls execute and the preamble is discarded with
		// the round.
		calls, parseErr := toolcall.ParseDSMLToolCalls(message)
		if parseErr != nil {
			fmt.Fprintf(os.Stderr, "⚠️ 工具调用不完整，已停止循环: %v\n", parseErr)
			return cleanExit()
		}
		if len(calls) == 0 {
			return cleanExit()
		}
		if !toolcall.IsDSMLToolCallEnd(message) {
			return cleanExit()
		}

		fmt.Fprintf(os.Stderr, "🤖 专家请求执行 %d 个工具调用（第 %d/%d 轮）…\n",
			len(calls), round, handleWebChatMaxDSMLRounds)
		outputs := handleWebChatExecDSML(ctx, calls)
		if len(outputs) == 0 {
			// Every parsed call was outside the whitelist (a quoted
			// DSML example, not an instruction): do NOT send an empty
			// feedback message - the expert would be confused by a
			// blank turn. Strip the quoted markup for role consultations
			// so the caller sees clean content; plain chat keeps the
			// original text (it is content, not a command).
			fmt.Fprintf(os.Stderr, "⚠️ 专家回复只包含非白名单工具调用（引用示例？），已跳过执行\n")
			return cleanExit()
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
		// 每轮回复都打印（含 reasoning 与 token 用量）：多轮咨询里用户
		// 看到的不是只有工具结果，还有专家每一轮说什么。
		printRound(res)
	}
	fmt.Fprintf(os.Stderr, "⚠️ 专家连续工具调用超过 %d 轮上限，已返回中间结果\n", handleWebChatMaxDSMLRounds)
	return cleanExit()
}
