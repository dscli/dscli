package outfmt

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// MarkdownToOrgConverter converts Markdown to Org mode
type MarkdownToOrgConverter struct {
	inCodeBlock     bool
	inOrgBlock      bool
	inBlockQuote    bool
	currentCodeLang string
	buf             bytes.Buffer
	tableBuf        []string // buffered table rows for multi-line processing
}

// NewMarkdownToOrgConverter creates a new converter
func NewMarkdownToOrgConverter() *MarkdownToOrgConverter {
	return &MarkdownToOrgConverter{
		inCodeBlock:     false,
		inOrgBlock:      false,
		inBlockQuote:    false,
		currentCodeLang: "",
	}
}

// ConvertLine converts a single line of Markdown to Org mode
func (c *MarkdownToOrgConverter) ConvertLine(line string) string {
	trimmedLine := strings.TrimSpace(line)

	// --- Table buffering ---
	// Flush pending table when transitioning to a non-table line.
	if len(c.tableBuf) > 0 && !isTableRow(trimmedLine) {
		flushed := c.flushMdTableBuf()
		c.tableBuf = nil
		return flushed + c.convertLineCore(line)
	}

	// Buffer table rows (only outside code/org blocks).
	if !c.inCodeBlock && !c.inOrgBlock && isTableRow(trimmedLine) {
		c.tableBuf = append(c.tableBuf, trimmedLine)
		return ""
	}
	// --- End table buffering ---

	return c.convertLineCore(line)
}

// convertLineCore handles blockquote state transitions, then delegates
// inner-line conversion to convertNonQuoteLine. Blockquotes are detected
// first so that they can contain code blocks, headings, and other elements.
func (c *MarkdownToOrgConverter) convertLineCore(line string) string {
	trimmedLine := strings.TrimSpace(line)

	// ---- Blockquote handling (outermost wrapper) ----

	// Exit blockquote: current line is NOT a blockquote line but we were in one.
	if c.inBlockQuote && !isBlockQuoteLine(trimmedLine) {
		c.inBlockQuote = false
		// Close the quote, then process the current line normally.
		return "#+end_quote\n" + c.convertNonQuoteLine(line)
	}

	// Enter or continue blockquote.
	if isBlockQuoteLine(trimmedLine) {
		// Strip the "> " prefix(es) and process the remaining line.
		// Nested "> > text" becomes just "text".
		innerLine := stripBlockQuotePrefix(line)
		innerResult := c.convertNonQuoteLine(innerLine)

		if !c.inBlockQuote {
			c.inBlockQuote = true
			return "#+begin_quote\n" + innerResult
		}
		return innerResult
	}

	// ---- Non-quote line: delegate to original pipeline ----
	return c.convertNonQuoteLine(line)
}

// isBlockQuoteLine reports whether trimmed line starts a markdown blockquote.
func isBlockQuoteLine(trimmed string) bool {
	return strings.HasPrefix(trimmed, ">")
}

// stripBlockQuotePrefix removes all leading ">", space, and tab characters
// from a blockquote line, returning the inner content.
// "> > text" → "text", ">text" → "text", "> " → "".
func stripBlockQuotePrefix(line string) string {
	return strings.TrimLeft(line, " >\t")
}

// isCJK reports whether r is a CJK character, fullwidth punctuation, or
// other wide character that prevents Org emphasis markers (*, /, +, =, ~, _,
// ^) from being recognized without surrounding spaces.
//
// The Org manual §12.2 requires emphasis markers to sit at word boundaries
// (preceded by whitespace or specific punctuation).  CJK text has no
// inter-word spaces, so markers directly adjacent to CJK characters are
// invisible to Org — the output must insert a space on the boundary.
func isCJK(r rune) bool {
	if r < 0x2000 {
		return false // fast path for ASCII, Latin-1, general punctuation
	}
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r) ||
		(r >= 0x3000 && r <= 0x303F) || // CJK Symbols and Punctuation
		(r >= 0xFF00 && r <= 0xFFEF) || // Halfwidth and Fullwidth Forms
		(r >= 0xFE30 && r <= 0xFE4F) // CJK Compatibility Forms
}

// writePreSpaceIfCJK writes a space to sb when the last output rune is CJK.
// Call this before writing an Org emphasis-marker character so that Org
// recognises the marker.
func writePreSpaceIfCJK(sb *strings.Builder) {
	s := sb.String()
	if len(s) == 0 {
		return
	}
	runes := []rune(s)
	if isCJK(runes[len(runes)-1]) {
		sb.WriteByte(' ')
	}
}

