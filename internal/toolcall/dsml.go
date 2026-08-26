// DSML tool-call support for WebChat (chat.deepseek.com) conversations.
//
// DeepSeek's web model emits tool calls in a DSML-like XML markup instead
// of the OpenAI tool_calls JSON: the assistant's reply text contains
// <tool_calls> blocks with <invoke name="..."> and <parameter> children.
// WebChat cannot execute tools locally, so lp.HandleWebChat parses these
// blocks, executes the underlying dscli tools, and feeds the results back
// into the same conversation (see handleWebChatToolLoop in internal/lp).
//
// The mapping is deliberately narrow: only read-only review tools are
// exposed (exec_command -> shell, read_file). Anything else returns an
// "unsupported tool" feedback so the expert can adapt, instead of silently
// executing an unvetted command.
package toolcall

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/nanjj/clog"
)

// DSMLCall is one parsed DSML tool call.
type DSMLCall struct {
	Name string
	Args map[string]any
}

// dsmlInvokeRe matches a complete <invoke> block: <invoke name="X">...</invoke>.
// (?s) lets the body span lines; the body is captured non-greedily so nested
// parameter tags stay inside a single invoke.
var dsmlInvokeRe = regexp.MustCompile(`(?s)<\s*invoke\s+name="([^"]+)"[^>]*>(.*?)</\s*invoke\s*>`)

// dsmlParamRe matches one <parameter> child: name + optional string="true|false".
// DeepSeek sometimes omits the string attribute entirely; the group then
// captures "", and decodeDSMLValue's coercion path keeps text text and
// numbers numeric, so the call is never silently dropped.
var dsmlParamRe = regexp.MustCompile(`(?s)<\s*parameter\s+name="([^"]+)"(?:\s*string="(true|false)")?[^>]*>(.*?)</\s*parameter\s*>`)

// dsmlEntityReplacer decodes the XML entities DeepSeek may emit inside
// parameter values. &amp; must be last so other entities are not re-escaped.
var dsmlEntityReplacer = strings.NewReplacer(
	"&lt;", "<",
	"&gt;", ">",
	"&quot;", `"`,
	"&apos;", "'",
	"&#39;", "'",
	"&amp;", "&",
)

// HasDSMLToolCalls reports whether text contains a tool-call block, complete
// OR truncated. The open-tag check is deliberate: a cut-off emission (opening
// tag without </invoke>) must still route into the loop, where ParseDSML
// reports the truncation and the residue is stripped instead of leaking to
// the caller. Only a NAMED opening tag closed with ">" counts: a bare
// "<invoke>" in prose carries no tool name (nothing to execute) and prose
// mentioning "<invoke name=" without ">" (including a mid-tag cutoff such as
// "<invoke name=\"exec_command" - no closing ">") never triggers routing.
// The latter is a documented limitation: a cut-off tag without ">" is
// indistinguishable from prose and is passed through untouched.
func HasDSMLToolCalls(text string) bool {
	return dsmlNamedInvokeOpenRe.MatchString(normalizeDSMLText(text))
}

// dsmlNamedInvokeOpenRe matches an opening <invoke> tag that carries a name
// attribute - the only shape that can be a real tool call. It is also the
// gate for HasDSMLToolCalls and the "no opens" early exit in ParseDSMLToolCalls:
// a bare "<invoke>" in prose carries no tool name (nothing to execute) and
// must not count as a truncated call either. <\s*invoke tolerates whitespace
// after the opener: models sometimes emit "< invoke name=..." as a
// tokenization artifact.
//
// The attribute region is consumed as quoted strings ('...' or "...") or
// single chars, so name must be its own attribute at a whitespace boundary:
// "name" inside another attribute ("<invoke filename=...>",
// "<invoke data-name=...>") or INSIDE a quoted attribute value
// ("<invoke note=\"use name=x here\">") must NOT match.
var dsmlNamedInvokeOpenRe = regexp.MustCompile(`(?s)<\s*invoke\b(?:'[^']*'|"[^"]*"|[^'">])*\s+name\s*=[^>]*>`)

