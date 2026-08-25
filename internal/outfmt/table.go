package outfmt

import "strings"

// isTableDelimitedLine reports whether trimmed is a pipe-delimited line of at
// least two characters, e.g. "||", "| |", "| a |".
//
// A lone "|" satisfies both prefix and suffix checks by itself, but slicing
// trimmed[1:len-1] on it would panic with slice bounds out of range. Converters
// process arbitrary model output, so it is deliberately not treated as a line.
func isTableDelimitedLine(trimmed string) bool {
	return len(trimmed) >= 2 && strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|")
}

// parseTableRow parses a table row like "| a | b | c |" into ["a", "b", "c"].
// Leading/trailing whitespace in each cell is trimmed.
// Returns nil if the line isn't a pipe-delimited table line.
func parseTableRow(line string) []string {
	trimmed := strings.TrimSpace(line)
	if !isTableDelimitedLine(trimmed) {
		return nil
	}
	inner := trimmed[1 : len(trimmed)-1]
	parts := strings.Split(inner, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}

// isTableRow checks if a line is a table row (pipe-delimited line).
func isTableRow(line string) bool {
	return isTableDelimitedLine(strings.TrimSpace(line))
}

// isOrgTableSeparator checks if a line is an org table separator.
// Org separators look like: |----+----+----|
// Must contain both '-' and '+' characters.
func isOrgTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !isTableDelimitedLine(trimmed) {
		return false
	}
	inner := trimmed[1 : len(trimmed)-1]
	hasMinus := false
	hasPlus := false
	for _, ch := range inner {
		switch ch {
		case '-':
			hasMinus = true
		case '+':
			hasPlus = true
		case '|', ' ', '\t':
			// allowed
		default:
			return false
		}
	}
	return hasMinus && hasPlus
}

// isMarkdownTableSeparator checks if a line is a markdown table separator.
// Markdown separators look like: |---|---| or |:---|:---:|---:|
// Must contain '-' characters; ':' is optional (alignment syntax).
func isMarkdownTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !isTableDelimitedLine(trimmed) {
		return false
	}
	inner := trimmed[1 : len(trimmed)-1]
	hasMinus := false
	for _, ch := range inner {
		switch ch {
		case '-':
			hasMinus = true
		case ':', '|', ' ', '\t':
			// allowed
		default:
			return false
		}
	}
	return hasMinus
}
