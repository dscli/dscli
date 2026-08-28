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
// The documentation metadata below describes the DSML layer (parameter names
// the model writes, e.g. cmd/justification/timeout-in-milliseconds), which is
// deliberately NOT the native tool schema (script/summary/timeout-in-seconds
// for the shell tool). Keeping the two apart is what makes the rendered
// examples executable: normalizeDSMLInvoke translates from DSML names to the
// native ones, and anything documented here must round-trip through it.
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
	// exec_command is DeepSeek's habitual name for the shell tool.
	dsmlName string
	// filterNames are the role-config names that enable this entry
	// ("exec_command" and "shell" are synonyms: both map to the same
	// local tool, and normalizeDSMLInvoke accepts both spellings).
	filterNames []string
	// example is one XML invocation sample (DSML parameter names).
	example string
	// paramsLine summarizes the DSML-layer parameters in one line.
	paramsLine string
	// schema is the DSML-layer JSON schema (see the package comment).
	schema map[string]any
}

// dsmlDocToolsAll is the hand-written DSML documentation, in stable display
// order. It covers the tools whose DSML layer differs from the native one
// (exec_command's cmd/justification/timeout-ms naming vs shell's
// script/summary/timeout-s). Tools without an entry here are documented by
// dsmlGeneratedDocEntry straight from their registered ToolDef, so a role
// configured via `dscli role update --tools` is always registered for the
// web model with no code change.
var dsmlDocToolsAll = []dsmlDocTool{
	{
		dsmlName:    "exec_command",
		filterNames: []string{"exec_command", "shell"},
		example: `<tool_calls>
<invoke name="exec_command">
<parameter name="cmd" string="true">go test ./internal/...</parameter>
<parameter name="justification" string="true">Verify the behavior being discussed</parameter>
<parameter name="timeout" string="false">120000</parameter>
</invoke>
</tool_calls>`,
		paramsLine: "`exec_command`: `cmd` (string, required) — shell command; `justification` (string, optional); `timeout` (integer, optional, **milliseconds**, e.g. 10000 = 10s; omit for the default 120s).",
		schema: dsmlFunctionSchema("exec_command",
			"Run a shell command on the local project host. Prefer read-only commands (go test, git show, git log, grep, sed, ls).",
			map[string]any{
				"cmd":           dsmlStringProp("Shell command to run."),
				"justification": dsmlStringProp("Why this command answers the question (display only)."),
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in milliseconds, e.g. 10000 = 10s. Omit for the default 120s.",
				},
			},
			[]string{"cmd"}),
	},
	{
		dsmlName:    "read_file",
		filterNames: []string{"read_file"},
		example: `<tool_calls>
<invoke name="read_file">
<parameter name="path" string="true">internal/foo/bar.go</parameter>
<parameter name="justification" string="true">Why this file answers the question</parameter>
</invoke>
</tool_calls>`,
		paramsLine: "`read_file`: `path` (string, required) — file to read; `justification` (string, optional). Optional `start_line`/`end_line` (integer, 1-based inclusive) read only a slice — prefer them for large files.",
		schema: dsmlFunctionSchema("read_file",
			"Read a file, or a 1-based inclusive line range of it. Path is relative to the repo root.",
			map[string]any{
				"path":          dsmlStringProp("File path, e.g. main.go"),
				"start_line":    dsmlIntProp("Start line (1-based), optional, default 1"),
				"end_line":      dsmlIntProp("End line, optional, default end of file"),
				"justification": dsmlStringProp("Why this file answers the question (display only)."),
			},
			[]string{"path"}),
	},
	{
		dsmlName:    "apply_patch",
		filterNames: []string{"apply_patch"},
		example: `<tool_calls>
<invoke name="apply_patch">
<parameter name="patch" string="true">--- a/internal/foo/bar.go
+++ b/internal/foo/bar.go
@@ -10,3 +10,5 @@
 func Foo() {
+    return 1
+}
+</parameter>
<parameter name="check" string="false">true</parameter>
<parameter name="justification" string="true">Dry-run the suggested fix before applying it</parameter>
</invoke>
</tool_calls>`,
		paramsLine: "`apply_patch`: `patch` (string, required) — unified diff text, or a path to a `.patch` file; `cwd` (string, optional, default project root, must stay inside it); `check` (boolean, optional, true = dry-run, no writes); `reverse` (boolean, optional, true = undo). Returns `{applied, check_only, changed_files, summary, error}`.",
		schema: dsmlFunctionSchema("apply_patch",
			"Apply a unified diff patch. Atomic: a conflict fails the whole patch with no partial writes.",
			map[string]any{
				"patch":         dsmlStringProp("Unified diff text, or a path to a .patch file."),
				"cwd":           dsmlStringProp("Git repo root, default project root; must stay inside it."),
				"check":         dsmlBoolProp("true = dry-run, no writes."),
				"reverse":       dsmlBoolProp("true = reverse-apply (undo)."),
				"justification": dsmlStringProp("Why this tool call is needed (display only)."),
			},
			[]string{"patch"}),
	},
}