// dsmlInvokeCloseRe matches a closing </invoke> tag, tolerating the same
// whitespace variants as dsmlInvokeRe (models emit "</ invoke >").
var dsmlInvokeCloseRe = regexp.MustCompile(`(?s)</\s*invoke\s*>`)

// normalizeDSMLText repairs markup artifacts LLMs commonly emit around DSML
// tags before parsing, so a well-formed call is never misread as truncated
// ("DSML tool call truncated: N unclosed <invoke>"):
//
//   - full-width angle brackets ＜＞ -> half-width (Chinese models often
//     emit them instead of < >)
//   - Unicode format characters (zero-width space/joiner, BOM, soft hyphen,
//     direction marks) are dropped - they are invisible but break exact
//     tag matching. This is INTENTIONAL data loss: the same characters
//     also poison parameter values (shell commands), and pasted content
//     carrying them is a model artifact, not user intent.
//   - junk between a tag opener and its name is removed, but ONLY when a
//     known DSML tag name follows, e.g. "</｜｜\r\nDSML｜｜invoke>" ->
//     "</invoke>". Content such as "<dsml_config" stays untouched.
var dsmlFullwidthReplacer = strings.NewReplacer("＜", "<", "＞", ">")

// dsmlTagJunkRe strips separator noise directly after < or </, before the
// tag name: ASCII/full-width double-pipe markers (||, ｜) and "DSML"
// literals, with whitespace only between noise tokens. (?i) plus
// d\s*s\s*m\s*l covers spelling variants a model may emit ("D S M L").
//
// The noise is stripped ONLY when immediately followed by a known DSML tag
// name (invoke/parameter/tool_calls), which the capture group captures and
// re-emits ($1$2): a global "dsml" sweep would corrupt real content such as
// "cat <dsml_config" or prose "a <d s m l b". Go's RE2 has no lookahead, so
// the "followed by a tag" condition is expressed by capturing the tag name
// instead. Bare whitespace is deliberately NOT noise: "a < b" in prose or
// in a parameter value must survive normalization untouched.
var dsmlTagJunkRe = regexp.MustCompile(`(?i)(</?)(?:[|｜]\s*|d\s*s\s*m\s*l\s*)+((?:invoke|parameter|tool_calls)\b)`)

