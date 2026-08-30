package dsml

import (
	_ "embed"
	"strings"
	"testing"
)

// The six files below are verbatim captures of non-conforming DSML emitted by
// the web expert (dscli/dscli .dscli/tmp/dsml.org). They exercise the parser's
// tolerance contracts: parameter values embedding DSML tag text, a typo'd
// wrapper close, a call dropped without its close tags, a bare invoke ending
// in an empty-name close, and a task value carrying a full tool-call example.

//go:embed testdata/case1_edit_argument.txt
var wcCase1 string

//go:embed testdata/case2_write_file.txt
var wcCase2 string

//go:embed testdata/case3_no_close.txt
var wcCase3 string

//go:embed testdata/case4_two_shells.txt
var wcCase4 string

//go:embed testdata/case5_agent_task.txt
var wcCase5 string

//go:embed testdata/case6_bare_short_close.txt
var wcCase6 string

func TestWebchatCase1EditArgument(t *testing.T) {
	// code_edit whose oldText/newText values contain literal DSML tag text
	// (Go comments about <parameter>/</invoke>), closed with a typo'd </_calls>.
	calls, err := ParseDSMLToolCalls(wcCase1)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "code_edit" {
		t.Fatalf("calls = %d (%+v), want one code_edit call", len(calls), calls)
	}
	for _, k := range []string{"file", "mode", "oldText", "newText"} {
		if _, ok := calls[0].Args[k]; !ok {
			t.Errorf("missing arg %q in %v", k, calls[0].Args)
		}
	}
	oldText, _ := calls[0].Args["oldText"].(string)
	if !strings.Contains(oldText, "ParseDSMLToolCalls extracts all DSML tool calls") {
		t.Errorf("oldText value truncated or corrupted:\n%q", oldText)
	}
	newText, _ := calls[0].Args["newText"].(string)
	if !strings.Contains(newText, "psed") && !strings.Contains(newText, "parseDSMLToolCallsStrict") {
		t.Errorf("newText value truncated or corrupted:\n%q", newText)
	}
	if !IsDSMLToolCallReply(wcCase1) {
		t.Error("IsDSMLToolCallReply = false, want true (typo'd wrapper close is a reply)")
	}
}

func TestWebchatCase2WriteFile(t *testing.T) {
	// write_file whose content value is a full Go file snippet containing
	// literal parameter/invoke tag text in comments.
	calls, err := ParseDSMLToolCalls(wcCase2)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "write_file" {
		t.Fatalf("calls = %d (%+v), want one write_file call", len(calls), calls)
	}
	if got, _ := calls[0].Args["path"].(string); got != "internal/dsml/dsml.go" {
		t.Errorf("path = %q", got)
	}
	content, _ := calls[0].Args["content"].(string)
	if !strings.Contains(content, "func ParseDSMLToolCalls(text string)") {
		t.Errorf("content value truncated or corrupted:\n%.200q", content)
	}
}

func TestWebchatCase3NoClose(t *testing.T) {
	// The invoke never gets its close tags: the last parameter and the call
	// are implicitly closed at the wrapper close. The script value runs to
	// the wrapper close and must be preserved.
	calls, err := ParseDSMLToolCalls(wcCase3)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "shell" {
		t.Fatalf("calls = %d (%+v), want one shell call", len(calls), calls)
	}
	script, _ := calls[0].Args["script"].(string)
	if !strings.HasPrefix(script, "cd /home/nanjj") {
		t.Errorf("script = %q, want the full command", script)
	}
	if !strings.Contains(script, "head -20") {
		t.Errorf("script truncated at %q", script)
	}
	if !IsDSMLToolCallReply(wcCase3) {
		t.Error("IsDSMLToolCallReply = false, want true")
	}
}

func TestWebchatCase4TwoShells(t *testing.T) {
	// Looks well-formed; the round failed for an unknown reason. The parser
	// must keep producing both calls (regression guard).
	calls, err := ParseDSMLToolCalls(wcCase4)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d, want 2", len(calls))
	}
	for _, c := range calls {
		if c.Name != "shell" {
			t.Errorf("name = %q, want shell", c.Name)
		}
		if _, ok := c.Args["script"]; !ok {
			t.Errorf("shell call missing script arg: %v", c.Args)
		}
	}
}

