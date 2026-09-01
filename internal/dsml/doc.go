// DSML tool documentation for WebChat role prompts.
//
// chat.deepseek.com has no native tool protocol: the role prompt is the only
// channel that can register tools for the HandleWebChat loop, and the web
// model emits calls in DSML markup. This file renders that registration
// section dynamically from the role's tool configuration (roles.DefaultFor +
// role_configs), so `dscli webchat --role X` registers exactly the tools the
// local engine will actually execute - the same set GetAllTools exposes to
// dscli chat. No separate DSML whitelist: `dscli role update --tools` is the
// single place that decides which tools a role may use.
//
// There is NO hand-written DSML naming layer: every entry is generated
// straight from the registered ToolDef (dsmlGeneratedDocEntry), so the
// model sees the native tool names and parameter schemas - what the model
// writes is what the executor accepts, no translation.
//
// Since JSON schemas proved too verbose for the role prompt, the section is
// now one block per tool: a heading, the tool's own description, a one-line
// parameter summary, and an XML invocation example. No JSON schema is
// rendered anywhere.
package dsml

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

// dsmlXMLClose fragments assemble the literal XML close tags emitted in the
// generated examples. This file deliberately avoids a whole close-tag
// sequence (parameter, invoke, or tool_calls) anywhere in its source: when
// such a sequence appears inside a write or code_edit payload, this session's
// file-modification host reads it as the end of a tool-call block and
// truncates the payload at that point, corrupting the file. Open-tag
// fragments without a closing bracket (e.g. the "<invoke name=\"" prefix in
// dsmlGeneratedDocEntry) do not trigger it. The fragments reconstruct the
// exact close tags at runtime, so the generated prompt output is
// byte-identical to the literal form; the 32 literal close tags already in
// dsml.go (16 invoke, 3 parameter, 8 tool_calls, 5 tool_result, across parse
// regexes and result templates) are the precedent that the output text is
// what matters, not how this file spells it.
var (
	dsmlParamClose  = "</para" + "meter>"
	dsmlInvokeClose = "</inv" + "oke>"
	dsmlCallsClose  = "</tool_" + "calls>"
)

// dsmlDocTool is one tool entry in the DSML registration section.
type dsmlDocTool struct {
	// dsmlName is the name the model writes in <invoke name="...">.
	dsmlName string
	// description is the tool's own description (def.Description), shown
	// verbatim under the per-tool heading.
	description string
	// example is one XML invocation sample.
	example string
	// paramsLine summarizes the parameters in one line.
	paramsLine string
}

// dsmlGeneratedDocEntry builds a DSML documentation entry straight from a
// registered ToolDef: the DSML layer uses the native tool name and parameter
// schema, so the description and example/params line are derived from it.
// This is the only doc source - the role's tools spec decides what gets
// registered, with no code change needed per tool.
func dsmlGeneratedDocEntry(def toolcall.ToolDef) dsmlDocTool {
	name := def.Name
	props, _ := def.Parameters["properties"].(map[string]any)
	keys := sortedSchemaKeys(props)
	requiredSet := map[string]bool{}
	switch req := def.Parameters["required"].(type) {
	case []string:
		for _, r := range req {
			requiredSet[r] = true
		}
	case []any:
		for _, r := range req {
			if s, ok := r.(string); ok {
				requiredSet[s] = true
			}
		}
	}

	var b strings.Builder
	b.WriteString("<tool_calls>\n")
	b.WriteString("<invoke name=\"")
	b.WriteString(name)
	b.WriteString("\">\n")
	for _, k := range keys {
		p, _ := props[k].(map[string]any)
		typ, _ := p["type"].(string)
		switch typ {
		case "integer", "number":
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">0%s\n", k, dsmlParamClose)
		case "boolean":
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">true%s\n", k, dsmlParamClose)
		case "array":
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">[]%s\n", k, dsmlParamClose)
		case "object":
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">{}%s\n", k, dsmlParamClose)
		default:
			fmt.Fprintf(&b, "<parameter name=%q string=\"true\">...%s\n", k, dsmlParamClose)
		}
	}
	b.WriteString(dsmlInvokeClose)
	b.WriteString("\n")
	b.WriteString(dsmlCallsClose)

	var parts []string
	for _, k := range keys {
		p, _ := props[k].(map[string]any)
		typ, _ := p["type"].(string)
		desc, _ := p["description"].(string)
		opt := "optional"
		if requiredSet[k] {
			opt = "required"
		}
		parts = append(parts, fmt.Sprintf("`%s` (%s, %s) — %s", k, typ, opt, desc))
	}
	paramsLine := "Parameters: " + strings.Join(parts, "; ")
	if len(parts) == 0 {
		// A schema without properties (no required params) still gets a
		// useful registration line; the empty example body is parseable.
		paramsLine = "Parameters: (no parameters)"
	}

	return dsmlDocTool{
		dsmlName:    name,
		description: def.Description,
		example:     b.String(),
		paramsLine:  paramsLine,
	}
}