func normalizeDSMLText(text string) string {
	text = dsmlFullwidthReplacer.Replace(text)
	text = strings.Map(func(r rune) rune {
		if unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, text)
	// $1 keeps the opener (</?), $2 re-emits the captured tag name so the
	// junk match can never eat into content that is not a DSML tag.
	return dsmlTagJunkRe.ReplaceAllString(text, "$1$2")
}

// ParseDSMLToolCalls extracts all DSML tool calls from text. It returns an
// error when an <invoke> block is left unclosed (e.g. the response was cut
// off mid-emission): a truncated call must never be executed.
func ParseDSMLToolCalls(text string) ([]DSMLCall, error) {
	text = normalizeDSMLText(text)
	// Truncation check by state-machine scan: named <invoke> opens are
	// matched against </invoke> closes in text order, with <parameter>
	// bodies treated as opaque content (a raw "<invoke" or "</invoke>"
	// inside a parameter VALUE is content, not structure). A cut-off
	// emission (an opening tag never closed) must never be executed.
	// Unlike an opens-vs-closes count, a stray "</invoke>" in prose can
	// never satisfy an unclosed call (false negative); unlike stripping
	// complete blocks first, a nested or mis-nested open (two opens, one
	// close) is still detected instead of being silently swallowed by the
	// non-greedy block regex.
	if unclosed, _ := unclosedInvokePositions(text); unclosed > 0 {
		return nil, fmt.Errorf("DSML tool call truncated: %d unclosed <invoke>", unclosed)
	}
	if opens := len(dsmlNamedInvokeOpenRe.FindAllString(text, -1)); opens == 0 {
		return nil, nil
	}

	var calls []DSMLCall
	for _, m := range dsmlInvokeRe.FindAllStringSubmatch(text, -1) {
		inv := DSMLCall{Name: m[1], Args: map[string]any{}}
		for _, pm := range dsmlParamRe.FindAllStringSubmatch(m[2], -1) {
			key := pm[1]
			val := decodeDSMLValue(pm[3], pm[2] == "true")
			// Repeated parameters mean an array (DeepSeek models emit
			// <parameter name="justification"> twice for a []string arg).
			if prev, ok := inv.Args[key]; ok {
				if arr, isArr := prev.([]any); isArr {
					inv.Args[key] = append(arr, val)
				} else {
					inv.Args[key] = []any{prev, val}
				}
			} else {
				inv.Args[key] = val
			}
		}
		calls = append(calls, inv)
	}
	return calls, nil
}

// decodeDSMLValue converts a raw parameter value to a Go value. string=true
// keeps the text verbatim; string=false tries numeric or boolean coercion so
// numeric parameters (e.g. timeout) become real numbers, not "10000" text.
func decodeDSMLValue(raw string, isString bool) any {
	if isString {
		return dsmlEntityReplacer.Replace(raw)
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if b, err := strconv.ParseBool(trimmed); err == nil {
		return b
	}
	if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return f
	}
	// Not numeric (e.g. a quoted string without string="true"): keep as text.
	return dsmlEntityReplacer.Replace(raw)
}

// dsmlParamOpenRe / dsmlParamCloseRe delimit a <parameter> body. The
// structural scan needs the outer boundary only (no name/string attribute
// requirement): everything between a parameter open and its close is
// opaque VALUE content, and any tag-looking text inside it (a raw
// "<invoke", a "</invoke>", a nested "<parameter>") must not be read as
// structure.
var dsmlParamOpenRe = regexp.MustCompile(`(?s)<\s*parameter\b[^>]*>`)
var dsmlParamCloseRe = regexp.MustCompile(`(?s)</\s*parameter\s*>`)

// unclosedInvokePositions pairs named <invoke> opening tags with </invoke>
// closes in text order and returns the number of opens left unmatched and
// the byte offset of the FIRST unmatched open (or -1 when there is none).
//
// It is a small state machine, not a bare regex replace, because DSML
// structure must survive model artifacts:
//
//   - A stack gives correct nesting semantics: opens push, closes pop the
//     most recent open; a close with an empty stack is prose noise and is
//     ignored (it neither matches nor cancels anything). Mismatched shapes
//     such as two opens followed by one close leave one unclosed open
//     instead of silently pairing the first open with the first close.
//   - Parameter bodies are opaque: inside a <parameter> value, a raw
//     "<invoke name=...>" (shell snippet or DSML example the model did not
//     entity-escape) is content — it never pushes. A literal "</invoke>"
//     in a value likewise never pops. This is what makes a value's text
//     invisible to the structural scan.
//   - Param depth is counted (an open increments, a close decrements) so
//     nested parameter-looking text inside a value cannot leak structure.
//     Known limitation: a literal "</parameter>" inside a value closes the
//     body early (indistinguishable from a real close without a full XML
//     tokenizer); entity-escaped forms (&lt;/parameter&gt;) are safe
//     because escaping resolution happens after the scan.
func unclosedInvokePositions(text string) (count int, firstPos int) {
	type ev struct {
		pos  int
		kind byte // 'o' invoke open, 'c' invoke close, 'p' param open, 'q' param close
	}
	events := []ev{}
	for _, m := range dsmlNamedInvokeOpenRe.FindAllStringIndex(text, -1) {
		events = append(events, ev{m[0], 'o'})
	}
	for _, m := range dsmlInvokeCloseRe.FindAllStringIndex(text, -1) {
		events = append(events, ev{m[0], 'c'})
	}
	for _, m := range dsmlParamOpenRe.FindAllStringIndex(text, -1) {
		events = append(events, ev{m[0], 'p'})
	}
	for _, m := range dsmlParamCloseRe.FindAllStringIndex(text, -1) {
		events = append(events, ev{m[0], 'q'})
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].pos != events[j].pos {
			return events[i].pos < events[j].pos
		}
		return events[i].kind < events[j].kind
	})
	stack := []int{} // byte offsets of unmatched opens, in text order
	paramDepth := 0
	for _, e := range events {
		switch e.kind {
		case 'p':
			paramDepth++
		case 'q':
			if paramDepth > 0 {
				paramDepth--
			}
		case 'o':
			if paramDepth == 0 {
				stack = append(stack, e.pos)
			}
		case 'c':
			if paramDepth == 0 && len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	if len(stack) == 0 {
		return 0, -1
	}
	return len(stack), stack[0]
}

// StripDSMLToolCalls removes tool-call blocks from text, leaving the
// surrounding prose. Used to return a clean partial result when the expert
// stops at a tool call (round cap or parse error). A truncated invoke - an
// opening tag that was never closed - chops everything from that tag to the
// end of the text: the tail is unparseable residue of a cut-off emission.
// The chop point comes from unclosedInvokePositions, which already treats
// parameter VALUES as opaque, so a raw "<invoke" inside a value is never
// taken for a real truncated call.
func StripDSMLToolCalls(text string) string {
	text = normalizeDSMLText(text)
	if _, first := unclosedInvokePositions(text); first >= 0 {
		text = text[:first]
	}
	out := dsmlInvokeRe.ReplaceAllString(text, "")
	// Collapse leftover empty <tool_calls> wrappers and blank noise.
	out = strings.ReplaceAll(out, "<tool_calls>", "")
	out = strings.ReplaceAll(out, "</tool_calls>", "")
	return strings.TrimSpace(out)
}

// dsmlToolNames is the whitelist of tools a WebChat expert may invoke.
// Review is a read-only context: write/execute-anything tools stay closed.
// 不变式：白名单里的工具（shell/read_file）从不返回 DualMessage——它们的
// handler 结果只可能是文本或错误，不会有附加 user 消息（vision 类才用
// dual 协议，而视觉工具不在白名单）。DSML 执行内核据此丢弃 dual 消息。
var dsmlToolNames = map[string]bool{
	"exec_command": true, // DeepSeek's habitual name for a shell tool
	"shell":        true,
	"read_file":    true,
}

// dsmlBlockedCmdRe rejects destructive shell commands the web model could
// emit. The review prompt asks for read-only commands, but a remote model is
// not a trusted local agent, so clearly destructive patterns are refused
// outright and the expert is told why (it can adapt its approach).
var dsmlBlockedCmdRe = regexp.MustCompile(`(?i)(^|\s|;|&&|\|\|)(` +
	// Filesystem/data destruction.
	`rm\s+(-[a-zA-Z]*[rf][a-zA-Z]*\s+)+(/|~)|` +
	`mkfs|dd\s+[^\n]*of=/dev/|` +
	`chmod\s+-R\s+777\s+/|` +
	// Machine control.
	`shutdown|reboot|halt\b|poweroff|` +
	// Fork bomb.
	`:\(\)\s*\{|` +
	// Privilege escalation / remote code execution.
	`sudo\s+|curl\s+[^\n]*\|\s*(ba)?sh|wget\s+[^\n]*\|\s*(ba)?sh|` +
	// History/state rewriting git operations.
	`git\s+(push\s+(-f|--force)|reset\s+--hard|clean\s+-[a-zA-Z]*[fd]|stash|checkout\s+--))`)

// normalizeDSMLInvoke maps a DSML call to a native tool name and arguments.
//
// exec_command uses the parameter names DeepSeek was trained on
// (cmd/justification/timeout); the shell tool uses
// (script/summary/timeout-in-seconds), so the translation is done here
// rather than by a bare name alias:
//   - cmd     -> script
//   - justification -> summary (first non-empty element, display only)
//   - timeout -> seconds (DSML uses milliseconds)
//
// read_file passes its parameters through (path), and any other name is
// rejected so unvetted tools are never executed.
func normalizeDSMLInvoke(inv DSMLCall) (name string, args ToolArgs, err error) {
	if !dsmlToolNames[inv.Name] {
		return "", nil, fmt.Errorf("unsupported tool %q (available: exec_command, shell, read_file)", inv.Name)
	}
	if inv.Name == "read_file" {
		return "read_file", ToolArgs(inv.Args), nil
	}
	name = "shell"
	args = ToolArgs{}
	if script, ok := inv.Args["script"]; ok && script != "" {
		args["script"] = script
	} else if cmd, ok := inv.Args["cmd"]; ok {
		args["script"] = cmd
	}
	if _, ok := args["script"]; !ok {
		return "", nil, fmt.Errorf("exec_command missing parameter cmd")
	}
	if scriptStr, ok := args["script"].(string); ok && dsmlBlockedCmdRe.MatchString(scriptStr) {
		return "", nil, fmt.Errorf("destructive command rejected (review is read-only): %q", truncateDSMLSummary(scriptStr))
	}
	if _, ok := inv.Args["summary"]; !ok {
		if sum, ok := firstNonEmptyDSMLEntry(inv.Args["justification"]); ok {
			args["summary"] = truncateDSMLSummary(sum)
		}
	}
	if t, ok := inv.Args["timeout"]; ok {
		if secs := dsmlTimeoutSeconds(t); secs > 0 {
			args["timeout"] = secs
		}
	}
	return name, args, nil
}

// firstNonEmptyDSMLEntry returns the first non-empty string of a value that
// may be a string, []any (repeated parameter), or absent.
func firstNonEmptyDSMLEntry(v any) (string, bool) {
	seen := []any{}
	switch x := v.(type) {
	case string:
		seen = []any{x}
	case []any:
		seen = x
	default:
		return "", false
	}
	for _, e := range seen {
		s, ok := e.(string)
		if ok && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), true
		}
	}
	return "", false
}