// writePostSpaceIfCJK writes a space to sb when the rune at position pos in
// text is CJK.  Call this after writing an Org emphasis-marker character so
// that Org recognises the closing marker.
func writePostSpaceIfCJK(sb *strings.Builder, text string, pos int) {
	if pos >= len(text) {
		return
	}
	r, _ := utf8.DecodeRuneInString(text[pos:])
	if r != utf8.RuneError && isCJK(r) {
		sb.WriteByte(' ')
	}
}

// convertNonQuoteLine handles code blocks, org blocks, headings, and inline
// formatting. This is the original convertLineCore logic, extracted so that
// blockquote-wrapped lines can recursively pass through it.
func (c *MarkdownToOrgConverter) convertNonQuoteLine(line string) string {
	// 保存原始行是否有换行符
	hasNewline := strings.HasSuffix(line, "\n")
	trimmedLine := strings.TrimSpace(line)

	// Handle code blocks - 必须放在最前面
	if after, ok := strings.CutPrefix(trimmedLine, "```"); ok {
		if !c.inCodeBlock {
			// Code block start
			c.inCodeBlock = true
			lang := after
			lang = strings.TrimSpace(lang)
			if lang == "" {
				lang = "text"
			}
			return fmt.Sprintf("#+begin_src %s\n", lang)
		} else {
			// Code block end
			c.inCodeBlock = false
			return "#+end_src\n"
		}
	}

	// handle org blocks - 需要精确匹配
	if strings.HasPrefix(trimmedLine, "#+begin_src") || strings.HasPrefix(trimmedLine, "#+begin_example") {
		if !c.inOrgBlock {
			c.inOrgBlock = true
		}
		// 保持原始行的换行符
		if hasNewline && !strings.HasSuffix(line, "\n") {
			return line + "\n"
		}
		return line
	}

	if strings.HasPrefix(trimmedLine, "#+end_src") || strings.HasPrefix(trimmedLine, "#+end_example") {
		if c.inOrgBlock {
			c.inOrgBlock = false
		}
		// 保持原始行的换行符
		if hasNewline && !strings.HasSuffix(line, "\n") {
			return line + "\n"
		}
		return line
	}

	// If in code block, return as-is
	if c.inCodeBlock || c.inOrgBlock {
		// 保持原始行的换行符
		if hasNewline && !strings.HasSuffix(line, "\n") {
			return line + "\n"
		}
		return line
	}

	// Convert headers (# -> *, ## -> **, etc.)
	if len(line) > 0 && line[0] == '#' {
		level := 0
		for level < len(line) && line[level] == '#' {
			level++
		}
		if level > 0 && level <= 6 {
			text := strings.TrimSpace(line[level:])
			// 对标题文本应用格式转换
			text = c.convertMarkdownSimple(text)
			stars := strings.Repeat("*", level)
			return fmt.Sprintf("%s %s\n", stars, text)
		}
	}

	result := line

	// Use simple string processing
	result = c.convertMarkdownSimple(result)

	// 确保有换行符
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return result
}

