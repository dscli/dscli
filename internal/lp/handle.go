package lp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	dsctx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

// webChatMaxInputRunes caps every message HandleWebChat sends to
// chat.deepseek.com. The site rejects inputs past its limit - the composer
// shows "超出字数限制，请删减后发送或开启新对话" / "你输入的信息过长，请调整后重试"
// and the send is dropped, which the wait loop then misreads as a send
// failure and retries in vain.
//
// The limit is a RUNE (character) count, not a byte count. Measured on the
// live site (probe, 2026-08-29): ASCII-only inputs pass at 162000 runes and
// are rejected at ~165339 runes (a 165339-rune code dump was rejected,
// "超长约 4%"), while a mixed input of 180000 BYTES (~105k runes of
// Chinese+code) was accepted. A byte-count cap would cut Chinese text at 1/3
// of its real budget and over-carry ASCII text - both wrong. 155000 is the
// safety value: ~4.5k runes (3%) below the measured reject boundary.
const webChatMaxInputRunes = 155000

// webChatOutputNoteReserve is the rune budget reserved inside a truncated
// tool-feedback message for the trailing "<tool_result>⚠️ …</tool_result>"
// omission note. The note template is 27 tag runes plus ~45 prose runes
// plus small digit counts, so 256 is a conservative reserve that keeps the
// total under webChatMaxInputRunes.
const webChatOutputNoteReserve = 256

// countRunes returns the rune count of s (the site's limit is a character
// count — see webChatMaxInputRunes).
func countRunes(s string) int {
	return utf8.RuneCountInString(s)
}

// headRunes returns the largest prefix of s whose RUNE count is <= maxRunes.
// Slicing []rune(s) never splits a multi-byte rune, so the prefix always
// ends on a UTF-8 sequence boundary; invalid bytes inside s are preserved
// as-is (binary tool output is not sanitized here).
func headRunes(s string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	return string([]rune(s)[:maxRunes])
}

// truncateWebChatMessage caps s at webChatMaxInputRunes (characters) before
// it is sent: keep the head (when HandleWebChat prepends a role/system
// prompt the user's task sits right after it - templates are a few KB, far
// under the cap, so the task statement survives the cut), append an explicit
// marker so the remote model knows the text it sees is partial, and print a
// stderr note for the user. The marker is charged against the budget so the
// result is guaranteed to fit the site's input limit.
func truncateWebChatMessage(s string) string {
	if countRunes(s) <= webChatMaxInputRunes {
		return s
	}
	mark := fmt.Sprintf("\n\n[已截断] 输入原文 %d 字符，超过长度限制（%d 字符），仅保留开头。",
		countRunes(s), webChatMaxInputRunes)
	keep := webChatMaxInputRunes - countRunes(mark)
	fmt.Fprintf(os.Stderr, "⚠️ 输入过长（%d 字符），已截断至 %d 字符再发送（保留开头）。\n",
		countRunes(s), webChatMaxInputRunes)
	return headRunes(s, keep) + mark
}

// webChatOutputNote builds the trailing <tool_result> omission note of a
// truncated feedback message. Extracted so the note-length invariant
// (len(note) <= webChatOutputNoteReserve) can be pinned by a test - the
// whole cap guarantee of buildWebChatFeedback hangs on it.
func webChatOutputNote(omittedCount, omittedRunes int) string {
	if omittedCount > 0 {
		return fmt.Sprintf("<tool_result>⚠️ 输入超过长度限制：已省略 %d 个工具结果（约 %d 字符），以上结果已截断。</tool_result>",
			omittedCount, omittedRunes)
	}
	return fmt.Sprintf("<tool_result>⚠️ 输入超过长度限制：以上结果已截断（约 %d 字符）。</tool_result>", omittedRunes)
}