// truncateDSMLSummary caps the display summary at the shell tool's 40-char
// convention so terminal output stays tidy.
func truncateDSMLSummary(s string) string {
	if len(s) <= 40 {
		return s
	}
	return s[:37] + "..."
}

// dsmlTimeoutSeconds converts a DSML timeout value (milliseconds) to whole
// seconds, the unit the tool framework expects.
func dsmlTimeoutSeconds(v any) int64 {
	var ms float64
	switch x := v.(type) {
	case float64:
		ms = x
	case int:
		ms = float64(x)
	case int64:
		ms = float64(x)
	case string:
		ms, _ = strconv.ParseFloat(strings.TrimSpace(x), 64)
	default:
		return 0
	}
	if ms <= 0 {
		return 0
	}
	secs := int64(ms / 1000)
	if secs < 1 {
		secs = 1
	}
	return secs
}

// dsmlToolCallID 为一次 DSML 调用派生稳定的 ToolCall ID。DSML 标记没有
// ID 字段，而执行内核的 tool 消息需要 ToolCallID；用 name + 规范化参数
// JSON 的 SHA-256 摘要代替——同一调用（同工具同参数）跨轮次得到同一 ID，
// 便于识别。工具使用统计按 name 记录（tools/tool_usage 表），不受 ID 影响。
// 截断为前 8 字节（64-bit 空间）：单次 webchat 会话只有几十个调用，碰撞
// 概率可忽略；完整摘要会放长日志与消息，收益不成比例。
func dsmlToolCallID(name, argsJSON string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + argsJSON))
	return fmt.Sprintf("dsml_%x", sum[:8])
}

