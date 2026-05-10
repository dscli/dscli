package outfmt

import (
	"strings"
)

// OrgToMarkdown converts Org mode content to Markdown.
// This is the reverse of MarkdownToOrgConverter, used when
// output mode is "org" and user input (org format) needs to
// be stored internally as Markdown.
func OrgToMarkdown(input string) string {
	c := &orgToMarkdownConverter{}
	return c.convert(input)
}

type orgToMarkdownConverter struct {
	inSrcBlock     bool
	inQuoteBlock   bool
	inExampleBlock bool
}

// convert converts org content line by line.
func (c *orgToMarkdownConverter) convert(input string) string {
	// Normalize line endings: split on \n, handle \r\n
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\r", "\n")
	lines := strings.Split(input, "\n")

	// Convert each line, collecting results.
	var outLines []string
	for _, line := range lines {
		converted := c.convertLine(line)
		outLines = append(outLines, converted)
	}

	// Trim leading/trailing empty lines that result from
	// block boundaries (e.g. #+begin_quote / #+end_quote).
	start := 0
	for start < len(outLines) && outLines[start] == "" {
		start++
	}
	end := len(outLines)
	for end > start && outLines[end-1] == "" {
		end--
	}

	return strings.Join(outLines[start:end], "\n")
}

// convertLine converts a single org line to markdown.
func (c *orgToMarkdownConverter) convertLine(line string) string {
	// Strip zero-width spaces inserted by markdown2org converter
	// (defensive: clean input that might have been round-tripped)
	line = strings.ReplaceAll(line, "\u200b", "")

	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]

	// 1. Block boundaries: #+begin_xxx / #+end_xxx
	if handled, result := c.handleBlockBoundary(trimmed, indent); handled {
		return result
	}

	// 2. Inside quote block: prefix with "> "
	if c.inQuoteBlock && !c.inSrcBlock && !c.inExampleBlock {
		return indent + "> " + c.convertInline(trimmed)
	}

	// 3. Inside code/example blocks: pass through unchanged
	if c.inSrcBlock || c.inExampleBlock {
		return line
	}

	// 4. Headings: * Title → # Title
	if isOrgHeading(trimmed) {
		return c.convertHeading(trimmed, indent)
	}

	// 5. Normal line: convert inline formatting
	return indent + c.convertInline(trimmed)
}

// handleBlockBoundary detects and converts org block begin/end markers.
func (c *orgToMarkdownConverter) handleBlockBoundary(trimmed, indent string) (handled bool, result string) {
	// #+begin_src lang → ```lang
	if after, ok := strings.CutPrefix(trimmed, "#+begin_src"); ok {
		c.inSrcBlock = true
		lang := strings.TrimSpace(after)
		if lang == "" {
			lang = "text"
		}
		return true, indent + "```" + lang
	}
	// #+end_src → ```
	if trimmed == "#+end_src" {
		c.inSrcBlock = false
		return true, indent + "```"
	}

	// #+begin_example → ```
	if trimmed == "#+begin_example" {
		c.inExampleBlock = true
		return true, indent + "```"
	}
	// #+end_example → ```
	if trimmed == "#+end_example" {
		c.inExampleBlock = false
		return true, indent + "```"
	}

	// #+begin_quote → (start quote mode, no output)
	if trimmed == "#+begin_quote" {
		c.inQuoteBlock = true
		return true, ""
	}
	// #+end_quote → (end quote mode, no output)
	if trimmed == "#+end_quote" {
		c.inQuoteBlock = false
		return true, ""
	}

	return false, ""
}

// isOrgHeading checks if a line is an org heading: 1-6 stars followed by a space.
func isOrgHeading(line string) bool {
	if len(line) == 0 || line[0] != '*' {
		return false
	}
	n := 0
	for n < len(line) && n < 6 && line[n] == '*' {
		n++
	}
	// Must have a space after stars to be a heading
	return n <= 6 && n < len(line) && line[n] == ' '
}

// convertHeading converts org heading (* Title) to markdown heading (# Title).
func (c *orgToMarkdownConverter) convertHeading(line, indent string) string {
	n := 0
	for n < len(line) && line[n] == '*' {
		n++
	}
	text := strings.TrimSpace(line[n:])
	// Convert inline formatting in heading text
	text = c.convertInline(text)
	hashes := strings.Repeat("#", n)
	return indent + hashes + " " + text
}