func TestWebchatCase5AgentTask(t *testing.T) {
	// code_dev call whose task value is a long instruction document carrying
	// a full tool-call example (inside a fenced Go block) and escaped quotes;
	// the wrapper open/close are the typo'd <_calls> form and the task
	// parameter closed late (the timeout parameter right after it).
	calls, err := ParseDSMLToolCalls(wcCase5)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "code_dev" {
		t.Fatalf("calls = %d (%+v), want one code_dev call", len(calls), calls)
	}
	task, _ := calls[0].Args["task"].(string)
	if !strings.HasPrefix(task, "Refactor DSML tool-call logic") {
		t.Fatalf("task value corrupted at start: %.80q", task)
	}
	if !strings.Contains(task, "Note: AGENTS.md already read") {
		t.Errorf("task value truncated before its end:\n%.300q", task)
	}
	if got, ok := calls[0].Args["timeout"].(float64); !ok || got != 3600 {
		t.Errorf("timeout = %v (%T), want 3600 number", calls[0].Args["timeout"], calls[0].Args["timeout"])
	}
	if !IsDSMLToolCallReply(wcCase5) {
		t.Error("IsDSMLToolCallReply = false, want true")
	}
}

func TestWebchatCase6BareShortClose(t *testing.T) {
	// A bare invoke (no wrapper at all) closed with an empty-name </> tag.
	calls, err := ParseDSMLToolCalls(wcCase6)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "shell" {
		t.Fatalf("calls = %d (%+v), want one shell call", len(calls), calls)
	}
	if !IsDSMLToolCallReply(wcCase6) {
		t.Error("IsDSMLToolCallReply = false, want true (empty-name close)")
	}
	if !IsPureDSMLToolCalls(wcCase6) {
		t.Error("IsPureDSMLToolCalls = false, want true (only the call plus wrapper residue)")
	}
}

func TestWebchatCaseEmptyNameCloseShapes(t *testing.T) {
	// The empty-name form admit is the bare slash close only; plain angle
	// brackets in prose must NOT open the gate.
	lt := string(rune(60))
	gts := string(rune(62))
	if !IsDSMLToolCallEnd(lt + "/" + gts) {
		t.Error("bare </> must qualify as an emission-complete signal")
	}
	if IsDSMLToolCallEnd("use " + lt + gts + " for not equal") {
		t.Error("prose ending in <> must NOT qualify")
	}
	if IsDSMLToolCallEnd(lt + gts) {
		t.Error("bare <> must NOT qualify (need the slash)")
	}
	if IsDSMLToolCallEnd(lt + gts + "  ") {
		t.Error("spaced <> must NOT qualify")
	}
	// slash-less <_calls> stays admitted (QA regression, kept by design).
	if !IsDSMLToolCallEnd("x\n" + lt + "_calls" + gts) {
		t.Error("slash-less <_calls> close must still qualify")
	}
}

func TestWebchatCaseMisNestedUnclosedParam(t *testing.T) {
	// A wrapper close after an unclosed parameter whose value embeds a nested
	// unclosed invoke: the outer call executes (tolerance), the value runs to
	// the wrapper close and keeps the embedded text verbatim.
	lt := string(rune(60))
	gts := string(rune(62))
	text := toolCallsBlock(
		lt + "invoke name=\"a\"" + gts + "\n" +
			lt + "parameter name=\"p\" string=\"true\">v\n" +
			lt + "invoke name=\"b\"" + gts + "\n",
	)
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Name != "a" {
		t.Fatalf("calls = %d (%+v), want one a call", len(calls), calls)
	}
	if got, _ := calls[0].Args["p"].(string); !strings.Contains(got, "v\n"+lt+"invoke name=\"b\"") {
		t.Errorf("p value = %q, want the value running to the wrapper close", got)
	}
}