// dsmlExecPlan 记录一个 DSML 调用在 ExecuteDSMLToolCalls 中的去向：
// content != nil 表示调用在转换阶段就被拒绝（不执行），直接使用该错误
// 结果；否则 index 是它在可执行列表 tcs（也是 outcomes）中的下标。
type dsmlExecPlan struct {
	content *ToolContent
	index   int
}

// dsmlCallsToToolCalls 把解析出的 DSML 调用转换为协议 ToolCall，交给
// executeToolCalls 批量执行。转换失败的调用（工具不在白名单、参数缺失、
// 危险命令被拦截）不进执行列表，由计划表记录错误结果，保证输出与原始
// 调用 1:1 对齐——专家按顺序对应它自己发出的 tool_calls。
func dsmlCallsToToolCalls(calls []DSMLCall) (tcs []prompt.ToolCall, plan []dsmlExecPlan) {
	plan = make([]dsmlExecPlan, 0, len(calls))
	for _, inv := range calls {
		name, args, err := normalizeDSMLInvoke(inv)
		if err != nil {
			plan = append(plan, dsmlExecPlan{content: &ToolContent{ToolName: inv.Name, Error: err.Error()}})
			continue
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			plan = append(plan, dsmlExecPlan{content: &ToolContent{ToolName: inv.Name, Error: err.Error()}})
			continue
		}
		tcs = append(tcs, prompt.ToolCall{
			ID:   dsmlToolCallID(name, string(argsJSON)),
			Type: "function",
			Function: prompt.ToolCallFunction{
				Name:      name,
				Arguments: string(argsJSON),
			},
		})
		plan = append(plan, dsmlExecPlan{index: len(tcs) - 1})
	}
	return tcs, plan
}

