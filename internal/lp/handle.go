package lp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	dsctx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/shellblock"
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

// handleWebChatRetryDelays is the backoff sequence between retry attempts for
// transient server overload and truncation (ErrServerBusy / ErrSendRejected /
// ErrTruncated). The official DeepSeek error docs recommend retrying 429/500/
// 503 after a brief wait. A first-round retry (Keep empty) starts a fresh
// conversation, so a duplicate send has no side effects; a retry with Keep
// set (resume, tool-loop follow-up) targets the SAME conversation - busy and
// rejected sends produced no reply, and a truncated follow-up is answered
// with webChatContinueWarning instead of the original text because its
// partial reply already sits in that conversation (see
// handleWebChatFollowUpSend). A package variable so tests can shorten it.
var handleWebChatRetryDelays = []time.Duration{5 * time.Second, 15 * time.Second, 30 * time.Second}

// handleWebChatSend is the transport used by HandleWebChat; tests replace it
// with a mock to skip browser automation.
var handleWebChatSend = WebChatWithOptions

// webChatContinueWarning is sent to the SAME conversation when a follow-up
// reply comes back truncated. The partial answer stays in the conversation
// history, so the model is asked to continue from the cut point instead of
// repeating itself - re-sending the feedback verbatim would duplicate it.
// The round's pending work is a `<shell>` block; the warning names it so a
// block cut mid-emission is re-sent whole.
const webChatContinueWarning = "WARNING: your previous reply was cut off mid-generation (truncated output). Continue from where it stopped; do not repeat what you already sent. If you were in the middle of a `<shell>` block, re-send that block completely."

// handleWebChatTransient reports whether err is a transient web-chat failure
// the retry policy covers: server overload, a rejected send, or a truncated
// reply. Permanent errors (login, bad arguments) fail immediately.
func handleWebChatTransient(err error) bool {
	return errors.Is(err, ErrServerBusy) || errors.Is(err, ErrSendRejected) || errors.Is(err, ErrTruncated)
}

