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
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// DSMLCall is one parsed DSML tool call.
type DSMLCall struct {
	Name string
	Args map[string]any
}

// dsmlInvokeRe matches a complete <invoke> block: <invoke name="X">...</invoke>.
// (?s) lets the body span lines; the body is captured non-greedily so nested
// parameter tags stay inside a single invoke.
var dsmlInvokeRe = regexp.MustCompile(`(?s)<invoke\s+name="([^"]+)"[^>]*>(.*?)</invoke>`)

// dsmlParamRe matches one <parameter> child: name + optional string="true|false".
// DeepSeek sometimes omits the string attribute entirely; the group then
// captures "", and decodeDSMLValue's coercion path keeps text text and
// numbers numeric, so the call is never silently dropped.
var dsmlParamRe = regexp.MustCompile(`(?s)<parameter\s+name="([^"]+)"(?:\s*string="(true|false)")?[^>]*>(.*?)</parameter>`)

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
// the caller. Only an opening tag that is closed with ">" counts, so prose
// mentioning "<invoke" alone never triggers it.
func HasDSMLToolCalls(text string) bool {
	return dsmlInvokeOpenRe.MatchString(text)
}

// dsmlInvokeOpenRe matches an opening <invoke> tag that is actually closed
// with ">" (a bare "<invoke" in prose without ">" is not a call, and a
// malformed name attribute must not be counted as a truncated call either).
var dsmlInvokeOpenRe = regexp.MustCompile(`(?s)<invoke\b[^>]*>`)

// ParseDSMLToolCalls extracts all DSML tool calls from text. It returns an
// error when an <invoke> block is left unclosed (e.g. the response was cut
// off mid-emission): a truncated call must never be executed.
func ParseDSMLToolCalls(text string) ([]DSMLCall, error) {
	opens := len(dsmlInvokeOpenRe.FindAllString(text, -1))
	closes := strings.Count(text, "</invoke>")
	if opens > closes {
		return nil, fmt.Errorf("DSML tool call truncated: %d unclosed <invoke>", opens-closes)
	}
	if opens == 0 {
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

// StripDSMLToolCalls removes tool-call blocks from text, leaving the
// surrounding prose. Used to return a clean partial result when the expert
// stops at a tool call (round cap or parse error). A truncated invoke - an
// opening tag that was never closed - chops everything from that tag to the
// end of the text: the tail is unparseable residue of a cut-off emission.
func StripDSMLToolCalls(text string) string {
	out := dsmlInvokeRe.ReplaceAllString(text, "")
	if loc := dsmlInvokeOpenRe.FindStringIndex(out); loc != nil {
		out = out[:loc[0]]
	}
	// Collapse leftover empty <tool_calls> wrappers and blank noise.
	out = strings.ReplaceAll(out, "<tool_calls>", "")
	out = strings.ReplaceAll(out, "</tool_calls>", "")
	return strings.TrimSpace(out)
}

// dsmlToolNames is the whitelist of tools a WebChat expert may invoke.
// Review is a read-only context: write/execute-anything tools stay closed.
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
// (cmd/justification/timeout-in-ms); the shell tool uses
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

// ExecuteDSMLToolCalls executes parsed DSML calls in order and returns one
// feedback block per call, formatted the way WebChat experts expect:
// "<tool> 工具调用的结果：" followed by the stdout in a code fence (or the
// error text when the call failed), so the expert can diagnose and continue.
func ExecuteDSMLToolCalls(ctx context.Context, calls []DSMLCall) (outputs []string) {
	for _, inv := range calls {
		name, args, err := normalizeDSMLInvoke(inv)
		if err != nil {
			outputs = append(outputs, formatDSMLToolError(inv.Name, err.Error()))
			continue
		}
		argsJSON, err := json.Marshal(args)
		if err != nil {
			outputs = append(outputs, formatDSMLToolError(inv.Name, err.Error()))
			continue
		}
		result, _, execErr := HandleToolCall(ctx, name, string(argsJSON))
		if execErr != nil {
			outputs = append(outputs, formatDSMLToolError(inv.Name, execErr.Error()))
			continue
		}
		if result == "" {
			result = "(no output)"
		}
		outputs = append(outputs, formatDSMLToolResult(inv.Name, result))
	}
	return outputs
}

func formatDSMLToolResult(name, result string) string {
	return fmt.Sprintf("%s 工具调用的结果：\n```\n%s\n```", name, result)
}

func formatDSMLToolError(name, errMsg string) string {
	return fmt.Sprintf("%s 工具调用失败：%s", name, errMsg)
}