// flushMdTableBuf converts buffered markdown table rows to org table.
// Returns the converted lines as a single string with \n separators.
// If no valid table separator is found, passes lines through convertLineCore.
func (c *MarkdownToOrgConverter) flushMdTableBuf() string {
	defer func() { c.tableBuf = nil }()

	if len(c.tableBuf) < 2 {
		return c.passThroughMdTableBuf()
	}

	// Find the separator line index.
	sepIdx := -1
	for i, line := range c.tableBuf {
		if isMarkdownTableSeparator(line) {
			sepIdx = i
			break
		}
	}

	// No separator, or separator at first position (no header).
	if sepIdx <= 0 {
		return c.passThroughMdTableBuf()
	}

	// Parse all rows, skipping separator lines.
	var rows [][]string
	for _, line := range c.tableBuf {
		if isMarkdownTableSeparator(line) {
			continue
		}
		cells := parseTableRow(line)
		if cells != nil {
			rows = append(rows, cells)
		}
	}

	if len(rows) == 0 {
		return c.passThroughMdTableBuf()
	}

	// Determine max columns across all rows.
	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	// Calculate per-column display widths based on converted cell content.
	// Uses runewidth.StringWidth (not len) to match Emacs org-string-width
	// behavior for CJK and other wide characters.
	colWidths := make([]int, maxCols)
	for _, row := range rows {
		for i, cell := range row {
			converted := c.convertMarkdownSimple(cell)
			w := runewidth.StringWidth(converted)
			if w > colWidths[i] {
				colWidths[i] = w
			}
		}
	}

	// Build org table output.
	var result strings.Builder
	for rowIdx, row := range rows {
		result.WriteString("|")
		for colIdx := 0; colIdx < maxCols; colIdx++ {
			var cell string
			if colIdx < len(row) {
				cell = c.convertMarkdownSimple(row[colIdx])
			}
			result.WriteString(" ")
			result.WriteString(cell)
			// Right-pad to column display width (using runewidth for CJK).
			pad := colWidths[colIdx] - runewidth.StringWidth(cell)
			for pad > 0 {
				result.WriteByte(' ')
				pad--
			}
			result.WriteString(" |")
		}
		result.WriteString("\n")

		// Insert org separator after header (first) row.
		// Format: |-------+-------+-----|
		if rowIdx == 0 {
			result.WriteString("|")
			for colIdx := 0; colIdx < maxCols; colIdx++ {
				// Two extra dashes for the spaces around cell content.
				for i := 0; i < colWidths[colIdx]+2; i++ {
					result.WriteByte('-')
				}
				if colIdx < maxCols-1 {
					result.WriteByte('+')
				}
			}
			result.WriteString("|\n")
		}
	}

	return result.String()
}
// passThroughMdTableBuf passes buffered table lines through convertNonQuoteLine
// individually, used when the buffer doesn't contain a valid table.
func (c *MarkdownToOrgConverter) passThroughMdTableBuf() string {
	var result strings.Builder
	for _, line := range c.tableBuf {
		result.WriteString(c.convertNonQuoteLine(line + "\n"))
	}
	return result.String()
}

func (c *MarkdownToOrgConverter) convertMarkdownSimple(text string) string {
	var result strings.Builder
	i := 0
	n := len(text)

	for i < n {
		// Check for bold **
		if i+1 < n && text[i] == '*' && text[i+1] == '*' {
			// Find closing **
			j := i + 2
			for j < n {
				if j+1 < n && text[j] == '*' && text[j+1] == '*' {
					// Found closing **
					boldText := text[i+2 : j]
					// 递归处理粗体中的斜体
					boldText = c.convertItalicInBold(boldText)
					writePreSpaceIfCJK(&result)
					result.WriteString("*")
					result.WriteString(boldText)
					result.WriteString("*")
					i = j + 2
					writePostSpaceIfCJK(&result, text, i)
					break
				}
				j++
			}
			if j >= n {
				// No closing ** found
				result.WriteString(text[i:])
				break
			}
			continue
		}

		// Check for italic * (but not part of **)
		if text[i] == '*' && (i == 0 || text[i-1] != '*') && (i+1 >= n || text[i+1] != '*') {
			// Find closing *
			j := i + 1
			for j < n {
				if text[j] == '*' && (j+1 >= n || text[j+1] != '*') {
					// Found closing *
					italicText := text[i+1 : j]
					writePreSpaceIfCJK(&result)
					result.WriteString("/")
					result.WriteString(italicText)
					result.WriteString("/")
					i = j + 1
					writePostSpaceIfCJK(&result, text, i)
					break
				}
				j++
			}
			if j >= n {
				// No closing * found
				result.WriteByte(text[i])
				i++
			}
			continue
		}

		// Check for strikethrough ~~
		if i+1 < n && text[i] == '~' && text[i+1] == '~' {
			j := i + 2
			for j < n {
				if j+1 < n && text[j] == '~' && text[j+1] == '~' {
					strikeText := text[i+2 : j]
					writePreSpaceIfCJK(&result)
					result.WriteString("+")
					result.WriteString(strikeText)
					result.WriteString("+")
					i = j + 2
					writePostSpaceIfCJK(&result, text, i)
					break
				}
				j++
			}
			if j >= n {
				result.WriteString(text[i:])
				break
			}
			continue
		}

		// Check for inline code `
		if text[i] == '`' {
			// Count consecutive backticks
			tickCount := 1
			for i+tickCount < n && text[i+tickCount] == '`' {
				tickCount++
			}
			// 2+ backticks (`` or ```...): not inline code, pass through
			if tickCount >= 2 {
				for k := 0; k < tickCount; k++ {
					result.WriteByte('`')
				}
				i += tickCount
				continue
			}
			// Single backtick: inline code
			j := i + 1
			for j < n && text[j] != '`' {
				j++
			}
			if j < n {
				codeText := text[i+1 : j]
				result.WriteString(" =")
				result.WriteString(codeText)
				result.WriteString("= ")
				i = j + 1
			} else {
				result.WriteByte(text[i])
				i++
			}
			continue
		}

		// Check for links [text](url)
		if text[i] == '[' {
			// Find closing ]
			bracketEnd := -1
			for j := i + 1; j < n; j++ {
				if text[j] == ']' {
					bracketEnd = j
					break
				}
			}
			if bracketEnd != -1 && bracketEnd+1 < n && text[bracketEnd+1] == '(' {
				// Find closing )
				parenEnd := -1
				for j := bracketEnd + 2; j < n; j++ {
					if text[j] == ')' {
						parenEnd = j
						break
					}
				}
				if parenEnd != -1 {
					linkText := text[i+1 : bracketEnd]
					url := text[bracketEnd+2 : parenEnd]
					result.WriteString("[[")
					result.WriteString(url)
					result.WriteString("][")
					result.WriteString(linkText)
					result.WriteString("]]")
					i = parenEnd + 1
					continue
				}
			}
		}

		// Default: copy character
		result.WriteByte(text[i])
		i++
	}

	return result.String()
}