// handleWebChatRetryWait prints the retry notice for a 1-based retry attempt
// and waits out its backoff. It returns ctx.Err() when the caller is
// cancelled while waiting.
func handleWebChatRetryWait(ctx context.Context, err error, attempt int) error {
	delay := handleWebChatRetryDelays[attempt-1]
	reason := "服务器繁忙"
	if errors.Is(err, ErrTruncated) {
		reason = "输出被截断"
	}
	fmt.Fprintf(os.Stderr, "🔄 %s，%.0fs 后重试 (attempt %d/%d)...\n",
		reason, delay.Seconds(), attempt+1, len(handleWebChatRetryDelays)+1)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// handleWebChatFollowUpSend sends one follow-up turn of the shell tool loop
// with the same transient-failure policy as the initial send, so a busy or
// truncated reply mid-conversation no longer aborts the whole session. A
// truncated follow-up already left its partial answer in the conversation,
// so the retry sends webChatContinueWarning instead of the original text;
// busy/rejected sends produced no reply and are re-sent verbatim. opts must
// carry Keep (the conversation URL). Permanent errors stay fatal:
// retrying mid-conversation is not safe for browser/network failures.
func handleWebChatFollowUpSend(ctx context.Context, message string, opts WebChatOptions) (WebChatResult, error) {
	var lastErr error
	for attempt := 0; attempt <= len(handleWebChatRetryDelays); attempt++ {
		if attempt > 0 {
			if err := handleWebChatRetryWait(ctx, lastErr, attempt); err != nil {
				return WebChatResult{}, err
			}
		}
		res, callErr := handleWebChatSend(ctx, message, webChatTransportOptions(opts))
		if callErr == nil {
			return res, nil
		}
		lastErr = callErr
		if !handleWebChatTransient(callErr) {
			return WebChatResult{}, callErr
		}
		if errors.Is(callErr, ErrTruncated) {
			message = webChatContinueWarning
		}
	}
	return WebChatResult{}, fmt.Errorf("%w (after %d attempts)", lastErr, len(handleWebChatRetryDelays)+1)
}

// handleWebChatMaxShellRounds caps the tool-call rounds within one web chat
// consultation. The loop naturally exits whenever a reply carries no
// executable content - the expert always finishes with prose - so this is
// only a failsafe. 6 was too small: real sessions can run many rounds (9
// consecutive tool calls were observed in the design case study), so the cap
// is generous. A variable so tests can shrink it.
var handleWebChatMaxShellRounds = 1024

// handleWebChatMaxShellWarns caps CONSECUTIVE warning rounds in the shell
// loop: blocked scripts count too. After this many unanswered warnings the
// next warn-class round aborts the session instead of sending another
// warning. A variable so tests can shrink it.
var handleWebChatMaxShellWarns = 3

// handleWebChatExecShell is the shell executor hook; tests replace it with
// a mock (the real runner writes the round's scriptN.* / scriptN.txt under
// PROJECT_ROOT/.dscli/tmp and needs no browser).
var handleWebChatExecShell = shellblock.Run

// scriptIcon returns the icon that names one <shell> block's execution mode
// on the user-facing channel. It mirrors scriptMode.String()'s default: an
// unknown label falls back to the bash icon, exactly as the mode label
// itself falls back to "bash".
func scriptIcon(kind string) string {
	switch kind {
	case "python":
		return "🐍"
	case "attach":
		return "📎"
	default:
		return "🐚"
	}
}

// webChatTransportOptions returns the options reduced to what the transport
// accepts: Role/System/SkipPromptInjection are HandleWebChat concerns
// (prompt rendering and injection gating) and are rejected by
// WebChatWithOptions. Every transport send - the initial send and the
// tool-loop follow-ups - goes through this helper, so a new handle-level
// option cannot leak into the transport (the live smoke test caught exactly
// that for SkipPromptInjection).
func webChatTransportOptions(opts WebChatOptions) WebChatOptions {
	transport := opts
	transport.Role = ""
	transport.System = ""
	transport.SkipPromptInjection = false
	return transport
}

// HandleWebChat sends message to chat.deepseek.com and returns the assistant
// reply, retrying transient failures and running every reply through the
// `<shell>` tool loop - WebChat's only tool channel: a reply carrying a
// `<shell>` block has the block executed locally and its output fed back
// into the SAME conversation until the expert produces a final answer.
//
// It is the high-level entry point shared by the ask_expert tool and the
// webchat CLI command: low-level transport (WebChatWithOptions) performs
// exactly one send, while HandleWebChat adds:
//
//   - Role/system prompt rendering: when opts.Role is non-empty, the
//     role-specific prompt template (see prompt.RenderPromptForRole) is
//     prepended, with the `<shell>` tool doc (shellblock.BuildToolDoc)
//     registered in place of the template's tool section; opts.System (raw
//     text) takes precedence over Role. Neither is injected when both are
//     empty - the message is sent verbatim (a session that needs the tool
//     doc carries it in the message itself). opts.SkipPromptInjection
//     suppresses the injection while keeping Role for display (round
//     labels); callers that ship the role instructions as attachments set
//     it. In all cases, injection happens only on the FIRST round of a
//     conversation: a non-empty Keep (resume) skips injection because the
//     session history already carries the persona/tool doc from round one -
//     put any tone or instructions for a resumed session in message instead
//     (that message is subject to the same webChatMaxInputRunes input cap as
//     the first round).
//   - Backoff retry on transient server overload and truncation
//     (ErrServerBusy / ErrSendRejected / ErrTruncated). Permanent errors
//     (login, bad arguments) fail immediately - retrying them is pointless.
//     The tool loop's follow-up sends use the same policy
//     (handleWebChatFollowUpSend), so a transient failure mid-conversation
//     no longer aborts the whole session; a truncated follow-up is answered
//     with a continuation nudge instead of a verbatim re-send.
//   - The `<shell>` tool loop (handleWebChatShellLoop): every reply is
//     judged by shellblock.Judge - a well-formed block runs locally (one
//     block per round; destructive commands are refused) and the merged
//     output returns as an attached scriptN.txt; a format warning or a
//     stalled round is sent back into the SAME conversation; anything else
//     is the final answer. Quoted examples (fenced code, inline spans) never
//     execute.
//   - Round visibility: every reply the loop receives (reasoning + content)
//     is printed via outfmt.PrintContent with the header shown per role
//     (icon + role·label, no token count), so a multi-tool consultation shows
//     what the expert said in each round instead of only the tool results.
//     The returned result is marked Printed so callers skip re-printing
//     the final answer.
//   - Shared browser session: every send of one call (initial, backoff
//     retries, tool follow-ups) reuses a single browser, booted lazily on the
//     first send and closed when the call returns - a 9-tool-call
//     consultation launches Chrome once instead of ten times.
//   - Input cap: messages are truncated at webChatMaxInputRunes before
//     sending - the site rejects longer inputs with "超出字数限制" and the
//     send is dropped, which the wait loop would misread as a retryable
//     send failure. A truncation marker is appended so the model knows the
//     text it sees is partial.
//
// Retries target the same conversation when Keep is set (resume/follow-up) or
// a fresh one when it is empty (first round). Busy/rejected sends were never
// acknowledged, so re-sending is harmless. A truncated initial reply is
// re-sent verbatim; a truncated follow-up uses webChatContinueWarning because
// its partial reply stays in the conversation (see handleWebChatFollowUpSend).
func HandleWebChat(ctx context.Context, message string, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleWebChat")
	defer span.Finish()

	// Carry the role in ctx: the round printer (outfmt.PrintContent) reads
	// it for the reply header. Always set it, even for plain chat (Role
	// ""): a caller may pass a ctx that already carries CurrentRoleKey from
	// an outer chat session, and webchat rounds must not inherit that
	// ambient role. This is DISTINCT from the chat CLI's --role default,
	// which is architect (defaultChatRole in chat.go).
	ctx = context.WithValue(ctx, dsctx.CurrentRoleKey, opts.Role)

	// WebChat has no system prompt concept, so persona text is prepended to
	// the user message. The separator helps the web model distinguish the
	// instructions from the actual task. Injection happens ONLY on the first
	// round (see the doc comment above): a resumed session already carries
	// the persona/tool doc in its history. Within that first round, System
	// wins over Role, matching the ask layer's previous precedence.
	// SkipPromptInjection suppresses the injection entirely while keeping
	// Role for display (round labels) - callers that ship the role
	// instructions as attachments set it.
	fullMessage := message
	firstRound := opts.Keep == ""
	injectPrompt := firstRound && !opts.SkipPromptInjection
	if injectPrompt && opts.System != "" {
		fullMessage = opts.System + "\n\n---\n\n## User Request\n\n" + message
	} else if injectPrompt && opts.Role != "" {
		// Role sessions register WebChat's single tool (`shell`) through the
		// role prompt: chat.deepseek.com has no native tool protocol, so the
		// tool doc is the only registration channel. The role template's
		// ShellToolDoc section renders it (see shellblock.BuildToolDoc).
		fullMessage = prompt.RenderPromptForRoleWithShellTool(ctx, opts.Role, shellblock.BuildToolDoc()) + "\n\n---\n\n## User Request\n\n" + message
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
			if err := handleWebChatRetryWait(ctx, lastErr, attempt); err != nil {
				return WebChatResult{}, err
			}
		}

		// Role/System/SkipPromptInjection are handle-level concerns; the
		// transport rejects them (see WebChatWithOptions), so strip them
		// before the send.
		res, callErr := handleWebChatSend(ctx, fullMessage, webChatTransportOptions(opts))
		if callErr == nil {
			// Entry gate: the reply routes into the shell loop when
			// shellblock.ShouldEnter judges anything other than a final
			// report - a block to run, or a shape that needs a format
			// warning. A reply that reads as a final report (no block, long
			// enough) returns as-is; quoted examples (fenced code, inline
			// spans) never delimit a block.
			if shellblock.ShouldEnter(res.Reasoning, res.Content) {
				return handleWebChatShellLoop(ctx, res, opts)
			}
			return res, nil
		}
		lastErr = callErr
		if !handleWebChatTransient(callErr) {
			return WebChatResult{}, callErr
		}
	}
	return WebChatResult{}, fmt.Errorf("%w (after %d attempts)", lastErr, len(handleWebChatRetryDelays)+1)
}

