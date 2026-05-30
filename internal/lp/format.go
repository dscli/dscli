package lp

import (
	"bytes"
	"encoding/json"
	"strings"
)

// prettyJSONInMarkdown finds fenced code blocks in markdown, tries to parse
// their content as JSON, and pretty-prints valid JSON blocks with a "json"
// language tag. Non-JSON blocks are left unchanged.
//
// This turns compact single-line JSON like:
//
//	```
//	{"key":"value"}
//	```
//
// into:
//
//	```json
//	{
//	  "key": "value"
//	}
//	```
func prettyJSONInMarkdown(md string) string {
	// Prepend newline so that opening fences at position 0 are found.
	s := "\n" + md

	var (
		result strings.Builder
		rest   = s
		fence  = "\n```"
	)

	result.Grow(len(s))

	for {
		idx := strings.Index(rest, fence)
		if idx == -1 {
			result.WriteString(rest)
			break
		}

		// Write up to and including "\n```".
		result.WriteString(rest[:idx+len(fence)])
		rest = rest[idx+len(fence):]

		if len(rest) == 0 {
			break
		}

		// Extract language tag: text between ``` and the next newline.
		nl := strings.IndexByte(rest, '\n')
		if nl == -1 {
			result.WriteString(rest)
			break
		}

		langTag := rest[:nl]     // e.g. "", "json", "go"
		afterLang := rest[nl+1:] // code content + closing fence

		// Find closing fence in the code content.
		end := strings.Index(afterLang, fence)
		if end == -1 {
			result.WriteString(rest)
			break
		}

		rawCode := afterLang[:end] // code between fences

		// Write the opening line: language tag + newline + code.
		if pretty, ok := tryParseJSON(rawCode); ok {
			result.WriteString("json\n")
			result.WriteString(pretty)
		} else {
			result.WriteString(langTag)
			result.WriteByte('\n')
			result.WriteString(rawCode)
		}

		// Continue after the closing fence.
		rest = afterLang[end:]
	}

	return strings.TrimPrefix(result.String(), "\n")
}

// tryParseJSON attempts to pretty-print s as JSON using json.Indent.
// This preserves number precision and avoids double-parsing.
// Returns pretty-printed JSON and true if valid, or ("", false) otherwise.
func tryParseJSON(s string) (string, bool) {
	trimmed := strings.TrimSpace(s)
	if trimmed == "" {
		return "", false
	}

	// Quick check: valid JSON containers start with { or [.
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return "", false
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, []byte(trimmed), "", "  "); err != nil {
		return "", false
	}

	return buf.String(), true
}
