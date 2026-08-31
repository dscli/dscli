package dsml

import "testing"

func TestMalformedDSMLToolCallsTypoInvoke(t *testing.T) {
	text := "<" + "invinvoke name=\"read_file\">\n" +
		"<" + "parameter name=\"path\" string=\"true\">AGENTS.md<" + "/parameter>\n" +
		"<" + "/invoke>\n" +
		"<" + "/tool_calls>"

	if !MalformedDSMLToolCalls(text) {
		t.Fatal("MalformedDSMLToolCalls = false, want true for a typo'd invoke open tag")
	}
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls error = %v, want nil (parser must not see the typo'd tag)", err)
	}
	if len(calls) != 0 {
		t.Fatalf("ParseDSMLToolCalls calls = %d, want 0 (the typo'd tag is not a real invoke)", len(calls))
	}
}

func TestMalformedDSMLToolCallsShapes(t *testing.T) {
	exact := "<" + `invoke name="shell">` + "<" + `parameter name="script" string="true">ls<` + "/parameter><" + "/invoke>"
	div := "<" + `div name="x">content<` + "/div>"
	noName := "<" + "invinvoke>content<" + "/invinvoke>"
	invokee := "<" + `invokee name="shell">` + "<" + `parameter name="script" string="true">ls<` + "/parameter><" + "/invokee>"
	cut := "<" + `tool_calls><invoke name="write_file">` + "<" + `parameter name="path" string="true">a.txt<` + "/parameter><" + "/invoke>" + "\n</"
	cutNoGT := "<" + `tool_calls><invoke name="write_file">` + "<" + `parameter name="path" string="true">a.txt<` + "/parameter><" + "/invoke><" + "/tool_calls"
	complete := "<" + `tool_calls><invoke name="write_file">` + "<" + `parameter name="path" string="true">a.txt<` + "/parameter><" + "/invoke><" + "/tool_calls>"
	fenced := "```\n<" + `invinvoke name="shell">` + "<" + `parameter name="script" string="true">ls<` + "/parameter><" + "/invoke>\n```\n"
	inline := "Example: `<" + `invinvoke name="shell">...` + "/invoke>` pins it."
	proseCut := "remember to wrap your calls in <" + "tool_calls> and close them properly\n</"
	proseCutQuoted := "Use `<" + "invoke name=\"shell\">" + "` exactly as shown\n</"

	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "exact invoke open is not malformed", text: exact, want: false},
		{name: "div with name attr is not malformed", text: div, want: false},
		{name: "invinvoke without name attr is not malformed", text: noName, want: false},
		{name: "invokee typo with name attr is malformed", text: invokee, want: true},
		{name: "cut close after complete invokes is malformed", text: cut, want: true},
		{name: "cut close without greater-than after complete invokes is malformed", text: cutNoGT, want: true},
		{name: "complete tool_calls close is not malformed", text: complete, want: false},
		{name: "fenced typo example is not malformed", text: fenced, want: false},
		{name: "inline-code typo example is not malformed", text: inline, want: false},
		{name: "prose ending with cut close but no tool-call attempt", text: proseCut, want: false},
		{name: "prose ending with cut close quoting example inside code", text: proseCutQuoted, want: false},
		{name: "empty is not malformed", text: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := MalformedDSMLToolCalls(tc.text); got != tc.want {
				t.Errorf("MalformedDSMLToolCalls(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}

// TestMalformedDSMLToolCallsParamValueTypoQuote pins the parameter-body
// awareness of hasTypoInvokeOpen: a typo'd invoke tag INSIDE a parameter
// VALUE is content, not markup - the call must parse and must NOT be
// flagged malformed.
func TestMalformedDSMLToolCallsParamValueTypoQuote(t *testing.T) {
	text := "<tool_calls>\n<invoke name=\"shell\">\n<parameter name=\"script\" string=\"true\">echo '<invinvoke name=\"x\">'</parameter>\n</invoke>\n</tool_calls>"

	if MalformedDSMLToolCalls(text) {
		t.Fatal("MalformedDSMLToolCalls = true, want false (typo inside parameter value is content)")
	}
	calls, err := ParseDSMLToolCalls(text)
	if err != nil {
		t.Fatalf("ParseDSMLToolCalls error = %v, want nil", err)
	}
	if len(calls) != 1 || calls[0].Name != "shell" {
		t.Fatalf("calls = %+v, want one shell call", calls)
	}
	if got, _ := calls[0].Args["script"].(string); got != "echo '<invinvoke name=\"x\">'" {
		t.Errorf("script = %q, want the echoed typo text verbatim", got)
	}
}