// buildWebChatFeedback joins tool outputs into the message sent back to the
// expert, truncating at the site's input cap. Truncation is block-wise so the
// DSML structure survives: as many complete <tool_result> blocks as fit are
// kept; the first block that does not fit has its BODY cut (both tags are
// preserved, the block stays well-formed); the remaining blocks are dropped
// and replaced by one explicit omission note. Budget accounting includes the
// "\n" separators that strings.Join inserts - a two-block feed totalling
// exactly the cap would otherwise come out one rune over. Without this an
// over-long tool result (e.g. a big read_file) would hit the site's
// "超出字数限制" rejection mid-conversation, where a rejected send is
// unrecoverable. The cap is a RUNE count (see webChatMaxInputRunes): a
// Chinese-heavy dump earns 3x the byte budget of ASCII, exactly as the
// site counts it.
func buildWebChatFeedback(outputs []string) string {
	total := 0
	for _, o := range outputs {
		total += countRunes(o)
	}
	// Join inserts len(outputs)-1 newlines; they are part of the sent text.
	if total+len(outputs)-1 <= webChatMaxInputRunes {
		return strings.Join(outputs, "\n")
	}

	var kept []string
	// The omission note is appended on EVERY truncation sub-path (cut and
	// drop alike), so its budget is reserved up front once. Without this the
	// drop path - kept blocks may consume the budget down to near-cap before
	// truncateToolResultBlock gives up - would leak the note runes and send
	// over the cap, the exact failure this code exists to prevent.
	budget := webChatMaxInputRunes - webChatOutputNoteReserve
	omittedCount, omittedRunes := 0, 0
	for i, o := range outputs {
		n := countRunes(o)
		if n+1 <= budget {
			// +1 pays for the trailing "\n" after this kept block; the last
			// kept block's +1 becomes the "\n" before the omission note.
			kept = append(kept, o)
			budget -= n + 1
			continue
		}
		// First block that does not fit: cut its body (tags intact) if the
		// remaining budget can hold it (budget-1 reserves its trailing
		// newline); otherwise drop the whole block. All later blocks are
		// omitted and summarized below.
		if head, ok := truncateToolResultBlock(o, budget-1); ok {
			kept = append(kept, head)
			omittedRunes = n - countRunes(head)
		} else {
			omittedRunes = n
			omittedCount++
		}
		for _, rest := range outputs[i+1:] {
			omittedRunes += countRunes(rest)
			omittedCount++
		}
		break
	}

	fmt.Fprintf(os.Stderr, "⚠️ 工具结果过长（约 %d 字符），已截断至 %d 字符再发送（省略 %d 个结果）。\n",
		total, webChatMaxInputRunes, omittedCount)
	return strings.Join(kept, "\n") + "\n" + webChatOutputNote(omittedCount, omittedRunes)
}