func dsmlStringProp(desc string) map[string]any {
	return map[string]any{"type": "string", "description": desc}
}

func dsmlIntProp(desc string) map[string]any {
	return map[string]any{"type": "integer", "description": desc}
}

func dsmlBoolProp(desc string) map[string]any {
	return map[string]any{"type": "boolean", "description": desc}
}

// dsmlFunctionSchema builds one OpenAI-style function schema for a DSML tool.
func dsmlFunctionSchema(name, desc string, props map[string]any, required []string) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        name,
			"description": desc,
			"parameters": map[string]any{
				"type":                 "object",
				"properties":           props,
				"required":             required,
				"additionalProperties": false,
			},
		},
	}
}

// dsmlDocForSpec resolves a stored tools spec ("all", "", "a,b") into the
// hand-written DSML documentation entries it matches. Names without a
// hand-written entry are not dropped - BuildDSMLToolDoc adds them via
// dsmlGeneratedDocEntry from the registered ToolDef.
func dsmlDocForSpec(spec string) []dsmlDocTool {
	allow := allowSetFromSpec(spec)
	if allow != nil && len(allow) == 0 {
		return nil // explicitly nothing
	}
	var out []dsmlDocTool
	for _, t := range dsmlDocToolsAll {
		if allow == nil { // "all"
			out = append(out, t)
			continue
		}
		for _, n := range t.filterNames {
			if allow[n] {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// dsmlGeneratedDocEntry builds a DSML documentation entry straight from a
// registered ToolDef: the DSML layer uses the native parameter names, so the
// schema is the tool's own schema and the example/params line are derived
// from it. Used for every role-configured tool that has no hand-written
// entry in dsmlDocToolsAll.
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
		dsmlName:    name,
		filterNames: []string{name},
		example:     b.String(),
		paramsLine:  paramsLine,
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

// dsmlGeneratedEntries adds auto-generated DSML doc entries for every
// role-configured tool without a hand-written entry. "all" covers every
// registered tool - the role's tools spec is the only gate (same source as
// GetAllTools), so a role configured via `dscli role update --tools` gets
// its tool registered for the web model with no code change. Unregistered
// names in an explicit list are skipped with a debug log: they would fail
// at execution anyway, and registering them only misleads the model into
// failed calls.
func dsmlGeneratedEntries(ctx context.Context, spec string, existing []dsmlDocTool) []dsmlDocTool {
	allow := allowSetFromSpec(spec)
	covered := map[string]bool{}
	for _, e := range existing {
		for _, n := range e.filterNames {
			covered[n] = true
		}
	}
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
	for _, n := range names {
		if covered[n] {
			continue // hand-written entry already registered
		}
		def, ok := GetToolDef(ctx, n)
		if !ok {
			outfmt.Debug("dsml doc: role tool %q is not a registered tool and will not be registered\n", n)
			continue
		}
		out = append(out, dsmlGeneratedDocEntry(def))
	}
	return out
}

// BuildDSMLToolDoc renders the DSML tool registration section for a role,
// driven by the role's tool configuration (role_configs row, falling back to
// roles.DefaultFor) - the same spec that gates GetAllTools. Hand-written
// entries cover the DSML-layer naming (exec_command etc.); every other
// role-configured tool is documented straight from its ToolDef. An empty
// result means the role has no executable tools and the prompt templates
// drop the section entirely.
func BuildDSMLToolDoc(ctx context.Context, role string) prompt.DSMLToolDoc {
	span, ctx := clog.StartSpanFromContext(ctx, "BuildDSMLToolDoc")
	defer span.Finish()

	spec := roleToolsSpec(ctx, role)
	entries := dsmlDocForSpec(spec)
	entries = append(entries, dsmlGeneratedEntries(ctx, spec, entries)...)
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
			// Schema maps are hand-built constants; a marshal failure here
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