// sortedSchemaKeys returns the property keys of a JSON-schema properties
// object, sorted for stable output (map iteration order is not stable).
func sortedSchemaKeys(props map[string]any) []string {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// dsmlGeneratedEntries builds DSML doc entries for every role-configured tool,
// straight from its registered ToolDef. "all" covers every registered tool -
// the role's tools spec is the only gate (same source as GetAllTools), so a
// role configured via `dscli role update --tools` gets its tool registered for
// the web model with no code change. Unregistered names in an explicit list
// are skipped with a debug log: they would fail at execution anyway, and
// registering them only misleads the model into failed calls.
func dsmlGeneratedEntries(ctx context.Context, spec string) []dsmlDocTool {
	allow := toolcall.AllowSetFromSpec(spec)
	var names []string
	if allow == nil { // "all"
		names = toolcall.KnownToolNames()
	} else {
		for n := range allow {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	var out []dsmlDocTool
	seen := map[string]bool{}
	for _, n := range names {
		def, ok := toolcall.GetToolDef(ctx, n)
		if !ok {
			outfmt.Debug("dsml doc: role tool %q is not a registered tool and will not be registered\n", n)
			continue
		}
		if seen[def.Name] {
			continue // alias and canonical name in the same spec: register once
		}
		seen[def.Name] = true
		out = append(out, dsmlGeneratedDocEntry(def))
	}
	return out
}

// BuildDSMLToolDoc renders the DSML tool registration section for a role,
// driven by the role's tool configuration (role_configs row, falling back to
// roles.DefaultFor) - the same spec that gates GetAllTools. Every entry is
// generated from the registered ToolDef, so the DSML layer and the local
// executor share one schema. The section is one block per tool (heading,
// description, parameter line, XML example); no JSON schemas are rendered.
//
// Tool titles intentionally use H2 ("## `name`"), the same level as
// "## 🛠️ Available Tools:", per the task specification: one section per tool
// maximizes visibility. Do not downgrade them to H3.
//
// An empty result means the role has no executable tools and the prompt
// templates drop the section entirely.
func BuildDSMLToolDoc(ctx context.Context, role string) prompt.DSMLToolDoc {
	span, ctx := clog.StartSpanFromContext(ctx, "BuildDSMLToolDoc")
	defer span.Finish()

	spec := toolcall.RoleToolsSpec(ctx, role)
	entries := dsmlGeneratedEntries(ctx, spec)
	if len(entries) == 0 {
		return prompt.DSMLToolDoc{}
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, "`"+e.dsmlName+"`")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "## 🛠️ Available Tools: %s\n\n", strings.Join(names, ", "))
	b.WriteString("You have access to a set of tools to help answer the user's question. Call the\n")
	b.WriteString("tools with DSML markup in your reply. Independent calls may be issued in one\n")
	b.WriteString("reply; dependent calls must wait for previous results.\n\n")
	b.WriteString("You MUST follow this exact format for every tool call:\n")
	b.WriteString("- Wrap all calls in a tool_calls block; each call is an invoke block with a name attribute.\n")
	b.WriteString("- Every argument MUST be a parameter tag carrying a string attribute (string=\"true\" for text, string=\"false\" for numbers/booleans/arrays/objects).\n")
	b.WriteString("- Do NOT add any extra attribute (such as justification) or any argument outside the examples.\n")
	b.WriteString("- Every invoke tag and every tool_calls tag MUST be closed (the close tag must be present).\n")
	b.WriteString("- You may emit several invoke blocks in one reply (independent calls); dependent calls must wait for the previous round's results.\n\n")
	for _, e := range entries {
		fmt.Fprintf(&b, "## `%s`\n\n", e.dsmlName)
		if desc := strings.TrimSpace(e.description); desc != "" {
			b.WriteString(desc)
			b.WriteString("\n\n")
		}
		b.WriteString(e.paramsLine)
		b.WriteString("\n\n")
		b.WriteString("```xml\n")
		b.WriteString(e.example)
		b.WriteString("\n```\n\n")
	}
	b.WriteString("String parameters should be specified as is and set `string=\"true\"`. For all\n")
	b.WriteString("other types (numbers, booleans, arrays, objects), pass the value in JSON format\n")
	b.WriteString("and set `string=\"false\"`.\n\n")
	b.WriteString("You MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.\n")
	b.WriteString("No extra attributes (such as justification), no extra arguments, and every tag closed - output the exact DSML shape shown above.\n")
	intro := strings.TrimSpace(b.String())

	return prompt.DSMLToolDoc{Intro: intro}
}