// convertItalicInBold converts italic text inside bold text
func (c *MarkdownToOrgConverter) convertItalicInBold(text string) string {
	var result strings.Builder
	i := 0
	n := len(text)

	for i < n {
		// Check for italic * inside bold
		if text[i] == '*' && (i == 0 || text[i-1] != '*') && (i+1 >= n || text[i+1] != '*') {
			// Find closing *
			j := i + 1
			for j < n {
				if text[j] == '*' && (j+1 >= n || text[j+1] != '*') {
					// Found closing *
					italicText := text[i+1 : j]
					writePreSpaceIfCJK(&result)
					result.WriteString("/")
					result.WriteString(italicText)
					result.WriteString("/")
					i = j + 1
					writePostSpaceIfCJK(&result, text, i)
					break
				}
				j++
			}
			if j >= n {
				// No closing * found
				result.WriteByte(text[i])
				i++
			}
			continue
		}

		// Default: copy character
		result.WriteByte(text[i])
		i++
	}

	return result.String()
}

// ConvertLines converts input to output line by line (simpler, more reliable)

func (c *MarkdownToOrgConverter) ConvertLines(input string, output io.Writer) error {
	// Reset state to prevent cross-call corruption.
	// A previous call might have left inCodeBlock=true
	// (e.g. DebugBytes interrupted, or malformed Printf without closing fence),
	// which would cause subsequent output to misinterpret
	// opening/closing fences and produce reversed begin_src/end_src.
	c.inCodeBlock = false
	c.inOrgBlock = false
	c.inBlockQuote = false
	c.currentCodeLang = ""
	c.tableBuf = nil
	// Discard any leftover content from previous malformed calls
	// (e.g. Printf without trailing \n that left content in buf).
	c.buf.Reset()

	for _, r := range input {
		if r != '\n' {
			c.buf.WriteRune(r)
		} else {
			line := c.buf.String()
			converted := c.ConvertLine(line + "\n")
			if _, err := output.Write([]byte(converted)); err != nil {
				return fmt.Errorf("failed to write output: %w", err)
			}
			c.buf.Reset()
		}
	}

	// Flush remaining content when input doesn't end with \n.
	if c.buf.Len() > 0 {
		line := c.buf.String()
		converted := c.ConvertLine(line + "\n")
		if _, err := output.Write([]byte(converted)); err != nil {
			return fmt.Errorf("failed to write output: %w", err)
		}
		c.buf.Reset()
	}

	// Flush remaining table buffer.
	if len(c.tableBuf) > 0 {
		flushed := c.flushMdTableBuf()
		if _, err := output.Write([]byte(flushed)); err != nil {
			return fmt.Errorf("failed to write table output: %w", err)
		}
	}

	// Flush unclosed blockquote (input ended while still in quote).
	if c.inBlockQuote {
		if _, err := output.Write([]byte("#+end_quote\n")); err != nil {
			return fmt.Errorf("failed to write end_quote: %w", err)
		}
		c.inBlockQuote = false
	}

	return nil
}