// handleWebChatResolveConversation resolves a Keep value to a conversation
// URL. A package variable so tests can replace it (the real resolver reads
// the file-backed registry; tests want deterministic URLs without touching
// the user's Chrome profile directory).
var handleWebChatResolveConversation = resolveConversation

// handleWebChatResumeRead reads the conversation tail via the shared session
// attached to ctx. The tail comes from the SERVER (see webchatReadTail): the
// local record is only the sync cursor, so a conversation continued
// elsewhere (the user's own browser, another machine) is seen as the server
// currently has it. A package variable so tests can replace it with a mock
// (skip browser automation).
var handleWebChatResumeRead = func(ctx context.Context, conversationURL string) (content, status, tailRole, reason string, ok bool) {
	sess := webChatSessionFrom(ctx)
	if sess == nil {
		return "", "", "", "no browser session", false
	}
	return sess.ReadTail(ctx, conversationURL)
}

// HandleWebChatResume continues a saved conversation that ended with a
// pending `<shell>` block — the web-chat twin of dscli chat's resume
// semantics ("last message has a pending tool call → execute and feed the
// results back; otherwise it's a normal multi-turn conversation"):
//
//   - The conversation's TAIL (newest message) is read from the SERVER
//     through the site's own history_messages sync (nothing is sent yet).
//     The local record only supplies the sync cursor, so a conversation
//     continued elsewhere (the user's own browser, another machine) is
//     seen as the server currently has it.
//   - The assistant message must be the TAIL of the conversation (newest
//     message): when the newest message is a USER message (typed in the
//     user's browser, no reply landed yet), an older assistant block is
//     not the continuation point and the round refuses with a clear error.
//   - When the content is judged pending (shellblock.ShouldEnter), the
//     block is executed locally and its output fed back into the SAME
//     conversation (Keep = the conversation URL) until the expert produces
//     a final answer — exactly handleWebChatShellLoop, with the interrupted
//     round as its first message. This is how a round that died mid-block
//     (a killed process, a closed browser) resumes without re-asking the
//     whole task.
//   - Execution is gated on a second, independently anchored read: when the
//     conversation changed between the two reads, NOTHING is executed and
//     the caller retries against the new state.
//   - Otherwise the last message is a normal reply: the conversation is a
//     multi-turn one with nothing pending — the last content is returned
//     as-is (Printed=false) and the caller decides whether to continue with
//     a follow-up send.
//
// opts.Keep selects the conversation (conversation ID, "last", or a full
// URL). opts.Role/System are NOT injected here: the conversation already
// carries its own context, and re-injecting a role prompt would duplicate
// it (Role still labels the round printer via ctx).
func HandleWebChatResume(ctx context.Context, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleWebChatResume")
	defer span.Finish()

	// Carry the role in ctx for the round printer (see HandleWebChat).
	ctx = context.WithValue(ctx, dsctx.CurrentRoleKey, opts.Role)

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

	content, status, tailRole, reason, ok := handleWebChatResumeRead(ctx, convURL)
	if !ok {
		if reason == "" {
			reason = "no readable conversation tail"
		}
		return WebChatResult{}, fmt.Errorf("webchat resume: cannot read the conversation tail (%s); the conversation may be gone, or the site did not answer", reason)
	}
	fmt.Fprintf(os.Stderr, "🔁 恢复会话: %s（最新消息 %d 字符，status=%s，role=%s）\n", convURL, countRunes(content), status, tailRole)

	// Tail guard: the message we read must BE the conversation tail. A USER
	// message on top (typed in the user's own browser, reply not landed
	// yet) means an older assistant block is not the continuation point -
	// executing it could run a script the user has already moved past.
	if tailRole != "ASSISTANT" {
		return WebChatResult{}, fmt.Errorf("webchat resume: the newest message is not an assistant reply (role %q); nothing safe to resume (retry after the reply lands)", tailRole)
	}

	pending := shellblock.ShouldEnter("", content)
	if !pending {
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
	// Pending round: execute and continue until the final answer. The
	// interrupted round may legitimately be stored non-FINISHED (the site
	// stopped mid-generation); the loop resolves it.
	//
	// The conversation is shared state and can be continued anywhere: the
	// read above is fresh, but still a snapshot. Execute ONLY when an
	// independent second read still shows the same tail; a conversation
	// that moved on must never run a stale block.
	fmt.Fprintf(os.Stderr, "🔎 二次读取确认会话尾部（其他端可能继续过该会话）...\n")
	content2, _, tailRole2, reason2, ok2 := handleWebChatResumeRead(ctx, convURL)
	if !ok2 || tailRole2 != "ASSISTANT" || strings.TrimSpace(content2) != strings.TrimSpace(content) {
		why := "the conversation changed between the two reads"
		switch {
		case !ok2:
			if reason2 == "" {
				reason2 = "no readable conversation tail"
			}
			why = fmt.Sprintf("the confirmation read failed (%s)", reason2)
		case tailRole2 != "ASSISTANT":
			why = fmt.Sprintf("the newest message is now a %q message", tailRole2)
		}
		return WebChatResult{}, fmt.Errorf("webchat resume: %s; nothing was executed - retry to resume the new state", why)
	}
	return handleWebChatShellLoop(ctx, WebChatResult{Content: content, URL: convURL}, opts)
}

// formatAttachNote renders the stderr audit suffix naming the files a round
// attached (bounded): the operator must see WHAT left the machine, not just
// how many files did. Empty when nothing was attached.
func formatAttachNote(attachments []string) string {
	n := len(attachments)
	if n == 0 {
		return ""
	}
	names := make([]string, 0, n)
	for _, p := range attachments {
		names = append(names, filepath.Base(p))
	}
	const maxShown = 5
	if len(names) > maxShown {
		names = append(names[:maxShown], fmt.Sprintf("…+%d", n-maxShown))
	}
	return fmt.Sprintf("；附件 %d 个（%s）", n, strings.Join(names, ", "))
}

// handleWebChatShellLoop continues a WebChat conversation: every round is
// judged (shellblock.Judge) as an executable block, a format warning, or the
// final report. An executable block is run locally - shellblock.Run writes
// the round's script (scriptN.sh / scriptN.py / scriptN.lst, by mode) and
// scriptN.txt under the project's .dscli/tmp, executes it with the project
// root as its working directory (or scans the file list of an `#!attach`
// block) - and the round's artifacts are fed back into the SAME conversation
// as attachments: scriptN.txt first, then every `#!attach` file in the
// listed order.
//
// Every round (reasoning + content) is printed, the local-execution warning
// is printed here (this is the moment a remote model's script runs
// locally), and the returned result is marked Printed so callers do not
// re-print it. Destructive-command interception (shellblock.Blocked) runs
// before anything is written or executed; a blocked script counts as a
// warn-class round. handleWebChatMaxShellWarns consecutive warnings abort
// the session; handleWebChatMaxShellRounds is the overall failsafe. The
// final answer is returned verbatim: a long report that merely quotes markup
// is content, not a command.
func handleWebChatShellLoop(ctx context.Context, first WebChatResult, opts WebChatOptions) (WebChatResult, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleWebChatShellLoop")
	defer span.Finish()

	roleName := roles.DisplayName(opts.Role)

	// The remote model's script is about to run locally with the user's OS
	// permissions; say so before the first execution (stderr, so piped
	// stdout stays clean) - silent local execution is the surprise.
	fmt.Fprintf(os.Stderr, "⚠️ 远程模型回复中的 `<shell>` 脚本将在本地执行（角色 %s；破坏性命令会被拦截）。\n", roleName)

	message := first.Content
	convURL := first.URL
	printRound := func(res WebChatResult) {
		outfmt.PrintContent(ctx, res.Reasoning, res.Content)
	}
	// Printed: the final answer was already printed inside the loop, so
	// callers must not re-print it.
	cleanExit := func() (WebChatResult, error) {
		return WebChatResult{Content: message, URL: convURL, Printed: true}, nil
	}
	// advance moves the loop to the reply a follow-up send produced.
	advance := func(res WebChatResult) {
		message = res.Content
		if res.URL != "" {
			convURL = res.URL
		}
		printRound(res)
	}
	// First round: the reply that entered the loop is equally visible.
	printRound(first)
	lastReasoning := first.Reasoning

	consecutiveWarns := 0
	// countWarn advances the consecutive-warning counter and reports the
	// abort as an error; shared by the format-warning and blocked-refusal
	// paths so the policy cannot drift.
	countWarn := func(round int) error {
		consecutiveWarns++
		if consecutiveWarns > handleWebChatMaxShellWarns {
			return fmt.Errorf("webchat shell loop: %d consecutive warnings during round %d - aborting", consecutiveWarns, round)
		}
		return nil
	}

	// sendWarnRound delivers one model-facing warning (format contract,
	// blocked-command refusal, or python-unavailable note): it counts the
	// warn-class round, prints the stderr audit line, sends the warning, and
	// advances to the reply. A nil return means "keep looping"; the
	// consecutive-warning abort and the send error come back as err. Shared
	// by every warn path so the policy cannot drift.
	sendWarnRound := func(round int, label, stderrMsg, modelMsg string) error {
		if err := countWarn(round); err != nil {
			return err
		}
		fmt.Fprint(os.Stderr, stderrMsg)
		res, callErr := handleWebChatFollowUpSend(ctx, modelMsg, WebChatOptions{Keep: convURL})
		if callErr != nil {
			return fmt.Errorf("webchat shell loop: %s during round %d: %w", label, round, callErr)
		}
		lastReasoning = res.Reasoning
		advance(res)
		return nil
	}

	// execRound runs one executable block or turns it into a warn-class
	// round: a blocked script and a missing python interpreter are refused
	// model-facing (nothing is written or run), everything else executes and
	// the round's artifacts - scriptN.txt plus any `#!attach` files - ride
	// back in the follow-up message. A nil return means "keep looping".
	execRound := func(round int, verdict shellblock.Verdict) error {
		block := verdict.Block
		if detail, blocked := shellblock.Blocked(block.Script); blocked {
			// Fail-closed: the script is neither written nor run.
			return sendWarnRound(round, "blocked-command refusal",
				fmt.Sprintf("⛔ `<shell>` 脚本命中破坏性命令拦截（%s），已拒绝并请求改写（第 %d/%d 轮）…\n", detail, round, handleWebChatMaxShellRounds),
				shellblock.BlockedWarning(detail))
		}
		// A block can run for a long time (up to 30 minutes), so the user
		// should see what is about to run: the icon names the tool (bash /
		// python / attach), the summary its intent. The mode label comes
		// from shellblock.Mode, the single source shared with
		// detectScriptMode, so the icon and the label cannot drift. A
		// blocked round never reaches this point and announces nothing.
		kind := shellblock.Mode(block.Script)
		if block.Summary != "" {
			outfmt.Printf("%s %s: %s\n", scriptIcon(kind), kind, block.Summary)
		} else {
			outfmt.Println(scriptIcon(kind), kind)
		}
		runRes, runErr := handleWebChatExecShell(ctx, dsctx.ProjectRoot, block.Script, block.Timeout)
		if runErr != nil {
			if errors.Is(runErr, shellblock.ErrNoPythonInterpreter) {
				// A host without python must not kill the session: warn
				// the model and let it fall back to bash, like the
				// blocked-command refusal.
				return sendWarnRound(round, "python-unavailable warning",
					fmt.Sprintf("⚠️ python 解释器不可用，已请求模型改用 bash（第 %d/%d 轮）…\n", round, handleWebChatMaxShellRounds),
					shellblock.NoPythonWarning())
			}
			return fmt.Errorf("webchat shell loop: run script during round %d: %w", round, runErr)
		}
		status := fmt.Sprintf("exit=%d", runRes.ExitCode)
		if runRes.TimedOut {
			status = fmt.Sprintf("timeout after %s", runRes.Timeout)
		}
		attachNote := formatAttachNote(runRes.Attachments)
		// The round's actual files name it in the audit line and the
		// feedback: scriptN.sh / scriptN.py / scriptN.lst, per the mode.
		// Run always sets the paths; a bare mock without them keeps the
		// historical labels instead of ".".
		scriptName := filepath.Base(runRes.ScriptPath)
		if scriptName == "" || scriptName == "." {
			scriptName = fmt.Sprintf("script%d.sh", runRes.Number)
		}
		outputName := filepath.Base(runRes.OutputPath)
		if outputName == "" || outputName == "." {
			outputName = fmt.Sprintf("script%d.txt", runRes.Number)
		}
		fmt.Fprintf(os.Stderr, "▶ %s: %s（%s, %s；输出 %s%s）\n",
			scriptName, block.Summary, status, runRes.Duration.Round(time.Millisecond), outputName, attachNote)
		feedback := fmt.Sprintf("output of %s (attached as %s):", scriptName, outputName)
		// The round's artifacts ride back as attachments: scriptN.txt
		// (the output backfill) first, then every file an `#!attach`
		// block requested, in the listed order.
		attachments := append([]string{runRes.OutputPath}, runRes.Attachments...)
		res, callErr := handleWebChatFollowUpSend(ctx, feedback, WebChatOptions{
			Keep:        convURL,
			Attachments: attachments,
		})
		if callErr != nil {
			return fmt.Errorf("webchat shell loop: feedback send during round %d: %w", round, callErr)
		}
		consecutiveWarns = 0
		lastReasoning = res.Reasoning
		advance(res)
		return nil
	}
	for round := 1; round <= handleWebChatMaxShellRounds; round++ {
		verdict := shellblock.Judge(lastReasoning, message)
		switch verdict.Action {
		case shellblock.ActionFinal:
			// No block, long enough to read as a final report: done. A final
			// reply that merely quotes markup is content, not a command.
			return cleanExit()
		case shellblock.ActionWarn:
			stderrMsg := fmt.Sprintf("⚠️ %s 的回复未通过 `<shell>` 格式判定（%s），已请求重发（第 %d/%d 轮）…\n",
				roleName, verdict.Issue, round, handleWebChatMaxShellRounds)
			if err := sendWarnRound(round, "warning send", stderrMsg, verdict.Warning); err != nil {
				return WebChatResult{}, err
			}
		case shellblock.ActionExecute:
			if err := execRound(round, verdict); err != nil {
				return WebChatResult{}, err
			}
		default:
			return WebChatResult{}, fmt.Errorf("webchat shell loop: unexpected judge action %v during round %d", verdict.Action, round)
		}
	}
	fmt.Fprintf(os.Stderr, "⚠️ %s 的 `<shell>` 块轮次超过 %d 上限，已返回中间结果\n", roleName, handleWebChatMaxShellRounds)
	return cleanExit()
}
