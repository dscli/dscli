package outfmt

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// MarkdownToOrgConverter converts Markdown to Org mode
// MarkdownToOrgConverter converts Markdown to Org mode
type MarkdownToOrgConverter struct {
	inCodeBlock     bool
	inOrgBlock      bool
	currentCodeLang string
	buf             bytes.Buffer
	tableBuf        []string // buffered table rows for multi-line processing
}

// NewMarkdownToOrgConverter creates a new converter
func NewMarkdownToOrgConverter() *MarkdownToOrgConverter {
	return &MarkdownToOrgConverter{
		inCodeBlock:     false,
		inOrgBlock:      false,
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

// convertLineCore contains the original ConvertLine logic (code blocks,
// headings, inline formatting).
func (c *MarkdownToOrgConverter) convertLineCore(line string) string {
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
			return strings.ReplaceAll(line, "_", "_\u200b") + "\n"
		}
		return strings.ReplaceAll(line, "_", "_\u200b")
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

	// Calculate per-column widths based on converted cell content.
	// Use visibleLen because convertMarkdownSimple inserts zero-width spaces.
	colWidths := make([]int, maxCols)
	for _, row := range rows {
		for i, cell := range row {
			converted := c.convertMarkdownSimple(cell)
			w := visibleLen(converted)
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
			// Right-pad to column width (use visibleLen for accurate spacing).
			pad := colWidths[colIdx] - visibleLen(cell)
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

// passThroughMdTableBuf passes buffered table lines through convertLineCore
// individually, used when the buffer doesn't contain a valid table.
func (c *MarkdownToOrgConverter) passThroughMdTableBuf() string {
	var result strings.Builder
	for _, line := range c.tableBuf {
		result.WriteString(c.convertLineCore(line + "\n"))
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
					result.WriteString("\u200b*")
					result.WriteString(boldText)
					result.WriteString("*\u200b")
					i = j + 2
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
					result.WriteString("\u200b/")
					result.WriteString(italicText)
					result.WriteString("/\u200b")
					i = j + 1
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
					result.WriteString("\u200b+")
					result.WriteString(strikeText)
					result.WriteString("+\u200b")
					i = j + 2
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
			j := i + 1
			for j < n && text[j] != '`' {
				j++
			}
			if j < n {
				codeText := text[i+1 : j]
				result.WriteString("\u200b=")
				result.WriteString(codeText)
				result.WriteString("=\u200b")
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
					result.WriteString("\u200b/")
					result.WriteString(italicText)
					result.WriteString("/\u200b")
					i = j + 1
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
// ConvertLines converts input to output line by line (simpler, more reliable)
func (c *MarkdownToOrgConverter) ConvertLines(input string, output io.Writer) error {
	// Reset state to prevent cross-call corruption.
	// A previous call might have left inCodeBlock=true
	// (e.g. DebugBytes interrupted, or malformed Printf without closing fence),
	// which would cause subsequent output to misinterpret
	// opening/closing fences and produce reversed begin_src/end_src.
	c.inCodeBlock = false
	c.inOrgBlock = false
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

	return nil
}