// ExecuteDSMLToolCalls executes parsed DSML calls in order and returns one
// feedback block per call, wrapped the way DeepSeek web models expect:
//
//	<tool_result>{"result":...,"warning":...,"error":...}</tool_result>
//
// (see the DSML encoding README: tool execution results are wrapped in
// <tool_result> tags within user messages, sorted by the order of the
// corresponding tool_calls in the preceding assistant message). The JSON
// payload is the ToolContent shape (result/warning/error), omitzero applied.
// Blocks are 1:1 with the input calls, in order — a call rejected before
// execution (unsupported tool, destructive command, missing parameter) still
// gets its own error block so the expert can adapt its approach.
//
// Execution reuses the HandleToolCalls core (usage statistics, result
// truncation, dual-message split) but does NOT persist tool messages: the
// web conversation lives in the browser, and writing results into the
// current session's messages table would break the assistant↔tool pairing
// that CleanupReverse relies on (the whole ask_expert turn would be dropped
// from history).
func ExecuteDSMLToolCalls(ctx context.Context, calls []DSMLCall) (outputs []string) {
	span, ctx := clog.StartSpanFromContext(ctx, "ExecuteDSMLToolCalls")
	defer span.Finish()

	tcs, plan := dsmlCallsToToolCalls(calls)
	outcomes, dualUsers := executeToolCalls(ctx, tcs, false)
	if len(dualUsers) > 0 {
		// 不变式被打破（见 dsmlToolNames）：白名单工具不应返回 DualMessage，
		// 静默丢弃会丢数据，至少留一条 debug 线索。
		outfmt.Debug("DSML dropped %d dual user message(s); whitelist assumption broken", len(dualUsers))
	}
	for _, step := range plan {
		if step.content != nil {
			outputs = append(outputs, formatDSMLToolResult(step.content))
			continue
		}
		outputs = append(outputs, formatDSMLToolResult(&outcomes[step.index].Content))
	}
	return outputs
}

// formatDSMLToolResult serializes one tool outcome as a DSML tool_result
// block. An all-empty result is normalized to "(no output)" so the model
// sees an explicit answer instead of an empty payload.
func formatDSMLToolResult(c *ToolContent) string {
	dup := *c // never mutate the caller's ToolContent (and avoid shadowing copy)
	if dup.Result == "" && dup.Error == "" && dup.Warning == "" {
		dup.Result = "(no output)"
	}
	b, err := json.Marshal(&dup)
	if err != nil { // unreachable: ToolContent fields are strings only
		return fmt.Sprintf(`<tool_result>{"error":%q}</tool_result>`, err.Error())
	}
	return "<tool_result>" + string(b) + "</tool_result>"
}