// truncateToolResultBlock cuts the BODY of one "<tool_result>…</tool_result>"
// block so the whole block fits maxRunes runes (the caller already reserved
// the omission note separately; maxRunes is the block's own budget). The
// tags are preserved and the body gets an inline marker, so the block
// remains well-formed DSML. ok=false means s is not a well-formed block
// (defensive: future format change) or cannot fit at all - the caller then
// treats the whole output as unkept.
func truncateToolResultBlock(s string, maxRunes int) (out string, ok bool) {
	const (
		open  = "<tool_result>"
		close = "</tool_result>"
	)
	if !strings.HasPrefix(s, open) || !strings.HasSuffix(s, close) {
		return s, false
	}
	mark := "\n⚠️[工具结果过长，已截断]"
	if countRunes(open)+countRunes(close)+countRunes(mark) > maxRunes {
		return s, false // even tags+marker do not fit: give up on this block
	}
	bodyBudget := maxRunes - countRunes(open) - countRunes(close)
	body := s[len(open) : len(s)-len(close)]
	if countRunes(body) <= bodyBudget {
		// Direct-call convenience only: buildWebChatFeedback calls this with
		// maxRunes = budget-1 < n, so there body > bodyBudget always holds.
		return s, true
	}
	keep := bodyBudget - countRunes(mark)
	// After the tags+marker guard above keep >= 0 always holds for the
	// production caller; the clamp only defends hypothetical direct calls.
	if keep < 0 {
		keep = 0
	}
	return open + headRunes(body, keep) + mark + close, true
}

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
//     role tool set plus destructive-command interception (see
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
//   - Input cap: messages (and tool feedback) are truncated at
//     webChatMaxInputRunes before sending - the site rejects longer inputs
//     with "超出字数限制" and the send is dropped, which the wait loop would
//     misread as a retryable send failure. Tool results are cut block-wise so
//     <tool_result> blocks stay well-formed; a truncated marker is appended so
//     the model knows the text is partial.
//
// A retried send targets the same conversation (Keep is preserved): the busy
// server never acknowledged the send, so re-sending is harmless.
func HandleWebChat(ctx context.Context, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleWebChat")
	defer span.Finish()

	// Carry the role in ctx so the DSML executor (ExecuteDSMLToolCalls)
	// gates tool execution by the SAME role config as the doc registration
	// and GetAllTools. Plain chat (Role "") keeps the default "dev" profile -
	// the web model may then call whatever dev's tool config allows.
	if opts.Role != "" {
		ctx = context.WithValue(ctx, dsctx.CurrentRoleKey, opts.Role)
	}

	// WebChat has no system prompt concept, so persona text is prepended to
	// the user message. The separator helps the web model distinguish the
	// instructions from the actual task. System wins over Role, matching the
	// ask layer's previous precedence.
	fullMessage := message
	if opts.System != "" {
		fullMessage = opts.System + "\n\n---\n\n## User Request\n\n" + message
	} else if opts.Role != "" {
		// The DSML tool section is derived from the role's tool config
		// (role_configs / roles.DefaultFor) at send time - the same source
		// as GetAllTools. A role without executable tools (expert/review by
		// default) gets no registration; a configured role gets exactly the
		// tools its config allows.
		doc := toolcall.BuildDSMLToolDoc(ctx, opts.Role)
		fullMessage = prompt.RenderPromptForRoleWithTools(ctx, opts.Role, doc) + "\n\n---\n\n## User Request\n\n" + message
	}
	// The site rejects inputs past its 字数 limit (composer shows "超出字数
	// 限制"), dropping the send; truncate BEFORE sending so the wait loop never
	// sees an unacknowledged submit it would mis-handle. The head is kept and
	// an explicit marker tells the model the text is partial.
	fullMessage = truncateWebChatMessage(fullMessage)

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
			if toolcall.IsDSMLToolCallEnd(res.Content) || toolcall.IsDSMLToolCallCut(res.Content) {
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

// handleWebChatResolveConversation resolves a Keep value to a conversation
// URL. A package variable so tests can replace it (the real resolver reads
// the file-backed registry; tests want deterministic URLs without touching
// the user's Chrome profile directory).
var handleWebChatResolveConversation = resolveConversation

// handleWebChatResumeRead reads the conversation's last assistant message
// via the shared session attached to ctx. A package variable so tests can
// replace it with a mock (skip browser automation).
var handleWebChatResumeRead = func(ctx context.Context, conversationURL string) (content, status string, ok bool) {
	sess := webChatSessionFrom(ctx)
	if sess == nil {
		return "", "", false
	}
	return sess.ReadLastAssistant(ctx, conversationURL)
}

// HandleWebChatResume continues a saved conversation that ended with a
// pending DSML tool-call round — the web-chat twin of dscli chat's resume
// semantics ("last message has tool calls → execute and feed the results
// back; otherwise it's a normal multi-turn conversation"):
//
//   - The conversation's LAST assistant message is read from the site's
//     IndexedDB cache (the continuation point; nothing is sent yet).
//   - When it ends with a </tool_calls>-style close tag (IsDSMLToolCallEnd),
//     the pending calls are executed locally and their results fed back into
//     the SAME conversation (Keep = the conversation URL) until the expert
//     produces a final answer — exactly handleWebChatToolLoop, with the
//     interrupted round as its first message. This is how the QA engineer's
//     round that died mid tool-call (a broken close tag, a killed process)
//     resumes without re-asking the whole assessment.
//   - Otherwise the last message is a normal reply: the conversation is a
//     multi-turn one with nothing pending — the last content is returned
//     as-is (Printed=false) and the caller decides whether to continue with
//     a follow-up send.
//
// opts.Keep selects the conversation (conversation ID, "last", or a full
// URL). opts.Role/System are honored for the tool loop's DSML stripping but
// are NOT injected here: the conversation already carries its own context,
// and re-injecting a role prompt would duplicate it.
func HandleWebChatResume(ctx context.Context, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleWebChatResume")
	defer span.Finish()

	if opts.Keep == "" {
		return WebChatResult{}, fmt.Errorf("webchat resume requires Keep (conversation ID, \"last\", or URL)")
	}
	convURL, err := handleWebChatResolveConversation(opts.Keep)
	if err != nil {
		return WebChatResult{}, err
	}
	if convURL == "" {
		return WebChatResult{}, fmt.Errorf("webchat resume: empty conversation URL for Keep %q", opts.Keep)
	}

	// One browser session serves the whole resume: the read probe and every
	// tool-loop follow-up reuse the same tab, closed once when the call
	// returns (mirrors HandleWebChat).
	sess := newWebChatSession()
	defer sess.Close()
	ctx = withWebChatSession(ctx, sess)

	content, status, ok := handleWebChatResumeRead(ctx, convURL)
	if !ok {
		return WebChatResult{}, fmt.Errorf("webchat resume: cannot read last assistant message (status %q); the conversation may be expired or its IndexedDB record unavailable", status)
	}
	fmt.Fprintf(os.Stderr, "🔁 恢复会话: %s（最后一条消息 %d 字符，status=%s）\n", convURL, countRunes(content), status)

	if !toolcall.IsDSMLToolCallEnd(content) && !toolcall.IsDSMLToolCallCut(content) {
		// Multi-turn conversation: the expert already gave a normal reply —
		// nothing pending, hand the last content to the caller verbatim.
		// A reply that is still streaming (status != FINISHED) is NOT a
		// final answer: surfacing it as the report would hand the caller a
		// half-written conclusion. Refuse with a clear error instead.
		if status != "" && status != "FINISHED" {
			return WebChatResult{}, fmt.Errorf("webchat resume: last assistant message is still %s (not finished); nothing to resume until the reply completes", status)
		}
		return WebChatResult{Content: content, URL: convURL}, nil
	}
	// Pending tool-call round: execute and continue until the final answer.
	// The interrupted round may legitimately be stored non-FINISHED (the
	// cut close tag IS the pending signal) — the loop resolves it.
	return handleWebChatToolLoop(ctx, WebChatResult{Content: content, URL: convURL}, opts)
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
	fmt.Fprintf(os.Stderr, "⚠️ 远程模型回复中的 DSML 工具调用（角色配置允许的本地工具）将在本地执行。\n")

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
	// 每轮回复都打印。Content 原样打印（含 DSML 工具调用块）是刻意为之：
	// 用户看到的不只是工具结果，还有专家为哪个调用、读哪个文件做了哪些
	// 思考；token 数是该轮总输出（站点不区分 thinking/content，thinking
	// 一侧传 0，见 WebChatResult.OutputTokens 注释）。
	printRound := func(res WebChatResult) {
		outfmt.PrintContent(ctx, res.Reasoning, res.Content, 0, res.OutputTokens)
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
		if !toolcall.IsDSMLToolCallEnd(message) && !toolcall.IsDSMLToolCallCut(message) {
			return cleanExit()
		}

		fmt.Fprintf(os.Stderr, "🤖 专家请求执行 %d 个工具调用（第 %d/%d 轮）…\n",
			len(calls), round, handleWebChatMaxDSMLRounds)
		outputs := handleWebChatExecDSML(ctx, calls)
		if len(outputs) == 0 {
			// Every parsed call was not executable (outside the role's
			// tool config, or an unregistered name - a quoted DSML
			// example, not an instruction): do NOT send an empty
			// feedback message - the expert would be confused by a
			// blank turn. Strip the quoted markup for role consultations
			// so the caller sees clean content; plain chat keeps the
			// original text (it is content, not a command).
			fmt.Fprintf(os.Stderr, "⚠️ 专家回复只包含非可执行工具调用（未在角色工具配置中或引用示例？），已跳过执行\n")
			return cleanExit()
		}
		// Each output is a self-delimiting <tool_result> block, in
		// tool_calls order — newline separation is enough. buildWebChatFeedback
		// truncates block-wise when the total exceeds the site's input cap, so
		// an over-long tool result cannot trigger a rejected send mid-loop
		// (unrecoverable: the round is part of a live conversation).
		feedback := buildWebChatFeedback(outputs)

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
