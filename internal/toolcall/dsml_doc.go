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
// There is NO hand-written DSML naming layer anymore: every entry is
// generated straight from the registered ToolDef (dsmlGeneratedDocEntry), so
// the model sees the native tool names and parameter schemas - what the model
// writes is what the executor accepts, no translation. The only legacy
// spelling is exec_command, resolved in the DSML layer (dsmlNativeName and
// normalizeDSMLInvoke) with its old parameter protocol - it is deliberately
// not a registry alias, so chat/API callers never see it.
package toolcall

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/nanjj/clog"
)

// dsmlDocTool is one tool entry in the DSML registration section.
type dsmlDocTool struct {
	// dsmlName is the name the model writes in <invoke name="...">.
	dsmlName string
	// example is one XML invocation sample.
	example string
	// paramsLine summarizes the parameters in one line.
	paramsLine string
	// schema is the JSON schema (the tool's own Parameters).
	schema map[string]any
}

// dsmlGeneratedDocEntry builds a DSML documentation entry straight from a
// registered ToolDef: the DSML layer uses the native tool name and parameter
// schema, so the schema is the tool's own schema and the example/params line
// are derived from it. This is the only doc source - the role's tools spec
// decides what gets registered, with no code change needed per tool.
func dsmlGeneratedDocEntry(def ToolDef) dsmlDocTool {
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
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">0</parameter>\n", k)
		case "boolean":
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">true</parameter>\n", k)
		case "array":
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">[]</parameter>\n", k)
		case "object":
			fmt.Fprintf(&b, "<parameter name=%q string=\"false\">{}</parameter>\n", k)
		default:
			fmt.Fprintf(&b, "<parameter name=%q string=\"true\">...</parameter>\n", k)
		}
	}
	b.WriteString("</invoke>\n")
	b.WriteString("</tool_calls>")

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
	paramsLine := "`" + name + "`: " + strings.Join(parts, "; ")
	if len(parts) == 0 {
		// A schema without properties (no required params) still gets a
		// useful registration line; the empty example body is parseable.
		paramsLine = "`" + name + "`: (no parameters)"
	}

	return dsmlDocTool{
		dsmlName:   name,
		example:    b.String(),
		paramsLine: paramsLine,
		schema: map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": def.Description,
				"parameters":  def.Parameters, // the tool's own JSON schema
			},
		},
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
// registering them only misleads the model into failed calls. The legacy
// exec_command spelling resolves to the shell tool (dsmlNativeName) and is
// registered under the canonical name.
func dsmlGeneratedEntries(ctx context.Context, spec string) []dsmlDocTool {
	allow := allowSetFromSpec(spec)
	var names []string
	if allow == nil { // "all"
		names = KnownToolNames()
	} else {
		for n := range allow {
			names = append(names, n)
		}
		sort.Strings(names)
	}
	var out []dsmlDocTool
	seen := map[string]bool{}
	for _, n := range names {
		def, ok := GetToolDef(ctx, dsmlNativeName(n))
		if !ok {
			outfmt.Debug("dsml doc: role tool %q is not a registered tool and will not be registered\n", n)
			continue
		}
		if seen[def.Name] {
			continue // legacy spelling and canonical name in the same spec: register once
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
// executor share one schema. An empty result means the role has no
// executable tools and the prompt templates drop the section entirely.
func BuildDSMLToolDoc(ctx context.Context, role string) prompt.DSMLToolDoc {
	span, ctx := clog.StartSpanFromContext(ctx, "BuildDSMLToolDoc")
	defer span.Finish()

	spec := roleToolsSpec(ctx, role)
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
	for _, e := range entries {
		b.WriteString("```xml\n")
		b.WriteString(e.example)
		b.WriteString("\n```\n\n")
	}
	b.WriteString("String parameters should be specified as is and set `string=\"true\"`. For all\n")
	b.WriteString("other types (numbers, booleans, arrays, objects), pass the value in JSON format\n")
	b.WriteString("and set `string=\"false\"`.\n\n")
	for _, e := range entries {
		b.WriteString("- ")
		b.WriteString(e.paramsLine)
		b.WriteString("\n")
	}
	intro := strings.TrimSpace(b.String())

	var s strings.Builder
	s.WriteString("### Available Tool Schemas\n\n")
	for _, e := range entries {
		raw, err := json.MarshalIndent(e.schema, "", "  ")
		if err != nil {
			// Schema maps come from the registry; a marshal failure here
			// is a programming error, so the doc must not silently drop a
			// registered tool - surface it loudly.
			raw = []byte(fmt.Sprintf("{/* schema error: %v */}", err))
		}
		s.WriteString("```json\n")
		s.Write(raw)
		s.WriteString("\n```\n\n")
	}
	s.WriteString("You MUST strictly follow the above defined tool name and parameter schemas to invoke tool calls.\n")
	schemas := strings.TrimSpace(s.String())

	return prompt.DSMLToolDoc{Intro: intro, Schemas: schemas}
}