// convertInline converts org inline formatting to markdown.
// Handles: =code=, *bold*, /italic/, +strikethrough+, _underline_, [[links]]
func (c *orgToMarkdownConverter) convertInline(text string) string {
	var buf strings.Builder
	i := 0
	n := len(text)

	for i < n {
		ch := text[i]

		switch ch {
		case '=':
			// Inline code: =code= → `code`
			if j := findClosing(text, i+1, '='); j != -1 {
				code := text[i+1 : j]
				buf.WriteByte('`')
				buf.WriteString(code)
				buf.WriteByte('`')
				i = j + 1
				continue
			}
		case '*':
			// Bold: *bold* → **bold** (but not when it's a heading or list item)
			// Only convert if surrounded by non-space or at word boundaries
			if j := findClosing(text, i+1, '*'); j != -1 {
				bold := text[i+1 : j]
				buf.WriteString("**")
				buf.WriteString(c.convertInline(bold))
				buf.WriteString("**")
				i = j + 1
				continue
			}
		case '/':
			// Italic: /italic/ → *italic*
			if j := findClosing(text, i+1, '/'); j != -1 {
				italic := text[i+1 : j]
				buf.WriteByte('*')
				buf.WriteString(c.convertInline(italic))
				buf.WriteByte('*')
				i = j + 1
				continue
			}
		case '+':
			// Strikethrough: +text+ → ~~text~~
			if j := findClosing(text, i+1, '+'); j != -1 {
				strike := text[i+1 : j]
				buf.WriteString("~~")
				buf.WriteString(c.convertInline(strike))
				buf.WriteString("~~")
				i = j + 1
				continue
			}
		case '_':
			// Underline: _text_ → <u>text</u> (markdown has no underline)
			if j := findClosing(text, i+1, '_'); j != -1 {
				under := text[i+1 : j]
				buf.WriteString("<u>")
				buf.WriteString(c.convertInline(under))
				buf.WriteString("</u>")
				i = j + 1
				continue
			}
		case '~':
			// Org also supports ~code~ for verbatim (same as =code=)
			if j := findClosing(text, i+1, '~'); j != -1 {
				code := text[i+1 : j]
				buf.WriteByte('`')
				buf.WriteString(code)
				buf.WriteByte('`')
				i = j + 1
				continue
			}
		case '[':
			// Links: [[url][desc]] → [desc](url) or [[url]] → [url](url)
			if i+1 < n && text[i+1] == '[' {
				if result, consumed := c.convertLink(text, i); consumed > 0 {
					buf.WriteString(result)
					i += consumed
					continue
				}
			}
		}

		buf.WriteByte(ch)
		i++
	}

	return buf.String()
}

// convertLink converts [[url][desc]] or [[url]] to markdown link.
// Returns the markdown string and number of bytes consumed from text.
func (c *orgToMarkdownConverter) convertLink(text string, start int) (string, int) {
	// Find the closing ]]
	close := strings.Index(text[start+2:], "]]")
	if close == -1 {
		return "", 0
	}
	close += start + 2
	inner := text[start+2 : close] // content between [[ and ]]

	// Check for [[url][desc]] format
	if bracket := strings.Index(inner, "]["); bracket != -1 {
		url := inner[:bracket]
		desc := inner[bracket+2:]
		return "[" + desc + "](" + url + ")", close + 2 - start
	}

	// [[url]] format
	url := inner
	return "[" + url + "](" + url + ")", close + 2 - start
}

// findClosing finds the closing delimiter starting from pos.
// Returns the position of the closing delimiter, or -1 if not found.
// The closing delimiter must be followed by a space, punctuation, or end of string
// (to avoid matching mid-word delimiters).
func findClosing(text string, start int, delim byte) int {
	for i := start; i < len(text); i++ {
		if text[i] == delim {
			// Check that this is a real closing delimiter:
			// either end of string, or followed by space/punctuation/end
			if i+1 >= len(text) || isBoundary(text[i+1]) {
				return i
			}
		}
	}
	return -1
}

// isBoundary checks if a byte is a word boundary character.
func isBoundary(b byte) bool {
	switch b {
	case ' ', '\t', '\n', '.', ',', ';', ':', '!', '?', ')', ']', '}', '>', '-', '/', '\\', '"', '\'':
		return true
	default:
		return false
	}
}
